#!/bin/bash
#
# Build the open MMF libraries libkvm_mmf links, from the PINNED sophgo-middleware source,
# with the riscv64 musl (T-Head) cross compiler and the pin's own GNU make files
# (v2/Makefile.param + v2/modules/*/Makefile). Cross-compiled; nothing is run.
#
#   built here   libsys libcvi_bin libvpu (VI/VO/VPSS/RGN/GDC are compiled into it)
#                libvenc libvdec 3rd/libini
#   pin blobs    libisp libcvi_bin_isp libisp_algo libae libaf libawb: closed, taken from
#                v2/modules/isp/lib/musl_riscv64 of the SAME pin (no source anywhere)
#
# Usage: build-mmf.sh <mw-src> <outdir>
#   <mw-src>   scripts/fetch-middleware.sh's <outdir> (the tree is <mw-src>/v2)
#   <outdir>   absolute, under $HOME. Writes:
#                <outdir>/lib/*.so, <outdir>/lib/3rd/libini.so   the 12-library link set
#                <outdir>/MANIFEST                               sha256 + origin of each
#                <outdir>/uapi                                   ";"-list for NANOKVM_MMF_UAPI
#                <outdir>/cvi                                    NANOKVM_MMF_CVI include root
#
# Other inputs, all from $NANOKVM_SRC_MIRROR (the artifacts share's src/), each checked
# against the sha256 below, fail closed:
#   osdrv-cbd9c582.tar.gz        scpcom/sophgo-osdrv cbd9c582: the v2 kernel uapi headers
#                                (base sys vi vpss dwa rgn vo ... : libsys, libvpu, libcvi_bin)
#   osdrv2-1b3a4964.tar.gz       sophgo/osdrv 1b3a4964: the v1 uapi headers of the vcodec
#                                driver (libvenc, libvdec). Same tarballs the 7.3 kernel
#                                build compiles its drivers from.
#   json-c-0.15-20200726.tar.gz  json-c headers, for the cvi_json-c declarations libcvi_bin
#                                needs (scripts/gen-cvi-headers.sh)
# The pin itself carries no kernel uapi headers.
#
# Needs on PATH: riscv64-unknown-linux-musl-gcc/ld/ar/strip, make, tar, sha256sum, readelf.

set -euo pipefail

OSDRV_V2_TARBALL=osdrv-cbd9c582.tar.gz
OSDRV_V2_SHA256=fb40b511ca4bd30534a42371fc825a657985cbd8f3c0b95162657d32dc7f3d14
OSDRV_V1_TARBALL=osdrv2-1b3a4964.tar.gz
OSDRV_V1_SHA256=9913c091e732e301f4dfa5386a91975e33959796546b77d50ee329a74e2743f3
JSONC_TARBALL=json-c-0.15-20200726.tar.gz
JSONC_SHA256=4ba9a090a42cf1e12b84c64e4464bb6fb893666841d5843cc5bef90774028882
MW_COMMIT=4e231ef5967f8f7d77cecd65d9e2052a6f92f18c

die() { echo "[build-mmf] ERROR: $*" >&2; exit 1; }
log() { echo "[build-mmf] $*"; }

[ $# -eq 2 ] || die "usage: $0 <mw-src> <outdir>"
MW="$1"; OUT="$2"
HERE="$(cd "$(dirname "$0")" && pwd)"
case "$OUT" in /*) ;; *) die "outdir must be absolute: $OUT" ;; esac
[ -n "${HOME:-}" ] || die "HOME is unset"
case "$OUT" in "$HOME"/*) ;; *) die "refusing to write $OUT outside HOME ($HOME)" ;; esac
[ -n "${NANOKVM_SRC_MIRROR:-}" ] || die "NANOKVM_SRC_MIRROR is unset"
MW="$(realpath "$MW")"     # the pin's makefiles build their targets from realpath
[ "$(cat "$MW/.pin" 2>/dev/null)" = "$MW_COMMIT" ] || die "$MW does not carry the pin $MW_COMMIT"

CROSS=riscv64-unknown-linux-musl-
CC="${CROSS}gcc"
for tool in "$CC" "${CROSS}ld" "${CROSS}ar" "${CROSS}strip" make tar sha256sum readelf; do
    command -v "$tool" >/dev/null 2>&1 || die "'$tool' is missing"
done
SYSROOT="$($CC -print-sysroot)"
[ -d "$SYSROOT" ] || die "the compiler reports no sysroot ($SYSROOT)"

fetch_check() { # tarball sha256 -> prints the path
    local f="$NANOKVM_SRC_MIRROR/$1" got
    [ -f "$f" ] || die "$f is missing from the artifacts share (sha256 $2)"
    got="$(sha256sum "$f" | cut -d' ' -f1)"
    [ "$got" = "$2" ] || die "$1: sha256 $got, pinned $2"
    echo "[build-mmf] $1 sha256 ok" >&2
    printf '%s' "$f"
}

rm -rf "$OUT"
mkdir -p "$OUT/lib/3rd" "$OUT/src" "$OUT/cfg"

# --- kernel uapi headers ----------------------------------------------------------
tgz="$(fetch_check "$OSDRV_V2_TARBALL" "$OSDRV_V2_SHA256")"
mkdir -p "$OUT/osdrv-v2"
tar -xzf "$tgz" -C "$OUT/osdrv-v2" --strip-components=1 --wildcards '*/interdrv/v2/include/*'
V2="$OUT/osdrv-v2/interdrv/v2/include"
tgz="$(fetch_check "$OSDRV_V1_TARBALL" "$OSDRV_V1_SHA256")"
mkdir -p "$OUT/osdrv-v1"
tar -xzf "$tgz" -C "$OUT/osdrv-v1" --strip-components=1 --wildcards '*/interdrv/include/*'
V1="$OUT/osdrv-v1/interdrv/include"
# CHIP_CODE=mars CHIP_ARCH=CV181X, as the kernel build: v2 reaches cv181x through chip/mars.
[ "$(cd "$V2/chip/mars" && pwd -P)" = "$(cd "$V2/chip/cv181x" && pwd -P)" ] || die "v2 chip/mars is not chip/cv181x"
V2_UAPI_A="$V2/common/uapi"; V2_UAPI_B="$V2/chip/mars/uapi"
V1_UAPI_A="$V1/common/uapi"; V1_UAPI_B="$V1/chip/cv181x/uapi"
for d in "$V2_UAPI_A/linux" "$V2_UAPI_B/linux" "$V1_UAPI_A/linux" "$V1_UAPI_B/linux"; do
    [ -d "$d" ] || die "$d is missing from the osdrv tarball"
done
printf '%s;%s' "$V2_UAPI_A" "$V2_UAPI_B" > "$OUT/uapi"

# --- cvi_json-c / cvi_miniz declarations for libcvi_bin ---------------------------------
tgz="$(fetch_check "$JSONC_TARBALL" "$JSONC_SHA256")"
mkdir -p "$OUT/src/json-c"
tar -xzf "$tgz" -C "$OUT/src/json-c" --strip-components=1
bash "$HERE/gen-cvi-headers.sh" "$OUT/src/json-c" "$OUT/cvi"

# --- build with the pin's own make files -----------------------------------------------
: > "$OUT/cfg/.config"      # Makefile.param includes $(BUILD_PATH)/.config; no option is needed
JOBS="$(nproc)"
LIBDIR="$MW/v2/lib"
mkdir -p "$LIBDIR/3rd"
rm -f "$LIBDIR"/*.so "$LIBDIR"/*.a "$LIBDIR/3rd"/*.so "$LIBDIR/3rd"/*.a

# Command-line variables override the pin's Makefile.param. KERNEL_INC is spliced as
# -I$(KERNEL_INC), hence the inner -I. CVI_TARGET_PACKAGES_INCLUDE lands in CFLAGS.
SHIM="$HERE/mmf-shim"
mk() { # module-dir target kernel-inc extra-cflags
    local dir="$1" target="$2" kinc="$3" extra="$4"
    log "make $dir: $(basename "$target")"
    # cd, not make -C: the modules' makefiles use $(PWD)
    ( cd "$MW/v2/$dir" && make -j"$JOBS" "$target" \
        BUILD_PATH="$OUT/cfg" CHIP_ARCH=CV181X SDK_VER=musl_riscv64 \
        CROSS_COMPILE_MUSL_RISCV64="$CROSS" SYSROOT="$SYSROOT" \
        PROJECT_FULLNAME=nanokvm KERNEL_DIR="$OUT/kernel-unused" \
        KERNEL_INC="${kinc%%|*} -I${kinc##*|}" \
        CVI_TARGET_PACKAGES_INCLUDE="-I$SHIM $extra" )
}
V2INC="$V2_UAPI_A|$V2_UAPI_B"
V1INC="$V1_UAPI_A|$V1_UAPI_B"
# libsys first: libvenc links -lsys. The vcodec headers (libvenc, libvdec) are the v1 set
# of the driver they ioctl; everything else is the v2 set (tools/abicheck).
mk 3rdparty/inih "$LIBDIR/3rd/libini.so" "$V2INC" ""
mk modules/sys "$LIBDIR/libsys.so" "$V2INC" ""
mk modules/bin "$LIBDIR/libcvi_bin.so" "$V2INC" "-idirafter $OUT/cvi/include/cvi_json-c -idirafter $OUT/cvi/include/cvi_miniz"
mk modules/vpu "$LIBDIR/libvpu.so" "$V2INC" ""
mk modules/venc "$LIBDIR/libvenc.so" "$V1INC" "-D__CV181X__"
mk modules/vdec "$LIBDIR/libvdec.so" "$V1INC" "-D__CV181X__"

# --- stage the 12-library link set ------------------------------------------------------
BLOBS="$MW/v2/modules/isp/lib/musl_riscv64"
OURS="libsys.so libcvi_bin.so libvpu.so libvenc.so libvdec.so"
PINBLOBS="libisp.so libcvi_bin_isp.so libisp_algo.so libae.so libaf.so libawb.so"
: > "$OUT/MANIFEST"
for l in $OURS 3rd/libini.so; do
    [ -f "$LIBDIR/$l" ] || die "the build did not produce $l"
    # Provenance before the strip: the object files carry the pin's source paths (-g).
    n="$(command grep -a -c "sophgo-middleware-4e231ef" "$LIBDIR/$l" || true)"
    [ "$n" -gt 0 ] || die "$l carries no path of the pinned source: not a build of this tree"
    install -m 0755 "$LIBDIR/$l" "$OUT/lib/$l"
    "${CROSS}strip" --strip-unneeded "$OUT/lib/$l"
    printf '%s  %s  built-from=sophgo-middleware@%s source-path-strings=%s\n' \
        "$(sha256sum "$OUT/lib/$l" | cut -d' ' -f1)" "$l" "$MW_COMMIT" "$n" >> "$OUT/MANIFEST"
done
for l in $PINBLOBS; do
    [ -f "$BLOBS/$l" ] || die "the pin has no $BLOBS/$l"
    install -m 0755 "$BLOBS/$l" "$OUT/lib/$l"
    printf '%s  %s  closed-blob-from=sophgo-middleware@%s v2/modules/isp/lib/musl_riscv64\n' \
        "$(sha256sum "$OUT/lib/$l" | cut -d' ' -f1)" "$l" "$MW_COMMIT" >> "$OUT/MANIFEST"
done
echo "[build-mmf] link set (sha256, name, origin):"; cat "$OUT/MANIFEST"

# --- closure: NEEDED and undefined symbols of the set ---------------------------------
libc="$($CC -print-file-name=libc.so)"; cpp="$($CC -print-file-name=libstdc++.so.6)"
gcc_s="$($CC -print-file-name=libgcc_s.so.1)"
# libsys leaves __atomic_compare_exchange_1 (riscv has no byte CAS) undefined, exactly as the
# MaixCDK build did; libkvm.so NEEDs libatomic.so.1, which satisfies it from the global scope.
atomic="$($CC -print-file-name=libatomic.so.1)"
for f in "$libc" "$cpp" "$gcc_s" "$atomic"; do [ -f "$f" ] || die "toolchain has no $f"; done
set_files=()
for l in $OURS $PINBLOBS 3rd/libini.so; do set_files+=("$OUT/lib/$l"); done
bash "$HERE/check-mmf-closure.sh" --lib "${set_files[@]}" --system "$libc" "$cpp" "$gcc_s" "$atomic"
log "done: $OUT/lib"
