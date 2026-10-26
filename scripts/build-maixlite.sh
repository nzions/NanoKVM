#!/bin/bash
#
# Build kvm_system without MaixCDK (support/sg2002/maixlite), hold its flags to the
# MaixCDK golden (tools/binabi/flags.sh), strip it as MaixCDK's gen_binary.cmake did for a
# Release build, and stage it at kvmapp/kvm_system/kvm_system.
#
# Usage: scripts/build-maixlite.sh <nanokvm-tree> <build-dir>
# Environment: MAIXLITE_TOOLCHAIN_BIN  the directory of riscv64-unknown-linux-musl-gcc
#                                      (a real path: it is recorded in the flag files)
#
# <build-dir> is created fresh and must lie under $HOME. The CMake build tree is what the
# evidence bundle's flags/kvm_system is read from (scripts/forge-build.sh).

set -euo pipefail

[ $# -eq 2 ] || { echo "usage: $0 <nanokvm-tree> <build-dir>" >&2; exit 2; }
TREE="$(cd "$1" && pwd)"
BUILD="$2"
LITE="$TREE/support/sg2002/maixlite"
TC_BIN="${MAIXLITE_TOOLCHAIN_BIN:-}"

die() {
    echo "[maixlite] ERROR: $*" >&2
    exit 1
}

[ -n "${HOME:-}" ] || die "HOME is unset"
case "$BUILD" in
    "$HOME"/?*) ;;
    *) die "build dir '$BUILD' is not under HOME ($HOME); refusing to clear it" ;;
esac
[ -f "$LITE/CMakeLists.txt" ] || die "$LITE/CMakeLists.txt is missing"
[ -n "$TC_BIN" ] || die "MAIXLITE_TOOLCHAIN_BIN is unset"
[ -x "$TC_BIN/riscv64-unknown-linux-musl-gcc" ] || die "$TC_BIN/riscv64-unknown-linux-musl-gcc is not executable"
for tool in cmake make; do
    command -v "$tool" >/dev/null 2>&1 || die "'$tool' is missing (a skipped build is a failed one)"
done

# The vendored MaixCDK files must still be the upstream bytes (UPSTREAM.sha256).
(cd "$LITE" && sha256sum -c --quiet UPSTREAM.sha256) || die "a vendored MaixCDK file differs from upstream (UPSTREAM.sha256)"

echo "::group::maixlite: configure"
rm -rf "$BUILD"
mkdir -p "$BUILD"
MAIXLITE_TOOLCHAIN_BIN="$TC_BIN" cmake -S "$LITE" -B "$BUILD" -G "Unix Makefiles" \
    -DCMAKE_TOOLCHAIN_FILE="$LITE/toolchain.cmake" -DCMAKE_BUILD_TYPE=MinSizeRel \
    -DCMAKE_EXPORT_COMPILE_COMMANDS=ON
echo "::endgroup::"

echo "::group::maixlite: build kvm_system"
cmake --build "$BUILD" --target kvm_system -- -j"$(nproc)"
echo "::endgroup::"
[ -f "$BUILD/kvm_system" ] || die "the build did not produce $BUILD/kvm_system"

echo "::group::maixlite: flags against the MaixCDK golden"
bash "$TREE/tools/binabi/flags.sh" "$BUILD" "$LITE" "$TREE" || die "flags drift (see above)"
echo "::endgroup::"

# MaixCDK's gen_binary.cmake: ${CMAKE_STRIP} <binary>, no options, for a Release build.
"$TC_BIN/riscv64-unknown-linux-musl-strip" "$BUILD/kvm_system"
mkdir -p "$TREE/kvmapp/kvm_system"
install -m 0755 "$BUILD/kvm_system" "$TREE/kvmapp/kvm_system/kvm_system"
echo "[maixlite] kvm_system staged at kvmapp/kvm_system/kvm_system"
