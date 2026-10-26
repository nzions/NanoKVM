#!/bin/bash
#
# Build the third-party shared libraries of the video stack from PINNED source,
# with the riscv64 musl (T-Head) cross compiler that builds everything else.
#
#   libopencv_core.so.409  libopencv_imgproc.so.409  libopencv_imgcodecs.so.409
#       OpenCV 4.9.0 (core, imgproc, imgcodecs; libjpeg-turbo, libpng and zlib are
#       OpenCV's bundled copies, linked in). MaixCDK only ships a prebuilt of
#       these, so they are built here instead of being taken from git.
#
# Usage: build-deps.sh <outdir>      libraries land in <outdir>/lib
#
# Every source is downloaded once, checked against the sha256 below and refused
# on any mismatch: no unpinned fetch, no fallback. FreeType and HarfBuzz are not
# here: MaixCDK builds them from its own pinned sources and libkvm.so embeds them.
# Needs on PATH: riscv64-unknown-linux-musl-gcc/g++, cmake, make, curl, tar.

set -euo pipefail

OPENCV_VERSION=4.9.0
OPENCV_URL="https://github.com/opencv/opencv/archive/refs/tags/$OPENCV_VERSION.tar.gz"
OPENCV_SHA256=ddf76f9dffd322c7c3cb1f721d0887f62d747b82059342213138dc190f28bc6c

# The flags MaixCDK builds libkvm.so with (support/sg2002 build log).
ARCH_FLAGS="-mcpu=c906fdv -march=rv64imafdcv0p7xthead -mcmodel=medany -mabi=lp64d"

die() { echo "[build-deps] ERROR: $*" >&2; exit 1; }
log() { echo "[build-deps] $*"; }

[ $# -eq 1 ] || die "usage: $0 <outdir>"
OUT="$1"
case "$OUT" in
    /*) ;;
    *) die "outdir must be absolute: $OUT" ;;
esac
[ -n "${HOME:-}" ] || die "HOME is unset"

CC=riscv64-unknown-linux-musl-gcc
CXX=riscv64-unknown-linux-musl-g++
for tool in "$CC" "$CXX" cmake make curl tar sha256sum readelf; do
    command -v "$tool" >/dev/null 2>&1 || die "'$tool' is missing"
done
STRIP=riscv64-unknown-linux-musl-strip
command -v "$STRIP" >/dev/null 2>&1 || die "'$STRIP' is missing"

DL="$HOME/deps-dl"
SRC="$OUT/src"
mkdir -p "$DL" "$SRC" "$OUT/lib"
JOBS="$(nproc)"

fetch() { # name url sha256 -> $DL/<name>.tar.gz, verified
    local name="$1" url="$2" sha="$3" file="$DL/$1.tar.gz"
    if [ ! -f "$file" ] || [ "$(sha256sum "$file" | cut -d' ' -f1)" != "$sha" ]; then
        log "downloading $url"
        curl -fsSL --retry 3 -o "$file.part" "$url"
        mv "$file.part" "$file"
    fi
    local got
    got="$(sha256sum "$file" | cut -d' ' -f1)"
    [ "$got" = "$sha" ] || die "$name: sha256 $got, pinned $sha"
    log "$name sha256 ok"
}

# soname_ok <lib> <soname>: the file must declare exactly that SONAME.
soname_ok() {
    readelf -d "$1" | grep -q "Library soname: \[$2\]" || die "$1 has no SONAME $2"
}

# --- OpenCV ----------------------------------------------------------------------
fetch opencv "$OPENCV_URL" "$OPENCV_SHA256"
tar -xzf "$DL/opencv.tar.gz" -C "$SRC"
OCV_SRC="$SRC/opencv-$OPENCV_VERSION"
[ -d "$OCV_SRC/modules/core" ] || die "opencv source tree missing after extraction"
OCV_BUILD="$OUT/opencv-build"
rm -rf "$OCV_BUILD"
log "configuring OpenCV $OPENCV_VERSION"
cmake -S "$OCV_SRC" -B "$OCV_BUILD" -G "Unix Makefiles" \
    -DCMAKE_SYSTEM_NAME=Linux -DCMAKE_SYSTEM_PROCESSOR=riscv64 \
    -DCMAKE_C_COMPILER="$(command -v "$CC")" -DCMAKE_CXX_COMPILER="$(command -v "$CXX")" \
    -DCMAKE_C_FLAGS="$ARCH_FLAGS" -DCMAKE_CXX_FLAGS="$ARCH_FLAGS" \
    -DCMAKE_TRY_COMPILE_TARGET_TYPE=STATIC_LIBRARY \
    -DCMAKE_BUILD_TYPE=MinSizeRel -DCMAKE_SKIP_RPATH=ON \
    -DBUILD_SHARED_LIBS=ON -DBUILD_LIST=core,imgproc,imgcodecs \
    -DBUILD_ZLIB=ON -DBUILD_PNG=ON -DBUILD_JPEG=ON -DWITH_PNG=ON -DWITH_JPEG=ON \
    -DWITH_TIFF=OFF -DWITH_WEBP=OFF -DWITH_OPENJPEG=OFF -DWITH_JASPER=OFF -DWITH_OPENEXR=OFF \
    -DWITH_IPP=OFF -DWITH_ITT=OFF -DWITH_PROTOBUF=OFF -DBUILD_PROTOBUF=OFF \
    -DWITH_FFMPEG=OFF -DWITH_GSTREAMER=OFF -DWITH_GTK=OFF -DWITH_V4L=OFF -DWITH_OPENCL=OFF \
    -DWITH_EIGEN=OFF -DWITH_LAPACK=OFF -DWITH_GDAL=OFF -DWITH_OPENMP=ON -DWITH_PTHREADS_PF=ON \
    -DCPU_BASELINE= -DCPU_DISPATCH= \
    -DENABLE_PRECOMPILED_HEADERS=OFF -DBUILD_TESTS=OFF -DBUILD_PERF_TESTS=OFF \
    -DBUILD_EXAMPLES=OFF -DBUILD_DOCS=OFF -DBUILD_opencv_apps=OFF -DBUILD_opencv_python3=OFF \
    -DBUILD_JAVA=OFF -DOPENCV_GENERATE_PKGCONFIG=OFF
log "building OpenCV core, imgproc, imgcodecs ($JOBS jobs)"
cmake --build "$OCV_BUILD" --parallel "$JOBS" --target opencv_core opencv_imgproc opencv_imgcodecs

for m in core imgproc imgcodecs; do
    built="$(ls "$OCV_BUILD"/lib/libopencv_$m.so.4.9.0)" || die "libopencv_$m.so.4.9.0 was not built"
    install -m 0644 "$built" "$OUT/lib/libopencv_$m.so.409"
    "$STRIP" --strip-unneeded "$OUT/lib/libopencv_$m.so.409"
    soname_ok "$OUT/lib/libopencv_$m.so.409" "libopencv_$m.so.409"
done

log "done: $(ls "$OUT/lib" | tr '\n' ' ')"
