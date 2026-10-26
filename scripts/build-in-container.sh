#!/bin/bash
#
# Build every riscv64 artifact of a release, from inside the nanokvm-builder
# image (docker/Dockerfile). Not meant to be run on the host: it needs the
# riscv64 musl toolchain and the patched MaixCDK checkout that the image ships.
#
# Produces:
#   kvmapp/kvm_system/kvm_system    kvm_system daemon
#   kvmapp/server/dl_lib/           video libraries, including a fresh libkvm.so
#   server/NanoKVM-Server           Go server (BoringCrypto, RPATH $ORIGIN/dl_lib)
#   tools/kvmgpio/kvmgpio           GPIO line owner (static)
#   tools/nanokvm_update_edid/nanokvm_update_edid   EDID helper (static libc via musl)

set -euo pipefail

# docker/entrypoint sets HOME for the build user. support/sg2002/build resolves
# both MaixCDK and the source tree through ~, so a wrong HOME would send the
# whole build at /root; say so plainly rather than emitting "directory missing".
BUILD_HOME="${BUILD_HOME:-/home/build}"
if [ "${HOME:-}" != "$BUILD_HOME" ]; then
    echo "[ERROR] HOME is '${HOME:-unset}', expected $BUILD_HOME" >&2
    echo "        run this inside the builder image via the Makefile" >&2
    exit 1
fi

MAIXCDK_PATH="$HOME/MaixCDK"
NANOKVM_PATH="$HOME/NanoKVM"

for dir in "$MAIXCDK_PATH" "$NANOKVM_PATH"; do
    if [ ! -d "$dir" ]; then
        echo "[ERROR] $dir not found - is this running inside nanokvm-builder?" >&2
        exit 1
    fi
done

# Check the toolchain up front. builder-image reuses any existing image, so an
# image built before a tooling change silently lacks it - and without this the
# SDK build would run for minutes before server/build.sh failed at the end.
for tool in go riscv64-unknown-linux-musl-gcc patchelf; do
    if ! command -v "$tool" >/dev/null 2>&1; then
        echo "[ERROR] '$tool' is missing from the builder image" >&2
        echo "        the image is out of date; rebuild it with: make rebuild-image" >&2
        exit 1
    fi
done

# The SDK tree is owned by the ids baked into the image (DOCKER_UID/DOCKER_GID),
# while the entrypoint drops to the caller's ids. If they disagree, syncing
# components fails partway through a long build; catch it here instead.
if [ ! -w "$MAIXCDK_PATH/components" ]; then
    echo "[ERROR] $MAIXCDK_PATH/components is not writable by $(id -u):$(id -g)" >&2
    echo "        the image was built for different ids; rebuild it with:" >&2
    echo "        make rebuild-image" >&2
    exit 1
fi

# shellcheck disable=SC1091
. "$MAIXCDK_PATH/bin/activate"

echo "::group::support: sync MaixCDK components"
cd "$NANOKVM_PATH/support/sg2002"
# The image bakes support/sg2002/additional/ into MaixCDK's components at image
# build time, and ./build only re-copies them when components/kvm is missing.
# Refresh unconditionally so a cached image never yields stale libraries.
./build update_lib
echo "::endgroup::"

echo "::group::support: build kvm_system"
./build kvm_system
./build kvm_system add_to_kvmapp
echo "::endgroup::"

# OpenCV from pinned source (scripts/build-deps.sh; the tarball comes from the artifacts
# share via NANOKVM_SRC_MIRROR, sha256-checked). Built BEFORE libkvm.so: the MaixCDK
# opencv component is replaced by support/sg2002/additional/opencv, which compiles and
# links libkvm.so against this build's headers and .so files, and refuses to configure
# without NANOKVM_OPENCV_DIR. The package ships these same .so files.
echo "::group::deps: OpenCV 4.9.0 from pinned source"
: "${NANOKVM_SRC_MIRROR:?NANOKVM_SRC_MIRROR is unset: the OpenCV source comes from the artifacts share (run via scripts/forge-build.sh)}"
bash "$NANOKVM_PATH/scripts/build-deps.sh" "$NANOKVM_PATH/build/deps"
export NANOKVM_OPENCV_DIR="$NANOKVM_PATH/build/deps"
echo "::endgroup::"

echo "::group::support: build kvm_vision (libkvm.so)"
./build kvm_vision
./build kvm_vision add_to_kvmapp
echo "::endgroup::"

# The 12-library MMF link set: libkvm_mmf.so was linked against NANOKVM_MMF_LIB
# (scripts/build-mmf.sh: libraries built from the pinned sophgo-middleware plus that
# pin's closed ISP/3A blobs; support/sg2002/additional/kvm_mmf/CMakeLists.txt). Fail
# closed: without it the link would silently fall back to MaixCDK's v2/lib.
echo "::group::mmf: ship the libraries built from the pinned middleware"
: "${NANOKVM_MMF_LIB:?NANOKVM_MMF_LIB is unset: libkvm_mmf must link the pinned build, not MaixCDK (run via scripts/forge-build.sh)}"
: "${NANOKVM_MIDDLEWARE_SRC:?NANOKVM_MIDDLEWARE_SRC is unset}"
mmf_dl="$NANOKVM_PATH/kvmapp/server/dl_lib"
mmf_set="libsys.so libcvi_bin.so libvpu.so libvenc.so libvdec.so libini.so libisp.so libcvi_bin_isp.so libisp_algo.so libae.so libaf.so libawb.so"
mmf_files=()
for lib in $mmf_set; do
    src="$NANOKVM_MMF_LIB/$lib"
    [ "$lib" = libini.so ] && src="$NANOKVM_MMF_LIB/3rd/libini.so"
    [ -f "$src" ] || { echo "[ERROR] $src is missing from the pinned build" >&2; exit 1; }
    # MaixCDK's dist stages the libraries the component linked; they must be these exact
    # bytes. A different file is a MaixCDK library that sneaked in.
    if [ -f "$mmf_dl/$lib" ] && ! cmp -s "$src" "$mmf_dl/$lib"; then
        echo "[ERROR] $mmf_dl/$lib is not the pinned build ($src); refusing to ship it" >&2
        exit 1
    fi
    install -m 0755 "$src" "$mmf_dl/$lib"
    mmf_files+=("$mmf_dl/$lib")
done
[ -f "$mmf_dl/libkvm_mmf.so" ] || { echo "[ERROR] the build did not stage libkvm_mmf.so" >&2; exit 1; }
# Nothing else may be in dl_lib at this point (OpenCV is added below).
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

# Stage the OpenCV libraries next to the libkvm.so / libkvm_mmf.so the support build just
# put in kvmapp/server/dl_lib, then PROVE libkvm.so came from them (fail closed):
#   1. the MaixCDK build tree of kvm_vision: every compile and link rule that mentions
#      OpenCV points into build/deps, and none into a MaixCDK prebuilt;
#   2. libkvm.so NEEDs exactly our three sonames, and every cv:: symbol it imports is
#      defined by those three libraries;
#   3. what is staged is byte-identical to build-deps.sh's output (nothing from MaixCDK).
# Nothing under server/dl_lib in git is a library we build; see server/dl_lib/README.md.
echo "::group::deps: stage OpenCV 4.9.0 and prove libkvm.so is built against it"
ocv_dir="$NANOKVM_PATH/build/deps"
ocv_set="libopencv_core.so.409 libopencv_imgproc.so.409 libopencv_imgcodecs.so.409"
kvm_so="$NANOKVM_PATH/kvmapp/server/dl_lib/libkvm.so"
mkdir -p "$NANOKVM_PATH/kvmapp/server/dl_lib"
for lib in $ocv_set; do
    [ -f "$ocv_dir/lib/$lib" ] || { echo "[ERROR] $ocv_dir/lib/$lib was not built" >&2; exit 1; }
    if [ -f "$mmf_dl/$lib" ] && ! cmp -s "$ocv_dir/lib/$lib" "$mmf_dl/$lib"; then
        echo "[ERROR] $mmf_dl/$lib is not our OpenCV build ($ocv_dir/lib/$lib); refusing to ship it" >&2
        exit 1
    fi
    install -m 0644 "$ocv_dir/lib/$lib" "$mmf_dl/$lib"
    cmp -s "$ocv_dir/lib/$lib" "$mmf_dl/$lib" || { echo "[ERROR] staged $lib differs from the build" >&2; exit 1; }
done
[ -f "$kvm_so" ] || { echo "[ERROR] the build did not stage libkvm.so" >&2; exit 1; }

# 1. compile and link rules of the kvm_vision build tree
kvm_build="$NANOKVM_PATH/support/sg2002/kvm_vision_test/build"
rules="$(find "$kvm_build" \( -name flags.make -o -name link.txt -o -name build.ninja -o -name compile_commands.json \) -type f)"
[ -n "$rules" ] || { echo "[ERROR] no flags.make/link.txt under $kvm_build: cannot prove where libkvm's OpenCV came from" >&2; exit 1; }
# (find output holds no spaces: the build tree is under $HOME/NanoKVM)
# shellcheck disable=SC2086
rules_ours="$(grep -l -F -e "$ocv_dir/include/opencv4" $rules || true)"
# shellcheck disable=SC2086
rules_ours_lib="$(grep -l -F -e "$ocv_dir/lib/libopencv_core.so.409" $rules || true)"
[ -n "$rules_ours" ] || { echo "[ERROR] no compile rule includes $ocv_dir/include/opencv4" >&2; exit 1; }
[ -n "$rules_ours_lib" ] || { echo "[ERROR] no link rule names $ocv_dir/lib/libopencv_core.so.409" >&2; exit 1; }
# every OpenCV include dir in any rule, and every libopencv path in any link rule, must be ours
# shellcheck disable=SC2086
foreign="$(cat $rules | tr ' ' '\n' \
    | grep -E 'opencv4(_lib|/include)|opencv-mobile|opencv4_install|libopencv_[a-z_]*\.(so|a)' \
    | grep -v -F -e "$ocv_dir/" | sort -u || true)"
if [ -n "$foreign" ]; then
    echo "[ERROR] the kvm_vision build references an OpenCV that is not ours ($ocv_dir):" >&2
    printf '  %s\n' $foreign >&2
    exit 1
fi
echo "[INFO] OpenCV in the kvm_vision build tree ($(printf '%s\n' "$rules" | wc -l | tr -d ' ') rule files):"
echo "  compile rules naming $ocv_dir/include/opencv4: $(printf '%s\n' "$rules_ours" | wc -l | tr -d ' ')"
echo "  link rules naming $ocv_dir/lib/libopencv_core.so.409: $(printf '%s\n' "$rules_ours_lib" | wc -l | tr -d ' ')"
echo "  no reference to a MaixCDK prebuilt OpenCV"

# 2. NEEDED and the cv:: symbols
needed_ocv="$(readelf -d "$kvm_so" | sed -n 's/.*(NEEDED).*\[\(libopencv_[a-z]*\.so\.[0-9]*\)\].*/\1/p' | LC_ALL=C sort -u | tr '\n' ' ')"
echo "[INFO] libkvm.so NEEDED OpenCV: $needed_ocv"
for n in $needed_ocv; do
    case " $ocv_set " in
        *" $n "*) ;;
        *) echo "[ERROR] libkvm.so NEEDs $n, which is not one of ours ($ocv_set)" >&2; exit 1 ;;
    esac
done
case " $needed_ocv " in
    *" libopencv_core.so.409 "*) ;;
    *) echo "[ERROR] libkvm.so does not NEED libopencv_core.so.409" >&2; exit 1 ;;
esac
ocv_defined="$(for lib in $ocv_set; do readelf --dyn-syms --wide "$ocv_dir/lib/$lib"; done \
    | awk '$7 != "UND" && $8 != "" { print $8 }' | sed 's/@.*//' | LC_ALL=C sort -u)"
ocv_wanted="$(readelf --dyn-syms --wide "$kvm_so" | awk '$7 == "UND" && $8 ~ /^_ZN2cv|^_ZNK2cv/ { print $8 }' \
    | sed 's/@.*//' | LC_ALL=C sort -u)"
[ -n "$ocv_wanted" ] || { echo "[ERROR] libkvm.so imports no cv:: symbol: its OpenCV use cannot be checked" >&2; exit 1; }
ocv_missing="$(comm -23 <(printf '%s\n' "$ocv_wanted") <(printf '%s\n' "$ocv_defined"))"
if [ -n "$ocv_missing" ]; then
    echo "[ERROR] libkvm.so imports cv:: symbols our OpenCV libraries do not define:" >&2
    printf '  %s\n' $ocv_missing >&2
    exit 1
fi
echo "[INFO] all $(printf '%s\n' "$ocv_wanted" | wc -l | tr -d ' ') cv:: symbols libkvm.so imports are defined by our libopencv_{core,imgproc,imgcodecs}"
echo "[INFO] shipped OpenCV libraries (sha256, size):"
for lib in $ocv_set; do
    printf '  %s  %s  %s\n' "$(sha256sum "$mmf_dl/$lib" | cut -d' ' -f1)" "$(stat -c %s "$mmf_dl/$lib")" "$lib"
done
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
# link against the one just built. That one leaves the mmf and opencv symbols to
# the other libraries of the package, which are not the link-time ones: the
# server's cgo flags carry -Wl,--allow-shlib-undefined for it. Symbols the
# server binary itself imports stay checked.
echo "::group::server: cross-compile NanoKVM-Server"
for lib in libkvm.so libkvm_mmf.so libopencv_core.so.409 libopencv_imgproc.so.409 \
    libopencv_imgcodecs.so.409 $mmf_set; do
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
