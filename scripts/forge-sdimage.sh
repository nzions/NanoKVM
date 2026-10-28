#!/bin/bash
#
# Build the first-boot microSD image of the NanoKVM Cube with OUR kernel on forge.
#
# Entered from scripts/forge-build.sh when the `build` profile is requested with
# scope `scripts` (forge_build repo=nanokvm ref=<sha> scope=["scripts"]); every
# other scope is the riscv64 release build. Runs in forge's `c` image as an
# unprivileged user, with network, HOME=/work/home and the artifacts share at
# /artifacts (rw). Journey 01k5sgmm98-24j2.
#
# It runs scripts/sd-image.sh --kernel-fit --no-stock-ko (the logic lives there):
#   - base image: an earlier sd-image.sh build (nanokvm 2.5.16 on the stock
#     Sipeed 1.4.3 image). Its bytes before partition 2 (MBR, FSBL, OpenSBI,
#     U-Boot, the FAT16 boot partition) are Sipeed's; Sipeed's own download has
#     no pinned URL. Published under https://hc.nikspace.net/nanokvm/sdimg/.
#   - userspace: the release tarball /artifacts/$USERSPACE/nanokvm_$USERSPACE.tar.gz
#     that the release build of this repo published.
#   - kernel: the Image + DTB of fleet containers/kernel-nanokvm-sg2002 (the -full
#     build, vendor drivers built in), copied to .../sdimg/inputs/ (sha256 below).
#   - tools forge-c lacks (mkimage, dumpimage, dtc, mtools, debugfs, e2fsck): pinned
#     Debian trixie .debs from snapshot.debian.org, unpacked with dpkg-deb -x under
#     $HOME and ldd-checked.
# Every input is checked against the sha256 pinned below. To change one, change its
# pin here. Nothing is ever written to a device; the output is a file:
#   /artifacts/sdimg/nanokvm-cube-$USERSPACE-$KERNEL_TAG-sd.img.xz
#   + .sha256 (of the raw image), build.log, verify.txt
# An existing output directory is never overwritten (bump a pin or the tag).

set -eu
set -o pipefail

SRC="$(cd "$(dirname "$0")/.." && pwd)"
ARTIFACTS="${ARTIFACTS_DIR:-/artifacts}"

# ------------------------------------------------------------------- the pins
USERSPACE=2.6.53
USERSPACE_SHA256=TODO-pin-after-the-2.6.53-release-build

KERNEL_TAG=k1.6
INPUTS=https://hc.nikspace.net/nanokvm/sdimg/inputs
KERNEL_IMAGE_URL=$INPUTS/kernel-nanokvm-sg2002-full-1.6.Image
KERNEL_IMAGE_SHA256=6146fb8b1e55ada93079b77b07e5fc265a763c27e24113dcea5f83037fa45f35
KERNEL_DTB_URL=$INPUTS/kernel-nanokvm-sg2002-full-1.6.sg2002-nanokvm-cube.dtb
KERNEL_DTB_SHA256=a4cb731543d3e30063e83a459dd834f0a3c6763865e404de03601e686fa24b76

BASE_XZ_URL=https://hc.nikspace.net/nanokvm/sdimg/nanokvm-cube-2.5.16-sd.img.xz
BASE_XZ_SHA256=0d85839b3a92b51f4c28fbaab4c0b148da23eac896b1f641184ff5477c11ba0c
BASE_RAW_SHA256=e0035fa435a6916ac58da3c908b4c6e93543d4c41c359ebfb83e76390f4dfe0b

# name|url|sha256 (Debian trixie amd64, snapshot.debian.org content-addressed /file/ URLs)
DEBS='
u-boot-tools|https://snapshot.debian.org/file/d303134b65fe4671b389589436d1ca487b37d9c7/u-boot-tools_2025.01-3%2Bdeb13u1_amd64.deb|059e15b46ae05c13d3d255f8a8d0ec559206d76979375a533423002992183a62
mtools|https://snapshot.debian.org/file/ffa895436af9fd0ca88e64488865d4afaaa60ec2/mtools_4.0.48-1_amd64.deb|e7069109ce49f5a450caeca681e454f24dc412f9fdb289cde54e115162203760
device-tree-compiler|https://snapshot.debian.org/file/be1d02a6ff1c045fb848ef38f05b827f9be57c9e/device-tree-compiler_1.7.2-2%2Bb1_amd64.deb|1d2ee77671eb76db21ad455a0e0211ece67f57c6d12b7501b92876bbcea390d6
libfdt1|https://snapshot.debian.org/file/8b4154815fd8f84748ba38349a3feebf6be045c9/libfdt1_1.7.2-2%2Bb1_amd64.deb|8e37034d7864807ae2063efcc82fc79aad59069546212e6e0908ed67f9f7e107
libyaml-0-2|https://snapshot.debian.org/file/cbe72701c2d6caa42cfde3c69572782e4f9494b9/libyaml-0-2_0.2.5-2_amd64.deb|8a5475402801de445a17887bb72d03613565a6a84b216b4b606d1462c613eeb2
e2fsprogs|https://snapshot.debian.org/file/594daea4c1b5bbae559ec71855a19c148bfe2db6/e2fsprogs_1.47.2-3%2Bb12_amd64.deb|e9e638eb3e22b5f14d01ce0b28aa512a5a22fcc0e70d1c733444579b67ea236b
libext2fs2t64|https://snapshot.debian.org/file/6dc4ecb2bc8215274c8e402204c04b7a0089617f/libext2fs2t64_1.47.2-3%2Bb12_amd64.deb|9f5783fa48239e42260d41846fbc6c8b19c233251063524456096195660eaa09
libss2|https://snapshot.debian.org/file/cce8edafae93ebb34ba7ef31468f5cca365c2f1c/libss2_1.47.2-3%2Bb12_amd64.deb|c5d38d71984c1bf774c1254238636e605fc4e0dc24d853f709921eb740327f4a
libcom-err2|https://snapshot.debian.org/file/0a08c25cbaba3fbe4faa800fdec8cdd0f05b4155/libcom-err2_1.47.2-3%2Bb12_amd64.deb|bd43e020b8fed399db3eebf098111f10fba4c436fff99ccad72cc7e27c9764ec
'

die() {
    echo "[forge-sdimage] ERROR: $*" >&2
    exit 1
}

log() {
    echo "[forge-sdimage] $*"
}

[ -n "${HOME:-}" ] || die "HOME is unset"
case "$HOME" in
/ | "") die "HOME is '$HOME'; refusing to build into the filesystem root" ;;
esac
[ "$(id -u)" -ne 0 ] || die "refusing to run as root"
[ "$(uname -m)" = x86_64 ] || die "the pinned tools are amd64 .debs; this is $(uname -m)"
for t in curl xz gzip dpkg-deb ldd sha256sum od cmp awk; do
    command -v "$t" >/dev/null 2>&1 || die "'$t' is missing from the build image"
done

OUTDIR=$ARTIFACTS/sdimg/nanokvm-cube-$USERSPACE-$KERNEL_TAG
[ ! -e "$OUTDIR" ] || die "$OUTDIR exists: a published image is never overwritten (bump a pin or KERNEL_TAG)"
mkdir -p "$ARTIFACTS/sdimg"

W=$HOME/sdimg-build
rm -rf "$W"
mkdir -p "$W/in" "$W/tools" "$W/out"
cd "$W"

fetch() { # <url> <sha256> <file>
    curl -fsS --retry 3 -o "$3" "$1" || die "download failed: $1"
    got=$(sha256sum "$3" | awk '{ print $1 }')
    [ "$got" = "$2" ] || die "$1: sha256 is $got, pinned $2"
    log "fetched $(basename "$3") ($(wc -c <"$3") bytes, sha256 ${got%"${got#????????????}"})"
}

# ------------------------------------------------------------------- tools
echo "$DEBS" | while IFS='|' read -r name url sha; do
    [ -n "$name" ] || continue
    fetch "$url" "$sha" "in/$name.deb"
    dpkg-deb -x "in/$name.deb" tools
done
export PATH="$W/tools/usr/bin:$W/tools/usr/sbin:$W/tools/sbin:$PATH:/usr/sbin:/sbin"
export LD_LIBRARY_PATH="$W/tools/usr/lib/x86_64-linux-gnu${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"
for b in mkimage dumpimage dtc mtools debugfs e2fsck; do
    p=$(command -v "$b") || die "$b is missing after unpacking the pinned .debs"
    case "$p" in "$W"/tools/*) ;; *) log "note: $b comes from the image: $p" ;; esac
    missing=$(ldd "$p" | grep 'not found' || true)
    [ -z "$missing" ] || die "$b: unresolved libraries: $missing"
done
mkimage -V
dtc --version
mtools -V
debugfs -V 2>&1 | head -1
for l in mcopy mdir mdel; do command -v "$l" >/dev/null || die "$l (mtools) is missing"; done

# ------------------------------------------------------------------ inputs
fetch "$KERNEL_IMAGE_URL" "$KERNEL_IMAGE_SHA256" in/kernel.Image
fetch "$KERNEL_DTB_URL" "$KERNEL_DTB_SHA256" in/kernel.dtb
fetch "$BASE_XZ_URL" "$BASE_XZ_SHA256" in/base.img.xz
TARBALL=$ARTIFACTS/$USERSPACE/nanokvm_$USERSPACE.tar.gz
[ -f "$TARBALL" ] || die "$TARBALL not found: build the release $USERSPACE first (forge_build repo=nanokvm)"
[ "$(sha256sum "$TARBALL" | awk '{ print $1 }')" = "$USERSPACE_SHA256" ] || die "$TARBALL is not the pinned release (sha256 $USERSPACE_SHA256)"
cp -- "$TARBALL" in/nanokvm.tar.gz
xz -dc in/base.img.xz >base.img
rm -f in/base.img.xz
log "base image $(wc -c <base.img) bytes"

# ------------------------------------------------------------------ build
RAW=nanokvm-cube-sd.img
{
    sh "$SRC/scripts/sd-image.sh" --kernel-fit in/kernel.Image in/kernel.dtb --no-stock-ko \
        --bootargs "cvi_vc_drv.MaxVencChnNum=9 cvi_vc_drv.MaxVdecChnNum=9" \
        --stock-sha256 "$BASE_RAW_SHA256" --sha256 "$USERSPACE_SHA256" \
        base.img in/nanokvm.tar.gz "$RAW"
} 2>&1 | tee out/build.log
[ -f "$RAW" ] || die "sd-image.sh produced no image (see build.log above)"
rm -f base.img

log "independent read of the finished image"
sh "$SRC/scripts/sdimage-verify.sh" "$RAW" in/kernel.Image 2>&1 | tee out/verify.txt

xz -T0 -6 -c "$RAW" >"out/nanokvm-cube-sd.img.xz"
sha256sum "$RAW" | sed "s#  .*#  nanokvm-cube-$USERSPACE-$KERNEL_TAG-sd.img#" >out/nanokvm-cube-sd.img.sha256
{
    echo "== sha256"
    cat out/nanokvm-cube-sd.img.sha256
    sha256sum out/nanokvm-cube-sd.img.xz | sed "s#  .*#  nanokvm-cube-$USERSPACE-$KERNEL_TAG-sd.img.xz#"
    ls -l "$RAW" out/nanokvm-cube-sd.img.xz
} 2>&1 | tee -a out/verify.txt
rm -f "$RAW"

# publish: copy, verify the copy, then it is done (a failed copy leaves a partial dir that the next run refuses; remove by hand)
mkdir "$OUTDIR"
cp -- out/nanokvm-cube-sd.img.xz "$OUTDIR/nanokvm-cube-$USERSPACE-$KERNEL_TAG-sd.img.xz"
cp -- out/nanokvm-cube-sd.img.sha256 out/build.log out/verify.txt "$OUTDIR/"
[ "$(sha256sum "$OUTDIR/nanokvm-cube-$USERSPACE-$KERNEL_TAG-sd.img.xz" | awk '{ print $1 }')" = "$(sha256sum out/nanokvm-cube-sd.img.xz | awk '{ print $1 }')" ] ||
    die "the published copy differs from the built one"
log "published $OUTDIR"
log "raw image sha256: $(awk '{ print $1 }' out/nanokvm-cube-sd.img.sha256)"
log "xz sha256: $(sha256sum "$OUTDIR/nanokvm-cube-$USERSPACE-$KERNEL_TAG-sd.img.xz" | awk '{ print $1 }'), $(wc -c <"$OUTDIR/nanokvm-cube-$USERSPACE-$KERNEL_TAG-sd.img.xz") bytes"
log "NOT flashed. Flashing a SPARE microSD card waits for Nik's go."
