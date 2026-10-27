#!/bin/bash
#
# Build every riscv64 artifact of a release, from inside the nanokvm-builder
# image (docker/Dockerfile). Not meant to be run on the host: it needs the
# riscv64 musl toolchain that the image ships. MaixCDK is not used: kvm_system and the two
# libraries are built by support/sg2002/maixlite (scripts/build-maixlite.sh).
#
# Produces:
#   kvmapp/kvm_system/kvm_system    kvm_system daemon
#   kvmapp/server/dl_lib/           video libraries, including a fresh libkvm.so
#   server/NanoKVM-Server           Go server (BoringCrypto, RPATH $ORIGIN/dl_lib)
#   tools/kvmgpio/kvmgpio           GPIO line owner (static)
#   tools/nanokvm_update_edid/nanokvm_update_edid   EDID helper (static libc via musl)

set -euo pipefail

# docker/entrypoint sets HOME for the build user. The source tree is resolved through
# ~, so a wrong HOME would send the whole build at /root; say so plainly rather than
# emitting "directory missing".
BUILD_HOME="${BUILD_HOME:-/home/build}"
if [ "${HOME:-}" != "$BUILD_HOME" ]; then
    echo "[ERROR] HOME is '${HOME:-unset}', expected $BUILD_HOME" >&2
    echo "        run this inside the builder image via the Makefile" >&2
    exit 1
fi

NANOKVM_PATH="$HOME/NanoKVM"

for dir in "$NANOKVM_PATH"; do
    if [ ! -d "$dir" ]; then
        echo "[ERROR] $dir not found - is this running inside nanokvm-builder?" >&2
        exit 1
    fi
done

# Check the toolchain up front. builder-image reuses any existing image, so an
# image built before a tooling change silently lacks it - and without this the
# SDK build would run for minutes before server/build.sh failed at the end.
for tool in go riscv64-unknown-linux-musl-gcc patchelf cmake make; do
    if ! command -v "$tool" >/dev/null 2>&1; then
        echo "[ERROR] '$tool' is missing from the builder image" >&2
        echo "        the image is out of date; rebuild it with: make rebuild-image" >&2
        exit 1
    fi
done

# The riscv64 programs are built without MaixCDK: support/sg2002/maixlite, the vendored subset
# of the MaixCDK sources they compiled, with the same flags and link lines
# (scripts/build-maixlite.sh holds them to the MaixCDK goldens). The compiler is the
# image's.
MAIXLITE_TOOLCHAIN_BIN="${MAIXLITE_TOOLCHAIN_BIN:-$(cd -P "$(dirname "$(command -v riscv64-unknown-linux-musl-gcc)")" && pwd)}"
export MAIXLITE_TOOLCHAIN_BIN

echo "::group::support: build kvm_system (maixlite)"
MAIXLITE_BUILD="${MAIXLITE_BUILD:-$HOME/maixlite-build}"
bash "$NANOKVM_PATH/scripts/build-maixlite.sh" "$NANOKVM_PATH" "$MAIXLITE_BUILD" kvm_system
echo "::endgroup::"

# libkvm_mmf.so links the 6-library MMF set in NANOKVM_MMF_LIB (scripts/build-mmf.sh:
# libraries built from the pinned sophgo-middleware, no closed ISP/3A blob;
# support/sg2002/maixlite/libs/kvm_mmf/CMakeLists.txt). Fail closed: without it there is
# nothing to link.
: "${NANOKVM_MMF_LIB:?NANOKVM_MMF_LIB is unset: libkvm_mmf must link the pinned build (run via scripts/forge-build.sh)}"
: "${NANOKVM_MIDDLEWARE_SRC:?NANOKVM_MIDDLEWARE_SRC is unset}"
: "${NANOKVM_MMF_UAPI:?NANOKVM_MMF_UAPI is unset}"

echo "::group::support: build libkvm_mmf.so and libkvm.so (maixlite)"
MAIXLITE_LIBS_BUILD="${MAIXLITE_LIBS_BUILD:-$HOME/maixlite-libs-build}"
bash "$NANOKVM_PATH/scripts/build-maixlite.sh" "$NANOKVM_PATH" "$MAIXLITE_LIBS_BUILD" kvm_vision_test
echo "::endgroup::"

echo "::group::mmf: ship the libraries built from the pinned middleware"
mmf_dl="$NANOKVM_PATH/kvmapp/server/dl_lib"
mmf_set="libsys.so libcvi_bin.so libvpu.so libvenc.so libvdec.so libini.so"
mmf_files=()
for lib in $mmf_set; do
    src="$NANOKVM_MMF_LIB/$lib"
    [ "$lib" = libini.so ] && src="$NANOKVM_MMF_LIB/3rd/libini.so"
    [ -f "$src" ] || { echo "[ERROR] $src is missing from the pinned build" >&2; exit 1; }
    # Nothing but libkvm.so and libkvm_mmf.so is staged in dl_lib so far; if a library is
    # already there it must be these exact bytes. A different file sneaked in.
    if [ -f "$mmf_dl/$lib" ] && ! cmp -s "$src" "$mmf_dl/$lib"; then
        echo "[ERROR] $mmf_dl/$lib is not the pinned build ($src); refusing to ship it" >&2
        exit 1
    fi
    install -m 0755 "$src" "$mmf_dl/$lib"
    mmf_files+=("$mmf_dl/$lib")
done
[ -f "$mmf_dl/libkvm_mmf.so" ] || { echo "[ERROR] the build did not stage libkvm_mmf.so" >&2; exit 1; }
# Nothing else may be in dl_lib at this point.
for f in "$mmf_dl"/*; do
    case " $mmf_set libkvm.so libkvm_mmf.so " in
        *" $(basename "$f") "*) ;;
        *) echo "[ERROR] unexpected library in kvmapp/server/dl_lib: $(basename "$f")" >&2; exit 1 ;;
    esac
done
mmf_cc=riscv64-unknown-linux-musl-gcc
bash "$NANOKVM_PATH/scripts/check-mmf-closure.sh" --lib "${mmf_files[@]}" "$mmf_dl/libkvm_mmf.so" \
    --system "$($mmf_cc -print-file-name=libc.so)" "$($mmf_cc -print-file-name=libstdc++.so.6)" \
    "$($mmf_cc -print-file-name=libgcc_s.so.1)" "$($mmf_cc -print-file-name=libatomic.so.1)"
echo "[INFO] shipped MMF libraries right after staging (sha256, size):"
for lib in $mmf_set; do
    printf '  %s  %s  %s\n' "$(sha256sum "$mmf_dl/$lib" | cut -d' ' -f1)" "$(stat -c %s "$mmf_dl/$lib")" "$lib"
done
echo "[INFO] libkvm_mmf.so NEEDED (readelf -d), build log of the pinned MMF link:"
readelf -d "$mmf_dl/libkvm_mmf.so" | command grep -E 'NEEDED|SONAME|RPATH|RUNPATH'
echo "::endgroup::"

# libkvm.so carries no OpenCV: the vision component is a NanoKVM cut of MaixCDK's
# (support/sg2002/additional/vision) and the camera path is MMF hardware only. PROVE it
# (fail closed): no libopencv* NEEDED entry, no imported cv:: symbol, no OpenCV library
# staged in kvmapp/server/dl_lib (the unexpected-library check above already refuses one).
# Nothing under server/dl_lib in git is a library we build; see server/dl_lib/README.md.
echo "::group::deps: prove libkvm.so has no OpenCV"
kvm_so="$NANOKVM_PATH/kvmapp/server/dl_lib/libkvm.so"
[ -f "$kvm_so" ] || { echo "[ERROR] the build did not stage libkvm.so" >&2; exit 1; }
kvm_dyn="$(readelf -d "$kvm_so")" || { echo "[ERROR] readelf -d failed on $kvm_so" >&2; exit 1; }
kvm_syms="$(readelf --dyn-syms --wide "$kvm_so")" || { echo "[ERROR] readelf --dyn-syms failed on $kvm_so" >&2; exit 1; }
[ "$(printf '%s\n' "$kvm_syms" | awk '$7 == "UND" { n++ } END { print n + 0 }')" -gt 0 ] || { echo "[ERROR] libkvm.so has no undefined dynamic symbol: the symbol check cannot see anything" >&2; exit 1; }
needed_all="$(printf '%s\n' "$kvm_dyn" | sed -n 's/.*(NEEDED).*\[\(.*\)\].*/\1/p' | LC_ALL=C sort -u)"
needed_ocv="$(printf '%s\n' "$needed_all" | grep -i 'opencv' || true)"
cv_syms="$(printf '%s\n' "$kvm_syms" | awk '$7 == "UND" && $8 ~ /^_ZN2cv|^_ZNK2cv|^_ZN6cv|opencv/ { print $8 }' | sed 's/@.*//' | LC_ALL=C sort -u)"
echo "[INFO] libkvm.so NEEDED: $(printf '%s' "$needed_all" | tr '\n' ' ')"
if [ -n "$needed_ocv" ]; then
    echo "[ERROR] libkvm.so NEEDs OpenCV: $needed_ocv" >&2
    exit 1
fi
if [ -n "$cv_syms" ]; then
    echo "[ERROR] libkvm.so imports cv:: symbols:" >&2
    printf '  %s\n' $cv_syms >&2
    exit 1
fi
ocv_staged="$(find "$NANOKVM_PATH/kvmapp/server/dl_lib" "$NANOKVM_PATH/server/dl_lib" -name 'libopencv*' || true)"
if [ -n "$ocv_staged" ]; then
    echo "[ERROR] OpenCV libraries staged for the package:" >&2
    printf '  %s\n' $ocv_staged >&2
    exit 1
fi
echo "[INFO] no OpenCV: libkvm.so has no libopencv NEEDED entry, imports no cv:: symbol, and no libopencv file is staged"
echo "::endgroup::"

# GPIO helpers shipped under system/tool/. kvmgpio owns the held GPIO outputs by
# DT line name (S15kvmhwd starts it). nanokvm_update_edid is rebuilt because its
# hdmi-rst reset goes through kvm_gpio.h now; the tracked prebuilt predates it.
# Static, the same musl cross compiler as everything else.
echo "::group::tools: build kvmgpio and nanokvm_update_edid"
riscv64-unknown-linux-musl-gcc -static -O2 -Wall -Wextra \
    -o "$NANOKVM_PATH/tools/kvmgpio/kvmgpio" "$NANOKVM_PATH/tools/kvmgpio/kvmgpio.c"
riscv64-unknown-linux-musl-gcc -march=rv64gc -O2 \
    -o "$NANOKVM_PATH/tools/nanokvm_update_edid/nanokvm_update_edid" \
    "$NANOKVM_PATH/tools/nanokvm_update_edid/nanokvm_update_edid.c" \
    "$NANOKVM_PATH/support/sg2002/additional/kvm/src/lt6911.c"
echo "::endgroup::"

# NanoKVM-Server links libkvm.so by name from server/dl_lib (-L../dl_lib -lkvm).
# The tracked copy there predates every export added since (kvmv_hdmi_wait), so
# link against the one just built. That one leaves the mmf symbols to
# the other libraries of the package, which are not the link-time ones: the
# server's cgo flags carry -Wl,--allow-shlib-undefined for it. Symbols the
# server binary itself imports stay checked.
echo "::group::server: cross-compile NanoKVM-Server"
for lib in libkvm.so libkvm_mmf.so $mmf_set; do
    [ -f "$NANOKVM_PATH/kvmapp/server/dl_lib/$lib" ] \
        || { echo "[ERROR] the build did not stage $lib" >&2; exit 1; }
    cp -f "$NANOKVM_PATH/kvmapp/server/dl_lib/$lib" "$NANOKVM_PATH/server/dl_lib/$lib"
done
cd "$NANOKVM_PATH/server"
./build.sh
echo "::endgroup::"

# support/sg2002/build reports "Build Error!" without a non-zero exit status for
# some failure paths, so assert on the artifacts rather than trusting $?.
echo "[INFO] verifying artifacts"
missing=0
for artifact in \
    "$NANOKVM_PATH/kvmapp/kvm_system/kvm_system" \
    "$NANOKVM_PATH/kvmapp/server/dl_lib/libkvm.so" \
    "$NANOKVM_PATH/server/NanoKVM-Server"
do
    if [ -f "$artifact" ]; then
        echo "  ok      ${artifact#$NANOKVM_PATH/}"
    else
        echo "  MISSING ${artifact#$NANOKVM_PATH/}" >&2
        missing=1
    fi
done

if [ "$missing" -ne 0 ]; then
    echo "[ERROR] build did not produce all expected artifacts" >&2
    exit 1
fi

# NanoKVM-Server is cgo-linked against server/dl_lib/libkvm.so (server/common/
# kvm_vision.go: -L../dl_lib -lkvm), which was just replaced by the freshly built
# one, and the package ships that same build. Confirm the shipped library still
# exports every symbol the binary actually imports from it, so a dropped symbol
# cannot first show up as a crash on a real device.
echo "[INFO] verifying shipped libkvm.so satisfies NanoKVM-Server"
if ! command -v readelf >/dev/null 2>&1; then
    echo "[WARN] readelf unavailable, skipping ABI check"
else
    exported() {
        readelf --dyn-syms --wide "$1" 2>/dev/null \
            | awk '$7 != "UND" && $8 != "" { print $8 }' \
            | sed 's/@.*//' | LC_ALL=C sort -u
    }
    imported() {
        readelf --dyn-syms --wide "$1" 2>/dev/null \
            | awk '$7 == "UND" && $8 != "" { print $8 }' \
            | sed 's/@.*//' | LC_ALL=C sort -u
    }

    linked_lib="$NANOKVM_PATH/server/dl_lib/libkvm.so"
    shipped_lib="$NANOKVM_PATH/kvmapp/server/dl_lib/libkvm.so"
    server_bin="$NANOKVM_PATH/server/NanoKVM-Server"

    # Symbols the binary imports that the link-time library provides, i.e. the
    # ones libkvm.so is actually responsible for at runtime.
    needed=$(comm -12 <(imported "$server_bin") <(exported "$linked_lib"))

    if [ -z "$needed" ]; then
        echo "[WARN] no libkvm symbols resolved; skipping ABI check"
    else
        absent=$(comm -23 <(printf '%s\n' "$needed") <(exported "$shipped_lib"))
        if [ -n "$absent" ]; then
            echo "[ERROR] shipped libkvm.so is missing symbols NanoKVM-Server needs:" >&2
            printf '  %s\n' $absent >&2
            exit 1
        fi
        echo "  ok      $(printf '%s\n' "$needed" | wc -l | tr -d ' ') libkvm symbols resolve"
    fi
fi

echo "[DONE] riscv64 artifacts built"
