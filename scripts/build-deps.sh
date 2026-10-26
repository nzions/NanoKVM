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
# Usage: build-deps.sh <outdir>      libraries land in <outdir>/lib, the public
#                                    headers in <outdir>/include/opencv4
#
# libkvm.so is compiled and linked against exactly these headers and libraries
# (support/sg2002/additional/opencv, which replaces MaixCDK's opencv component;
# scripts/build-in-container.sh checks it).
#
# The source tarball is NOT fetched from github.com: $NANOKVM_SRC_MIRROR (forge-build.sh
# sets it to $ARTIFACTS/src) names the directory on the artifacts share it must be in,
# and the script refuses to run without it. A missing file or any sha256 other than the
# one below aborts the build: no unpinned fetch, no fallback. FreeType and HarfBuzz are
# not here: MaixCDK builds them from its own pinned sources and libkvm.so embeds them.
# Needs on PATH: riscv64-unknown-linux-musl-gcc/g++, cmake, make, tar.

set -euo pipefail

OPENCV_VERSION=4.9.0
OPENCV_TARBALL=opencv-4.9.0.tar.gz
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
for tool in "$CC" "$CXX" cmake make tar sha256sum readelf; do
    command -v "$tool" >/dev/null 2>&1 || die "'$tool' is missing"
done
STRIP=riscv64-unknown-linux-musl-strip
command -v "$STRIP" >/dev/null 2>&1 || die "'$STRIP' is missing"
[ -n "${NANOKVM_SRC_MIRROR:-}" ] || die "NANOKVM_SRC_MIRROR is unset: the OpenCV source comes from the artifacts share, not from github.com"

SRC="$OUT/src"
rm -rf "$SRC"
mkdir -p "$SRC" "$OUT/lib"
JOBS="$(nproc)"

# fetch <tarball> <sha256>: the file must be in the mirror and hash to the pin.
fetch() {
    local file="$NANOKVM_SRC_MIRROR/$1" sha="$2" got
    [ -f "$file" ] || die "$file is missing: put $1 there (sha256 $sha)"
    got="$(sha256sum "$file" | cut -d' ' -f1)"
    [ "$got" = "$sha" ] || die "$1: sha256 $got, pinned $sha"
    log "$1 sha256 ok"
}

# soname_ok <lib> <soname>: the file must declare exactly that SONAME.
soname_ok() {
    readelf -d "$1" | grep -q "Library soname: \[$2\]" || die "$1 has no SONAME $2"
}

# --- OpenCV ----------------------------------------------------------------------
fetch "$OPENCV_TARBALL" "$OPENCV_SHA256"
tar -xzf "$NANOKVM_SRC_MIRROR/$OPENCV_TARBALL" -C "$SRC"
OCV_SRC="$SRC/opencv-$OPENCV_VERSION"
[ -d "$OCV_SRC/modules/core" ] || die "opencv source tree missing after extraction"
OCV_BUILD="$OUT/opencv-build"
rm -rf "$OCV_BUILD"
# OpenCV must not fetch anything at configure time (ADE for G-API, ippicv, ...): the
# download cache is an empty directory, G-API and ADE are off, and after configuring both
# the cache and the configure log must show no download (fail closed).
OCV_DL="$OUT/opencv-download"
OCV_CFG_LOG="$OUT/opencv-configure.log"
rm -rf "$OCV_DL"
mkdir -p "$OCV_DL"
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
    -DBUILD_JAVA=OFF -DOPENCV_GENERATE_PKGCONFIG=OFF \
    -DBUILD_opencv_gapi=OFF -DWITH_ADE=OFF -DOPENCV_DOWNLOAD_PATH="$OCV_DL" 2>&1 | tee "$OCV_CFG_LOG"
# (OpenCV writes its own .gitignore into the cache directory; any other non-empty file is a download)
ocv_dl_files="$(find "$OCV_DL" -type f -size +0 ! -name .gitignore)"
[ -z "$ocv_dl_files" ] || die "OpenCV downloaded into $OCV_DL at configure time: $(echo $ocv_dl_files)"
log "OpenCV download cache contents: $(find "$OCV_DL" -mindepth 1 | tr '\n' ' ')"
# cmake/OpenCVDownload.cmake (4.9.0) writes CMakeDownloadLog.txt on every configure: line 1 is
# '#use_cache "<cache dir>"'; a real network fetch appends '#cmake_download "<file>" "<url>"'
# (then '#try N'); a request served from the cache logs '#do_*' / '#copy' / '#unpack' instead.
# Print the log, and fail only on the network-download record.
OCV_DL_LOG="$OCV_BUILD/CMakeDownloadLog.txt"
[ -f "$OCV_DL_LOG" ] || die "$OCV_DL_LOG is missing: cannot prove OpenCV did not download"
log "OpenCV CMakeDownloadLog.txt:"
sed 's/^/    /' "$OCV_DL_LOG"
if grep -E '^#(cmake_download|try) ' "$OCV_DL_LOG"; then
    die "OpenCV downloaded at configure time (records above)"
fi
if grep -E 'Downloading |downloading |ocv_download' "$OCV_CFG_LOG" | grep -v 'Using github.com to download 3rdparty'; then
    die "OpenCV configure output mentions a download (lines above)"
fi
log "OpenCV configure: no download (no file in the cache $OCV_DL, no #cmake_download record in CMakeDownloadLog.txt, no download line in the configure output)"
log "building OpenCV core, imgproc, imgcodecs ($JOBS jobs)"
cmake --build "$OCV_BUILD" --parallel "$JOBS" --target opencv_core opencv_imgproc opencv_imgcodecs

for m in core imgproc imgcodecs; do
    built="$(ls "$OCV_BUILD"/lib/libopencv_$m.so.4.9.0)" || die "libopencv_$m.so.4.9.0 was not built"
    install -m 0644 "$built" "$OUT/lib/libopencv_$m.so.409"
    "$STRIP" --strip-unneeded "$OUT/lib/libopencv_$m.so.409"
    soname_ok "$OUT/lib/libopencv_$m.so.409" "libopencv_$m.so.409"
done

# MaixCDK turns a library path into -L<dir> -l<name>, which needs the unversioned name:
# <outdir>/link/libopencv_<m>.so -> ../lib/libopencv_<m>.so.409 (link time only; not shipped).
rm -rf "$OUT/link"
mkdir -p "$OUT/link"
for m in core imgproc imgcodecs; do
    ln -s "../lib/libopencv_$m.so.409" "$OUT/link/libopencv_$m.so"
done

# The public headers libkvm.so compiles against: what `cmake --install` puts in
# include/opencv4 for the three built modules (each module's include/, the umbrella
# opencv2/opencv.hpp, and the two generated headers cvconfig.h / opencv2/opencv_modules.hpp).
INC="$OUT/include/opencv4"
rm -rf "$OUT/include"
mkdir -p "$INC/opencv2"
for m in core imgproc imgcodecs; do
    [ -d "$OCV_SRC/modules/$m/include/opencv2" ] || die "opencv module $m has no include/opencv2"
    cp -R "$OCV_SRC/modules/$m/include/opencv2/." "$INC/opencv2/"
done
cp "$OCV_SRC/include/opencv2/opencv.hpp" "$INC/opencv2/opencv.hpp"
cp "$OCV_BUILD/cvconfig.h" "$INC/cvconfig.h"
cp "$OCV_BUILD/opencv2/opencv_modules.hpp" "$INC/opencv2/opencv_modules.hpp"
for h in opencv2/opencv.hpp opencv2/core.hpp opencv2/imgproc.hpp opencv2/imgcodecs.hpp \
         opencv2/core/version.hpp opencv2/opencv_modules.hpp cvconfig.h; do
    [ -f "$INC/$h" ] || die "header $h is missing from $INC"
done
grep -q "define CV_VERSION_MAJOR *4" "$INC/opencv2/core/version.hpp" || die "headers are not OpenCV 4.x"
grep -q "define CV_VERSION_MINOR *9" "$INC/opencv2/core/version.hpp" || die "headers are not OpenCV 4.9"

log "done: $(ls "$OUT/lib" | tr '\n' ' '); headers in $INC"
