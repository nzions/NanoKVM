#!/bin/bash
#
# Build without MaixCDK (support/sg2002/maixlite), hold the flags to the MaixCDK golden
# (tools/binabi/flags.sh), strip as MaixCDK's gen_binary.cmake did for a Release build, and
# stage:
#   kvm_system       (default) kvmapp/kvm_system/kvm_system
#   kvm_vision_test  kvmapp/server/dl_lib/libkvm.so and libkvm_mmf.so
#
# Usage: scripts/build-maixlite.sh <nanokvm-tree> <build-dir> [kvm_system|kvm_vision_test]
# Environment: MAIXLITE_TOOLCHAIN_BIN  the directory of riscv64-unknown-linux-musl-gcc
#                                      (a real path: it is recorded in the flag files)
#              kvm_vision_test also needs NANOKVM_MIDDLEWARE_SRC, NANOKVM_MMF_LIB and
#              NANOKVM_MMF_UAPI (scripts/forge-build.sh exports them)
#
# <build-dir> is created fresh and must lie under $HOME. The CMake build tree is what the
# evidence bundle's flags/<project> is read from (scripts/forge-build.sh).

set -euo pipefail

[ $# -eq 2 ] || [ $# -eq 3 ] || { echo "usage: $0 <nanokvm-tree> <build-dir> [kvm_system|kvm_vision_test]" >&2; exit 2; }
TREE="$(cd "$1" && pwd)"
BUILD="$2"
PROJECT="${3:-kvm_system}"
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
case "$PROJECT" in
    kvm_system) ;;
    kvm_vision_test)
        for v in NANOKVM_MIDDLEWARE_SRC NANOKVM_MMF_LIB NANOKVM_MMF_UAPI; do
            [ -n "${!v:-}" ] || die "$v is unset: libkvm_mmf must build against the pinned middleware"
        done
        ;;
    *) die "unknown project '$PROJECT' (kvm_system or kvm_vision_test)" ;;
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
    -DCMAKE_EXPORT_COMPILE_COMMANDS=ON -DMAIXLITE_PROJECT="$PROJECT"
echo "::endgroup::"

STRIP="$TC_BIN/riscv64-unknown-linux-musl-strip"
if [ "$PROJECT" = kvm_system ]; then
    echo "::group::maixlite: build kvm_system"
    cmake --build "$BUILD" --target kvm_system -- -j"$(nproc)"
    echo "::endgroup::"
    [ -f "$BUILD/kvm_system" ] || die "the build did not produce $BUILD/kvm_system"

    echo "::group::maixlite: flags against the MaixCDK golden"
    bash "$TREE/tools/binabi/flags.sh" "$BUILD" "$LITE" "$TREE" kvm_system || die "flags drift (see above)"
    echo "::endgroup::"

    # MaixCDK's gen_binary.cmake: ${CMAKE_STRIP} <binary>, no options, for a Release build.
    "$STRIP" "$BUILD/kvm_system"
    mkdir -p "$TREE/kvmapp/kvm_system"
    install -m 0755 "$BUILD/kvm_system" "$TREE/kvmapp/kvm_system/kvm_system"
    echo "[maixlite] kvm_system staged at kvmapp/kvm_system/kvm_system"
else
    echo "::group::maixlite: build libkvm_mmf.so and libkvm.so"
    cmake --build "$BUILD" --target kvm_mmf kvm -- -j"$(nproc)"
    echo "::endgroup::"
    [ -f "$BUILD/kvm_mmf/libkvm_mmf.so" ] || die "the build did not produce $BUILD/kvm_mmf/libkvm_mmf.so"
    [ -f "$BUILD/kvm/libkvm.so" ] || die "the build did not produce $BUILD/kvm/libkvm.so"

    echo "::group::maixlite: flags against the MaixCDK golden"
    bash "$TREE/tools/binabi/flags.sh" "$BUILD" "$LITE" "$TREE" kvm_vision_test || die "flags drift (see above)"
    echo "::endgroup::"

    # MaixCDK's gen_binary.cmake stripped the project's dynamic libraries too (${CMAKE_STRIP}).
    "$STRIP" "$BUILD/kvm_mmf/libkvm_mmf.so" "$BUILD/kvm/libkvm.so"
    mkdir -p "$TREE/kvmapp/server/dl_lib"
    install -m 0755 "$BUILD/kvm_mmf/libkvm_mmf.so" "$TREE/kvmapp/server/dl_lib/libkvm_mmf.so"
    install -m 0755 "$BUILD/kvm/libkvm.so" "$TREE/kvmapp/server/dl_lib/libkvm.so"
    echo "[maixlite] libkvm_mmf.so and libkvm.so staged at kvmapp/server/dl_lib/"
fi
