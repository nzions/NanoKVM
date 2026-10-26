#!/bin/sh
# An independent read of the finished image (not sd-image.sh's own checks):
# the MBR, the boot partition, the FIT, the kernel and DTB inside it, the rootfs.
# usage: verify.sh <image> <kernel Image>
set -eu
IMG=$1
KIMG=$2
T=$(mktemp -d "$(pwd)/.verify.XXXXXX")
trap 'rm -rf -- "$T"' EXIT
le32() { set -- $(od -An -tu1 -j"$2" -N4 "$1"); echo $(($1 + $2 * 256 + $3 * 65536 + $4 * 16777216)); }
p1s=$(le32 "$IMG" 454)
p1n=$(le32 "$IMG" 458)
p2s=$(le32 "$IMG" 470)
p2n=$(le32 "$IMG" 474)
echo "== image: $(wc -c <"$IMG") bytes"
echo "== MBR signature $(od -An -tx1 -j510 -N2 "$IMG")"
echo "partition 1: type $(od -An -tx1 -j450 -N1 "$IMG") start $p1s sectors $p1n"
echo "partition 2: type $(od -An -tx1 -j466 -N1 "$IMG") start $p2s sectors $p2n"
export MTOOLS_SKIP_CHECK=1
echo "== boot partition (FAT16, sector $p1s)"
mdir -i "$IMG@@$((p1s * 512))" ::
mcopy -i "$IMG@@$((p1s * 512))" ::BOOT.SD "$T/boot.sd"
echo "BOOT.SD sha256 $(sha256sum "$T/boot.sd" | cut -d' ' -f1)"
echo "-- uEnv.txt (kernel command line additions: othbootargs)"
mcopy -i "$IMG@@$((p1s * 512))" ::uEnv.txt - 2>&1
echo "== dumpimage -l BOOT.SD"
dumpimage -l "$T/boot.sd"
echo "== kernel in the FIT vs the Image it was built from"
dumpimage -T flat_dt -p 0 -o "$T/k.lzma" "$T/boot.sd" >/dev/null
xz --format=lzma -dc "$T/k.lzma" >"$T/k.Image"
echo "FIT kernel (decompressed) sha256 $(sha256sum "$T/k.Image" | cut -d' ' -f1)"
echo "input Image               sha256 $(sha256sum "$KIMG" | cut -d' ' -f1)"
cmp "$T/k.Image" "$KIMG" && echo "kernel: identical"
echo "== DTB in the FIT"
dumpimage -T flat_dt -p 1 -o "$T/k.dtb" "$T/boot.sd" >/dev/null
echo "DTB sha256 $(sha256sum "$T/k.dtb" | cut -d' ' -f1)"
dtc -I dtb -O dts "$T/k.dtb" 2>/dev/null | grep -E '^/memreserve/|compatible = "[^"]*nanokvm|bootargs|model =' | head -10
echo "== rootfs (partition 2)"
dd if="$IMG" of="$T/p2.ext4" bs=512 skip="$p2s" count="$p2n" status=none
debugfs -R "cat /kvmapp/version" "$T/p2.ext4" 2>/dev/null
echo "-- /etc/init.d/S00kmod"
debugfs -R "cat /etc/init.d/S00kmod" "$T/p2.ext4" 2>/dev/null
echo "-- /kvmapp/system/init.d/S00kmod"
debugfs -R "cat /kvmapp/system/init.d/S00kmod" "$T/p2.ext4" 2>/dev/null
echo "-- init scripts still mentioning insmod/rmmod/modprobe"
for f in $(debugfs -R "ls -p /etc/init.d" "$T/p2.ext4" 2>/dev/null | awk -F/ 'NF > 5 && $6 ~ /^S[0-9]/ { print $6 }'); do
    n=$(debugfs -R "cat /etc/init.d/$f" "$T/p2.ext4" 2>/dev/null | grep -cE '(^|[^a-z])(insmod|rmmod|modprobe) ') || true
    echo "$f: $n"
done
e2fsck -fn "$T/p2.ext4" && echo "e2fsck -fn: clean"
