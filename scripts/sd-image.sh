#!/bin/sh
#
# Build a flashable microSD image for a real NanoKVM Cube / Full (Sipeed SG2002)
# from Sipeed's stock image and one of OUR signed releases.
#
# The result boots exactly what an online update (OTA) of that release would
# leave on a stock device: the stock kernel, boot partition, musl loaders and
# sysvinit are untouched, and /kvmapp is replaced by the release tree the way
# server/service/application/install.go does it (whole tree, files 0755). On
# top of the OTA result it also installs the release's init scripts into
# /etc/init.d (the copies kvm_system's new_app_init makes at first boot), so the
# very first boot already runs our S95nanokvm. kvm_new_app stays in the tree and
# kvm_new_img is carried over from the stock /kvmapp, so kvm_system still does
# its normal first-boot work (resolv.conf, init.d refresh, directories).
#
# NOT done, on purpose (these are for the QEMU emulator only, see emu-image.sh):
# no loader swaps, no busybox init, no /etc/kvm/emulator marker.
#
# NO secret is written. /etc/kvm/pwd, initial.pass and root.pass are absent from
# the image; the one web account is root and its password is root's Linux
# password. On first boot the server takes /boot/root.pass for it, else sets a
# random password, shows it on the OLED and writes it to /etc/kvm/initial.pass
# (0600); the owner must change that one at first login. /etc/kvm/ssh_stop
# (stock: sshd off) is left as shipped.
#
# No root and no loop mount: the rootfs (partition 2) is cut out with dd, edited
# with debugfs -w, checked with e2fsck and spliced back with dd conv=notrunc.
# The inputs are only read. Needs: debugfs e2fsck dd tar find sort cmp
# diff od sha256sum awk (e2fsprogs, coreutils); the partition table is read
# from the MBR with od.
#
# OUR KERNEL (--kernel-fit): the stock FSBL, OpenSBI and U-Boot (FIP.BIN) stay,
# and so does everything else on the FAT boot partition (partition 1); only
# BOOT.SD, the FIT that stock U-Boot loads to uImage_addr (0x81800000) and
# boots with `bootm ${uImage_addr}#config-sg2002_licheervnano_sd`, is replaced
# by a FIT built with mkimage from OUR Image and DTB. The FIT matches the stock
# one (read from the stock BOOT.SD): #address-cells 2, one riscv/linux kernel
# image (crc32), one flat_dt image (sha256), the configuration named exactly
# config-sg2002_licheervnano_sd, and NO ramdisk (our DTB boots root=/dev/mmcblk0p2
# directly). The kernel is compressed (lzma by default; stock U-Boot 2021.10
# does gz, bz2, lzma, lz4, zstd) because our Image is ~38 MB and the boot
# partition is 16 MB FAT16, and it is loaded at --fit-load, ABOVE the FIT's own
# place in RAM (uImage_addr + FIT size), so the decompressor never overwrites
# the data it reads or the DTB that bootm leaves in place. The stock bootargs
# reserve the first 0x80000 of RAM for OpenSBI with `riscv.fwsz=0x80000`, a
# parameter only the vendor kernel reads; the mainline DTB has no node for it,
# so the DTB gets a /memreserve/ for that range (--fw-reserve).
# --no-stock-ko: our drivers are built into the kernel, so S00kmod (insmod of
# the stock soph_*.ko under /mnt/system/ko) becomes a no-op, both in
# /etc/init.d and in the /kvmapp/system/init.d copy kvm_system refreshes it from.
# Needs besides the above: mkimage dumpimage dtc mdir mcopy mdel (u-boot-tools,
# device-tree-compiler, mtools), xz gzip.
#
# Usage:
#   scripts/sd-image.sh [--stock-sha256 <hex>] [--sha256 <hex>] \
#       [--kernel-fit <Image> <dtb>] [--fit-load <hex>] [--fit-comp lzma|gzip] \
#       [--fw-reserve <hex>] [--no-stock-ko] \
#       <stock.img> <nanokvm_VERSION.tar.gz> <out.img>
#
#   --sha256        expected sha256 of the release tarball; default: read from
#                   <tarball>.sha256 next to it. One of the two is required.
#   --stock-sha256  expected sha256 of the stock image; default: the
#                   20260610_NanoKVM_Rev1_4_3.img below. An image this script
#                   built earlier works as the input too (its FSBL, U-Boot and
#                   boot partition are the stock ones): pass its sha256 here.
#   --kernel-fit    replace BOOT.SD with a FIT of this riscv64 Image and DTB.
#   --fit-load      kernel load = entry address (default 0x83000000, 2 MiB
#                   aligned, above uImage_addr + the FIT).
#   --fit-comp      kernel compression in the FIT (default lzma).
#   --fw-reserve    size of the /memreserve/ at the DRAM base 0x80000000 added
#                   to the DTB (default 0x80000, the stock riscv.fwsz; 0 = none).
#   --no-stock-ko   neutralise S00kmod, see above.
#
# <out.img> must not exist. Writes <out.img>.sha256 too.

set -eu

STOCK_SHA256_DEFAULT=b03f3be9b556dd68c6175b90ae80abd04f256922580f66186befffa399c47f71

die() {
    echo "[ERROR] $*" >&2
    exit 1
}

info() {
    echo "[sd-image] $*"
}

usage() {
    cat >&2 <<EOF
Usage: $0 [--stock-sha256 <hex>] [--sha256 <hex>] [--kernel-fit <Image> <dtb>] [--fit-load <hex>] [--fit-comp lzma|gzip] [--fw-reserve <hex>] [--no-stock-ko] <stock.img> <nanokvm_VERSION.tar.gz> <out.img>
EOF
    exit 1
}

need_tool() {
    command -v "$1" >/dev/null 2>&1 || die "required tool not found on PATH: $1"
}

sha_of() {
    sha256sum "$1" | awk '{ print $1 }'
}

# ------------------------------------------------------------------ arguments

WANT_STOCK=$STOCK_SHA256_DEFAULT
WANT_TAR=""
KFIT=0
KIMAGE=""
KDTB=""
FIT_LOAD=0x83000000
FIT_COMP=lzma
FW_RESERVE=0x80000
NO_STOCK_KO=0
while [ $# -gt 0 ]; do
    case "$1" in
    --stock-sha256)
        [ $# -ge 2 ] || usage
        WANT_STOCK=$2
        shift 2
        ;;
    --sha256)
        [ $# -ge 2 ] || usage
        WANT_TAR=$2
        shift 2
        ;;
    --kernel-fit)
        [ $# -ge 3 ] || usage
        KFIT=1
        KIMAGE=$2
        KDTB=$3
        shift 3
        ;;
    --fit-load)
        [ $# -ge 2 ] || usage
        FIT_LOAD=$2
        shift 2
        ;;
    --fit-comp)
        [ $# -ge 2 ] || usage
        FIT_COMP=$2
        shift 2
        ;;
    --fw-reserve)
        [ $# -ge 2 ] || usage
        FW_RESERVE=$2
        shift 2
        ;;
    --no-stock-ko)
        NO_STOCK_KO=1
        shift
        ;;
    -*) usage ;;
    *) break ;;
    esac
done
[ $# -eq 3 ] || usage
STOCK=$1
TARBALL=$2
OUT=$3

for t in debugfs e2fsck dd tar find sort cmp diff od sha256sum awk grep mktemp wc tr; do
    need_tool "$t"
done
if [ "$KFIT" = 1 ]; then
    for t in mkimage dumpimage dtc mdir mcopy mdel xz gzip; do
        need_tool "$t"
    done
    [ -f "$KIMAGE" ] || die "kernel Image not found: $KIMAGE"
    [ -f "$KDTB" ] || die "kernel DTB not found: $KDTB"
    case "$FIT_COMP" in lzma | gzip) ;; *) die "--fit-comp must be lzma or gzip" ;; esac
    case "$FIT_LOAD$FW_RESERVE" in *[!0-9a-fA-Fx]*) die "--fit-load and --fw-reserve must be hex numbers" ;; esac
    [ $((FIT_LOAD % 2097152)) -eq 0 ] || die "--fit-load $FIT_LOAD is not 2 MiB aligned"
    [ $((FIT_LOAD)) -ge $((0x80200000)) ] || die "--fit-load $FIT_LOAD is below 0x80200000"
else
    [ "$FIT_COMP$FIT_LOAD$FW_RESERVE" = "lzma0x830000000x80000" ] || [ "$KFIT" = 1 ] ||
        die "--fit-load, --fit-comp and --fw-reserve need --kernel-fit"
fi

[ -f "$STOCK" ] || die "stock image not found: $STOCK"
[ -f "$TARBALL" ] || die "release tarball not found: $TARBALL"
[ ! -e "$OUT" ] || die "refusing to overwrite existing file: $OUT"
OUT_DIR=$(cd "$(dirname "$OUT")" 2>/dev/null && pwd) || die "output directory does not exist: $(dirname "$OUT")"
OUT=$OUT_DIR/$(basename "$OUT")

is_hex64() {
    printf '%s' "$1" | grep -qE '^[0-9a-fA-F]{64}$'
}
is_hex64 "$WANT_STOCK" || die "--stock-sha256 must be 64 hex digits"
if [ -z "$WANT_TAR" ]; then
    [ -f "$TARBALL.sha256" ] || die "no --sha256 given and $TARBALL.sha256 does not exist"
    WANT_TAR=$(awk 'NR == 1 { print $1 }' "$TARBALL.sha256")
fi
is_hex64 "$WANT_TAR" || die "release sha256 must be 64 hex digits (got '$WANT_TAR')"

have=$(sha_of "$STOCK")
want=$(printf '%s' "$WANT_STOCK" | tr 'A-F' 'a-f')
[ "$have" = "$want" ] || die "stock image sha256 is $have, expected $want"
info "stock image sha256 verified"
have=$(sha_of "$TARBALL")
want=$(printf '%s' "$WANT_TAR" | tr 'A-F' 'a-f')
[ "$have" = "$want" ] || die "release tarball sha256 is $have, expected $want"
info "release tarball sha256 verified"

# ----------------------------------------------------------- partition 2 (p2)

# The stock image has an MBR table (0x55aa at 510, four 16-byte entries at 446).
SECTOR=512
le32() { # <file> <byte offset>: the little-endian u32 there
    set -- $(od -An -tu1 -j"$2" -N4 "$1")
    [ $# -eq 4 ] || die "cannot read 4 bytes from the image"
    echo $(($1 + $2 * 256 + $3 * 65536 + $4 * 16777216))
}
[ "$(echo $(od -An -tx1 -j510 -N2 "$STOCK"))" = "55 aa" ] || die "$STOCK has no MBR signature (55 aa at offset 510)"
part_start() { le32 "$STOCK" $((446 + 16 * ($1 - 1) + 8)); }
part_size() { le32 "$STOCK" $((446 + 16 * ($1 - 1) + 12)); }
part_type() { echo $(od -An -tx1 -j$((446 + 16 * ($1 - 1) + 4)) -N1 "$STOCK"); }
P2_START=$(part_start 2)
P2_SIZE=$(part_size 2)
case "$P2_START$P2_SIZE" in
"" | *[!0-9]*) die "cannot parse partition 2 geometry from the MBR: '$P2_START' '$P2_SIZE'" ;;
esac
[ "$P2_START" -gt 0 ] && [ "$P2_SIZE" -gt 0 ] || die "partition 2 has a zero start or size: '$P2_START' '$P2_SIZE'"
img_bytes=$(wc -c <"$STOCK")
[ $(((P2_START + P2_SIZE) * SECTOR)) -le "$img_bytes" ] ||
    die "partition 2 (start $P2_START, size $P2_SIZE sectors) runs past the end of $STOCK"
info "partition 2: start sector $P2_START, $P2_SIZE sectors of $SECTOR bytes (type $(part_type 2))"
if [ "$KFIT" = 1 ]; then
    P1_START=$(part_start 1)
    P1_SIZE=$(part_size 1)
    [ "$P1_START" -gt 0 ] && [ "$P1_SIZE" -gt 0 ] || die "partition 1 has a zero start or size: '$P1_START' '$P1_SIZE'"
    [ $((P1_START + P1_SIZE)) -le "$P2_START" ] || die "partition 1 (start $P1_START, size $P1_SIZE) overlaps partition 2"
    case "$(part_type 1)" in 0b | 0c | 06 | 0e) ;; *) die "partition 1 is type $(part_type 1), not FAT" ;; esac
    info "partition 1: start sector $P1_START, $P1_SIZE sectors of $SECTOR bytes (type $(part_type 1))"
fi

# ------------------------------------------------------------------ workspace

T=$(mktemp -d "$OUT_DIR/.sd-image.XXXXXX") || die "cannot create a temp dir in $OUT_DIR"
cleanup() {
    rm -rf -- "$T"
}
trap cleanup EXIT
FS=$T/rootfs.ext4
X=$T/x

# ------------------------------------------------- the release tarball, checked

# Same rules the on-device updater enforces: one root dir nanokvm_<ver>/, only
# directories and regular files, no traversal.
tar -tzvf "$TARBALL" >"$T/tar.list" 2>"$T/tar.err" || die "cannot list $TARBALL: $(cat "$T/tar.err")"
tar -tzf "$TARBALL" >"$T/tar.names" || die "cannot list $TARBALL"
grep -qE '[[:space:]]' "$T/tar.names" && die "release has a path with whitespace; refusing"
grep -qE '(^/|(^|/)\.\.(/|$))' "$T/tar.names" && die "release has an absolute or .. path; refusing"
awk '{ t = substr($1, 1, 1); if (t != "-" && t != "d") bad = 1 } END { exit bad }' "$T/tar.list" ||
    die "release contains something other than directories and regular files; refusing"
ROOT=$(awk -F/ 'NR == 1 { print $1 }' "$T/tar.names")
case "$ROOT" in
nanokvm_[0-9]*) ;;
*) die "release root '$ROOT' is not nanokvm_<version>" ;;
esac
awk -F/ -v r="$ROOT" '$1 != r { bad = 1 } END { exit bad }' "$T/tar.names" ||
    die "release has entries outside $ROOT/"
VERSION=${ROOT#nanokvm_}
mkdir "$X"
tar -xzf "$TARBALL" -C "$X" || die "cannot extract $TARBALL"
SRC=$X/$ROOT
[ "$(cat "$SRC/version")" = "$VERSION" ] || die "$ROOT/version says '$(cat "$SRC/version")', not $VERSION"
for f in server/NanoKVM-Server kvm_system/kvm_system system/init.d/S95nanokvm system/nanokvm-supervise system/nanokvm-firewall; do
    [ -f "$SRC/$f" ] || die "release lacks $f"
done
for f in server/NanoKVM-Server kvm_system/kvm_system; do
    elf=$(echo $(od -An -tx1 -N20 "$SRC/$f"))
    case "$elf" in
    "7f 45 4c 46 02 01 "*" f3 00") ;;
    *) die "$f is not a riscv64 ELF (header: $elf)" ;;
    esac
done
UNPACKED=$(find "$SRC" -type f -exec wc -c {} + | awk '$2 != "total" { t += $1 } END { print t + 0 }')
[ "$UNPACKED" -gt 0 ] || die "release unpacks to zero bytes"
info "release $VERSION: $UNPACKED bytes unpacked"

# --no-stock-ko: the kernel carries its drivers, there is nothing to insmod. The
# stub replaces the release's S00kmod in the extracted tree, so the copy under
# /kvmapp/system/init.d (which kvm_system's new_app_init refreshes /etc/init.d
# from) is the stub too; the read-back below then compares against the stub.
STOCK_KO_MARK="no stock .ko: drivers are built into the kernel"
if [ "$NO_STOCK_KO" = 1 ]; then
    grep -q insmod "$SRC/system/init.d/S00kmod" || die "release S00kmod has no insmod; nothing to neutralise (is this the right release?)"
    cat >"$SRC/system/init.d/S00kmod" <<EOF
#!/bin/sh
# $STOCK_KO_MARK (sd-image.sh --no-stock-ko); the stock script insmod'ed the
# soph_*.ko files from /mnt/system/ko, which a kernel with its drivers built in
# neither needs nor can load.
if [ "\$1" = "start" ]
then
	echo "kernel modules: built into the kernel, none loaded"
fi
exit 0
EOF
    chmod 0755 "$SRC/system/init.d/S00kmod"
    info "S00kmod replaced by a no-op stub (--no-stock-ko)"
fi

# ------------------------------------------------------------ debugfs helpers

dd if="$STOCK" of="$FS" bs="$SECTOR" skip="$P2_START" count="$P2_SIZE" status=none ||
    die "dd failed extracting partition 2"

dfs() { # read-only
    debugfs -R "$1" "$FS" 2>/dev/null
}
dfsw() { # write one command; debugfs exits 0 on failure, so output is checked
    debugfs -w -R "$1" "$FS" >"$T/dfsw.out" 2>&1 || die "debugfs '$1' failed: $(cat "$T/dfsw.out")"
}
exists() {
    case "$(dfs "stat $1")" in *"Inode:"*) return 0 ;; *) return 1 ;; esac
}
mode_is() { # <fs path> <permission digits, e.g. 755>
    dfs "stat $1" | grep -qE "Mode: +0*$2( |$)" || die "$1 does not have mode $2"
}
batch() { # <cmdfile>; debugfs -f echoes each command and any error after it
    debugfs -w -f "$1" "$FS" >"$T/batch.out" 2>&1 || die "debugfs -f $1 failed: $(cat "$T/batch.out")"
    # an error line is anything not a command echo ("debugfs: ...") or the banner
    # (`write` prints "Allocated inode: N" on success)
    if grep -vE '^(debugfs: |debugfs [0-9]|Allocated inode: )' "$T/batch.out" | grep -q .; then
        die "debugfs reported errors for $1: $(grep -vE '^(debugfs: |debugfs [0-9]|Allocated inode: )' "$T/batch.out" | head -5)"
    fi
}

root_stat=$(dfs "stat /")
case "$root_stat" in *"Type: directory"*) ;; *) die "partition 2 of $STOCK is not an ext2/3/4 filesystem" ;; esac
fsck() {
    rc=0
    e2fsck -fy "$FS" >"$T/fsck.out" 2>&1 || rc=$?
    [ "$rc" -le 1 ] || die "e2fsck -fy ($1) exited $rc: $(cat "$T/fsck.out")"
    info "e2fsck ($1): exit $rc"
}
fsck "before edits"

exists /kvmapp/version || die "/kvmapp/version not in the image: not a NanoKVM rootfs"
exists /kvmapp/kvm_new_img || die "/kvmapp/kvm_new_img not in the stock image (first-boot marker); cannot carry it over"
exists /etc/init.d/S95nanokvm || die "/etc/init.d/S95nanokvm not in the image"
info "stock /kvmapp/version: $(dfs 'cat /kvmapp/version')"

# ------------------------------------------- 1. remove the stock /kvmapp tree

# debugfs has no recursive rm: walk `ls -p` ("/ino/mode/uid/gid/name/size/").
rm_tree() { # <fs dir>; appends commands to $T/rm.cmd (children before the dir)
    list=$(dfs "ls -p $1")
    while IFS=/ read -r _ ino mode uid gid name size _; do
        case "$name" in "" | . | ..) continue ;; esac
        case "$mode" in
        04*) rm_tree "$1/$name" ;;
        *) echo "rm $1/$name" >>"$T/rm.cmd" ;;
        esac
    done <<EOF
$list
EOF
    echo "rmdir $1" >>"$T/rm.cmd"
}
: >"$T/rm.cmd"
rm_tree /kvmapp
batch "$T/rm.cmd"
! exists /kvmapp || die "/kvmapp is still present after removal"
info "stock /kvmapp removed ($(wc -l <"$T/rm.cmd") entries)"

# ---------------------------------------------------------- 2. space check

freeblocks=$(dfs stats | awk -F: '$1 == "Free blocks" { gsub(/ /, "", $2); print $2 }')
bsize=$(dfs stats | awk -F: '$1 == "Block size" { gsub(/ /, "", $2); print $2 }')
case "$freeblocks$bsize" in "" | *[!0-9]*) die "cannot read free space from debugfs stats" ;; esac
free=$((freeblocks * bsize))
need=$((UNPACKED + UNPACKED / 5 + 8388608))
[ "$free" -ge "$need" ] || die "not enough free space in the rootfs: $free bytes free, need about $need"
info "free space $free bytes, need about $need"

# --------------------------------------------- 3. install the release tree

# Like install.go: every file 0755, the root 0755; owner root:root. Directories
# are 0755 too (the tarball's 0750 would hide them from a non-root reader; root
# runs everything).
: >"$T/put.cmd"
echo "mkdir /kvmapp" >>"$T/put.cmd"
find "$SRC" -mindepth 1 -type d | sort | while read -r d; do
    rel=${d#"$SRC"/}
    echo "mkdir /kvmapp/$rel" >>"$T/put.cmd"
done
find "$SRC" -type f | sort | while read -r f; do
    rel=${f#"$SRC"/}
    echo "write $f /kvmapp/$rel" >>"$T/put.cmd"
    echo "sif /kvmapp/$rel mode 0100755" >>"$T/put.cmd"
    echo "sif /kvmapp/$rel uid 0" >>"$T/put.cmd"
    echo "sif /kvmapp/$rel gid 0" >>"$T/put.cmd"
done
{
    echo "sif /kvmapp mode 040755"
    find "$SRC" -mindepth 1 -type d | sort | while read -r d; do
        echo "sif /kvmapp/${d#"$SRC"/} mode 040755"
    done
    find "$SRC" -mindepth 1 -type d | sort | while read -r d; do
        echo "sif /kvmapp/${d#"$SRC"/} uid 0"
        echo "sif /kvmapp/${d#"$SRC"/} gid 0"
    done
    echo "sif /kvmapp uid 0"
    echo "sif /kvmapp gid 0"
} >>"$T/put.cmd"
# kvm_system's new_img_init (resolv.conf) keys on this stock marker.
: >"$T/kvm_new_img"
echo "write $T/kvm_new_img /kvmapp/kvm_new_img" >>"$T/put.cmd"
echo "sif /kvmapp/kvm_new_img mode 0100644" >>"$T/put.cmd"
echo "sif /kvmapp/kvm_new_img uid 0" >>"$T/put.cmd"
echo "sif /kvmapp/kvm_new_img gid 0" >>"$T/put.cmd"
batch "$T/put.cmd"
info "release tree written ($(wc -l <"$T/put.cmd") debugfs commands)"

# ------------------------------------- 4. init scripts into /etc/init.d

# The set kvm_system's new_app_init copies (S30wifi only where the board had it,
# which the stock image does). S95nanokvm is the one that matters: it
# supervises the server, applies the firewall and recovers an interrupted OTA.
: >"$T/init.cmd"
for s in S00kmod S01fs S03usbdev S15kvmhwd S30eth S50sshd S95nanokvm; do
    [ -f "$SRC/system/init.d/$s" ] || die "release lacks system/init.d/$s"
    if exists "/etc/init.d/$s"; then echo "rm /etc/init.d/$s" >>"$T/init.cmd"; fi
    echo "write $SRC/system/init.d/$s /etc/init.d/$s" >>"$T/init.cmd"
    echo "sif /etc/init.d/$s mode 0100755" >>"$T/init.cmd"
    echo "sif /etc/init.d/$s uid 0" >>"$T/init.cmd"
    echo "sif /etc/init.d/$s gid 0" >>"$T/init.cmd"
done
if exists /etc/init.d/S30wifi; then
    echo "rm /etc/init.d/S30wifi" >>"$T/init.cmd"
    echo "write $SRC/system/init.d/S30wifi /etc/init.d/S30wifi" >>"$T/init.cmd"
    echo "sif /etc/init.d/S30wifi mode 0100755" >>"$T/init.cmd"
    echo "sif /etc/init.d/S30wifi uid 0" >>"$T/init.cmd"
    echo "sif /etc/init.d/S30wifi gid 0" >>"$T/init.cmd"
fi
batch "$T/init.cmd"
info "init scripts installed"

# ----------------------------------------------------- 5. check, splice back

fsck "after edits"

# read the tree back out of the image and compare it with the release
rm -rf "$T/chk"
mkdir "$T/chk"
debugfs -R "rdump /kvmapp $T/chk" "$FS" >"$T/rdump.out" 2>&1 || die "rdump failed: $(cat "$T/rdump.out")"
diff -r "$SRC" "$T/chk/kvmapp" >"$T/diff.out" 2>&1 || true
# the only expected differences: the carried-over kvm_new_img
grep -vE '^Only in '"$T"'/chk/kvmapp: kvm_new_img$' "$T/diff.out" | grep -q . &&
    die "the /kvmapp read back from the image differs from the release: $(cat "$T/diff.out")"
for f in /kvmapp/server/NanoKVM-Server /kvmapp/kvm_system/kvm_system /kvmapp/system/init.d/S95nanokvm \
    /kvmapp/system/nanokvm-supervise /kvmapp/system/nanokvm-firewall /etc/init.d/S95nanokvm /etc/init.d/S50sshd; do
    mode_is "$f" 755
done
cmp -s "$SRC/system/init.d/S95nanokvm" "$T/chk/kvmapp/system/init.d/S95nanokvm" || die "S95nanokvm differs"
dfs "cat /etc/init.d/S95nanokvm" | cmp -s - "$SRC/system/init.d/S95nanokvm" || die "/etc/init.d/S95nanokvm is not the release's"
[ "$(dfs 'cat /kvmapp/version')" = "$VERSION" ] || die "/kvmapp/version is not $VERSION after the edit"
# no credential of any kind may be in the image
for f in /etc/kvm/pwd /etc/kvm/initial.pass /etc/kvm/root.pass /etc/kvm/wifi.pass /etc/kvm/server.yaml; do
    ! exists "$f" || die "$f exists in the image; this script must not ship credentials"
done
info "tree verified against the release; no credential files present"

if [ "$NO_STOCK_KO" = 1 ]; then
    for f in /etc/init.d/S00kmod /kvmapp/system/init.d/S00kmod; do
        body=$(dfs "cat $f")
        case "$body" in *"$STOCK_KO_MARK"*) ;; *) die "$f is not the no-op stub" ;; esac
        case "$body" in *insmod\ *) die "$f still has an insmod line" ;; esac
        mode_is "$f" 755
    done
    # every other init script of the image that still loads or removes a .ko is
    # listed (the release's S15kvmhwd rmmod/insmods i2c-gpio.ko, which fails
    # harmlessly where the file is absent), so the log shows what is left
    for f in $(dfs "ls -p /etc/init.d" | awk -F/ 'NF > 5 && $6 ~ /^S[0-9]/ { print $6 }'); do
        n=$(dfs "cat /etc/init.d/$f" | grep -cE '(^|[^a-z])(insmod|rmmod|modprobe) ') || true
        [ "${n:-0}" = 0 ] || info "/etc/init.d/$f still has $n insmod/rmmod/modprobe line(s)"
    done
    info "stock .ko loading neutralised: S00kmod is a no-op in /etc/init.d and in /kvmapp/system/init.d"
fi

# ----------------------------------------------- 6. boot partition: our kernel

if [ "$KFIT" = 1 ]; then
    BOOTFAT=$T/boot.fat
    FITDIR=$T/fit
    mkdir "$FITDIR"
    dd if="$STOCK" of="$BOOTFAT" bs="$SECTOR" skip="$P1_START" count="$P1_SIZE" status=none ||
        die "dd failed extracting partition 1"
    MTOOLS_SKIP_CHECK=1
    export MTOOLS_SKIP_CHECK
    mdir -i "$BOOTFAT" :: >"$FITDIR/mdir.before" 2>&1 || die "mdir cannot read partition 1 as FAT: $(cat "$FITDIR/mdir.before")"
    grep -qi '^BOOT *SD' "$FITDIR/mdir.before" || die "partition 1 has no BOOT.SD: $(cat "$FITDIR/mdir.before")"
    grep -qi '^FIP *BIN' "$FITDIR/mdir.before" || die "partition 1 has no FIP.BIN (the stock FSBL/U-Boot): $(cat "$FITDIR/mdir.before")"
    info "stock boot partition: $(grep -i 'bytes free' "$FITDIR/mdir.before" | tr -s ' ')"

    # the stock FIT is the reference for the node names U-Boot's sdboot command
    # (bootm ${uImage_addr}#config-sg2002_licheervnano_sd) and the addressing
    CONF=config-sg2002_licheervnano_sd
    mcopy -i "$BOOTFAT" ::BOOT.SD "$FITDIR/stock-boot.sd" || die "mcopy cannot read BOOT.SD"
    dumpimage -l "$FITDIR/stock-boot.sd" >"$FITDIR/stock-boot.sd.txt" 2>&1 || die "dumpimage cannot read the stock BOOT.SD: $(cat "$FITDIR/stock-boot.sd.txt")"
    grep -q "Configuration 0 ($CONF)" "$FITDIR/stock-boot.sd.txt" || die "the stock BOOT.SD has no configuration $CONF: $(cat "$FITDIR/stock-boot.sd.txt")"
    sed -n 's/^/[stock BOOT.SD] /p' "$FITDIR/stock-boot.sd.txt"

    # the kernel must be a riscv64 Image: magic "RSC\x05" at 0x38
    [ "$(echo $(od -An -tx1 -j56 -N4 "$KIMAGE"))" = "52 53 43 05" ] || die "$KIMAGE is not a riscv Image (no RSC\\x05 magic at 0x38)"
    case "$(od -An -tx1 -j0 -N2 "$KDTB" | tr -d ' ')" in d00d) ;; *) die "$KDTB is not a flat device tree" ;; esac

    # DTB: add the OpenSBI range the vendor kernel learns from riscv.fwsz
    cp -- "$KDTB" "$FITDIR/kernel.dtb"
    if [ $((FW_RESERVE)) -gt 0 ]; then
        dtc -I dtb -O dts -o "$FITDIR/kernel.dts" "$KDTB" 2>"$FITDIR/dtc.err" || die "dtc cannot decompile $KDTB: $(cat "$FITDIR/dtc.err")"
        grep -q '^/memreserve/' "$FITDIR/kernel.dts" && die "$KDTB already has /memreserve/ entries; refusing to add another"
        awk -v size="$FW_RESERVE" '
            { print }
            /^\/dts-v1\/;$/ && !done { print ""; print "/memreserve/ 0x80000000 " size ";"; done = 1 }
            END { if (!done) exit 1 }' "$FITDIR/kernel.dts" >"$FITDIR/kernel-res.dts" ||
            die "$KDTB decompiles without a /dts-v1/; line"
        dtc -I dts -O dtb -o "$FITDIR/kernel.dtb" "$FITDIR/kernel-res.dts" 2>"$FITDIR/dtc.err" || die "dtc cannot compile the DTB with /memreserve/: $(cat "$FITDIR/dtc.err")"
        dtc -I dtb -O dts -o "$FITDIR/kernel.check.dts" "$FITDIR/kernel.dtb" 2>/dev/null || die "dtc cannot re-read the new DTB"
        grep -q "^/memreserve/ 0x80000000 $FW_RESERVE;" "$FITDIR/kernel.check.dts" ||
            grep -q "^/memreserve/ 0x0*80000000 0x0*$(printf '%x' $((FW_RESERVE)));" "$FITDIR/kernel.check.dts" ||
            die "the new DTB lost its /memreserve/: $(grep memreserve "$FITDIR/kernel.check.dts")"
        info "DTB: /memreserve/ 0x80000000 $FW_RESERVE added"
    fi

    # the kernel, compressed (lzma: the .lzma header carries the real size, as lzma(1)
    # of a file writes it; xz leaves it unknown, which U-Boot's decoder is not told to expect)
    gzip -9 -n -c "$KIMAGE" >"$FITDIR/kernel.gz"
    xz --format=lzma -9e -c "$KIMAGE" >"$FITDIR/kernel.lzma" || die "xz --format=lzma failed"
    ksize=$(wc -c <"$KIMAGE")
    ks=$ksize
    : >"$FITDIR/size.le"
    i=0
    while [ $i -lt 8 ]; do
        printf "\\$(printf '%03o' $((ks % 256)))" >>"$FITDIR/size.le"
        ks=$((ks / 256))
        i=$((i + 1))
    done
    [ "$(wc -c <"$FITDIR/size.le")" = 8 ] || die "cannot build the 8-byte size field"
    dd if="$FITDIR/size.le" of="$FITDIR/kernel.lzma" bs=1 seek=5 conv=notrunc status=none || die "dd failed patching the .lzma size"
    xz --format=lzma -dc "$FITDIR/kernel.lzma" | cmp -s - "$KIMAGE" || die "the patched .lzma does not decompress to the kernel Image"
    gzip -dc "$FITDIR/kernel.gz" | cmp -s - "$KIMAGE" || die "the .gz does not decompress to the kernel Image"
    info "kernel Image $ksize bytes: lzma $(wc -c <"$FITDIR/kernel.lzma"), gzip $(wc -c <"$FITDIR/kernel.gz")"
    case "$FIT_COMP" in
    lzma) KDATA=$FITDIR/kernel.lzma ;;
    gzip) KDATA=$FITDIR/kernel.gz ;;
    esac

    cat >"$FITDIR/boot.its" <<EOF
/dts-v1/;

/ {
	description = "NanoKVM Cube: mainline kernel and DTB (sd-image.sh --kernel-fit)";
	#address-cells = <2>;

	images {
		kernel-1 {
			description = "mainline kernel";
			data = /incbin/("$KDATA");
			type = "kernel";
			arch = "riscv";
			os = "linux";
			compression = "$FIT_COMP";
			load = <0x0 $FIT_LOAD>;
			entry = <0x0 $FIT_LOAD>;
			hash-1 {
				algo = "crc32";
			};
		};
		fdt-sg2002_licheervnano_sd {
			description = "NanoKVM Cube device tree";
			data = /incbin/("$FITDIR/kernel.dtb");
			type = "flat_dt";
			arch = "riscv";
			compression = "none";
			hash-1 {
				algo = "sha256";
			};
		};
	};

	configurations {
		default = "$CONF";
		$CONF {
			description = "boot the NanoKVM Cube with its mainline kernel";
			kernel = "kernel-1";
			fdt = "fdt-sg2002_licheervnano_sd";
		};
	};
};
EOF
    MKIMAGE_DTC=$(command -v dtc)
    export MKIMAGE_DTC
    mkimage -f "$FITDIR/boot.its" "$FITDIR/boot.sd" >"$FITDIR/mkimage.out" 2>&1 || die "mkimage failed: $(cat "$FITDIR/mkimage.out")"
    dumpimage -l "$FITDIR/boot.sd" >"$FITDIR/boot.sd.txt" 2>&1 || die "dumpimage cannot read the new FIT: $(cat "$FITDIR/boot.sd.txt")"
    sed -n 's/^/[new BOOT.SD] /p' "$FITDIR/boot.sd.txt"
    fit_bytes=$(wc -c <"$FITDIR/boot.sd")

    # does it fit: the FAT's free space plus the BOOT.SD it replaces
    stock_fit_bytes=$(wc -c <"$FITDIR/stock-boot.sd")
    free_bytes=$(grep -i 'bytes free' "$FITDIR/mdir.before" | tr -cd '0-9')
    case "$free_bytes" in "" | *[!0-9]*) die "cannot read the free space of partition 1 from mdir" ;; esac
    [ "$fit_bytes" -le $((free_bytes + stock_fit_bytes - 65536)) ] ||
        die "the new BOOT.SD ($fit_bytes bytes) does not fit in partition 1 ($free_bytes bytes free + $stock_fit_bytes in the stock BOOT.SD)"
    [ "$fit_bytes" -lt $((FIT_LOAD - 0x81800000)) ] ||
        die "the new BOOT.SD ($fit_bytes bytes) would reach the kernel load address $FIT_LOAD when U-Boot loads it at 0x81800000"

    mdel -i "$BOOTFAT" ::BOOT.SD || die "mdel BOOT.SD failed"
    mcopy -i "$BOOTFAT" "$FITDIR/boot.sd" ::BOOT.SD || die "mcopy BOOT.SD failed"
    mdir -i "$BOOTFAT" :: >"$FITDIR/mdir.after" 2>&1 || die "mdir cannot read the edited partition 1: $(cat "$FITDIR/mdir.after")"
    mcopy -i "$BOOTFAT" ::BOOT.SD "$FITDIR/boot.sd.back" || die "mcopy cannot read BOOT.SD back"
    cmp -s "$FITDIR/boot.sd" "$FITDIR/boot.sd.back" || die "BOOT.SD read back from the boot partition differs from the FIT"
    # everything else on the partition is what the stock one holds
    for f in FIP.BIN; do
        mcopy -i "$BOOTFAT" "::$f" "$FITDIR/after.$f" || die "mcopy $f failed on the edited partition"
        mcopy -i "$STOCK@@$((P1_START * SECTOR))" "::$f" "$FITDIR/before.$f" || die "mcopy $f failed on the stock image"
        cmp -s "$FITDIR/before.$f" "$FITDIR/after.$f" || die "$f differs from the stock one"
    done
    grep -iv 'bytes free\|BOOT *SD' "$FITDIR/mdir.before" >"$FITDIR/mdir.before.rest"
    grep -iv 'bytes free\|BOOT *SD' "$FITDIR/mdir.after" >"$FITDIR/mdir.after.rest"
    cmp -s "$FITDIR/mdir.before.rest" "$FITDIR/mdir.after.rest" || die "the boot partition's directory differs beyond BOOT.SD: $(diff "$FITDIR/mdir.before.rest" "$FITDIR/mdir.after.rest")"
    sed -n 's/^/[boot partition] /p' "$FITDIR/mdir.after"

    # the kernel and DTB come back out of the FIT unchanged
    dumpimage -T flat_dt -p 0 -o "$FITDIR/out.kernel" "$FITDIR/boot.sd.back" >/dev/null 2>&1 || die "dumpimage cannot extract the kernel (image 0) from the FIT"
    case "$FIT_COMP" in
    lzma) xz --format=lzma -dc "$FITDIR/out.kernel" ;;
    gzip) gzip -dc "$FITDIR/out.kernel" ;;
    esac | cmp -s - "$KIMAGE" || die "the kernel in the FIT does not decompress to $KIMAGE"
    dumpimage -T flat_dt -p 1 -o "$FITDIR/out.dtb" "$FITDIR/boot.sd.back" >/dev/null 2>&1 || die "dumpimage cannot extract the DTB (image 1) from the FIT"
    cmp -s "$FITDIR/out.dtb" "$FITDIR/kernel.dtb" || die "the DTB in the FIT is not the one written"
    info "FIT verified: kernel Image sha256 $(sha_of "$KIMAGE"), DTB sha256 $(sha_of "$FITDIR/kernel.dtb") (input DTB $(sha_of "$KDTB")), BOOT.SD $fit_bytes bytes, load/entry $FIT_LOAD, $FIT_COMP"
fi

# ------------------------------------------------------------- 7. splice back

IMG=$T/image.img
cp -- "$STOCK" "$IMG"
if [ "$KFIT" = 1 ]; then
    HEAD=$P1_START # the sectors before partition 1 stay stock
else
    HEAD=$P2_START
fi
dd if="$IMG" of="$T/head.before" bs="$SECTOR" count="$HEAD" status=none || die "dd failed reading the head"
dd if="$FS" of="$IMG" bs="$SECTOR" seek="$P2_START" conv=notrunc status=none || die "dd failed splicing the rootfs back"
if [ "$KFIT" = 1 ]; then
    dd if="$BOOTFAT" of="$IMG" bs="$SECTOR" seek="$P1_START" conv=notrunc status=none || die "dd failed splicing the boot partition back"
    dd if="$IMG" of="$T/p1.check" bs="$SECTOR" skip="$P1_START" count="$P1_SIZE" status=none || die "dd failed re-reading p1"
    cmp -s "$BOOTFAT" "$T/p1.check" || die "partition 1 read back from the image differs from the edited boot partition"
    # the sectors between partition 1 and 2 (none on the stock layout) stay stock
    cmp -s -i $(((P1_START + P1_SIZE) * SECTOR)):$(((P1_START + P1_SIZE) * SECTOR)) -n $(((P2_START - P1_START - P1_SIZE) * SECTOR)) "$STOCK" "$IMG" ||
        die "bytes between partition 1 and 2 differ from the stock image"
fi
[ "$(wc -c <"$IMG")" -eq "$img_bytes" ] || die "image size changed during the splice"
dd if="$IMG" of="$T/head.after" bs="$SECTOR" count="$HEAD" status=none || die "dd failed reading the head"
cmp -s "$T/head.before" "$T/head.after" || die "bytes before partition $([ "$KFIT" = 1 ] && echo 1 || echo 2) changed"
cmp -s -n $((HEAD * SECTOR)) "$STOCK" "$IMG" || die "bytes before partition $([ "$KFIT" = 1 ] && echo 1 || echo 2) differ from the stock image"
dd if="$IMG" of="$T/p2.check" bs="$SECTOR" skip="$P2_START" count="$P2_SIZE" status=none || die "dd failed re-reading p2"
cmp -s "$FS" "$T/p2.check" || die "partition 2 read back from the image differs from the edited rootfs"
# whatever follows p2 in the stock image is unchanged
cmp -s -i $(((P2_START + P2_SIZE) * SECTOR)) "$STOCK" "$IMG" || die "bytes after partition 2 differ from the stock image"

mv -- "$IMG" "$OUT"
sum=$(sha_of "$OUT")
printf '%s  %s\n' "$sum" "$(basename "$OUT")" >"$OUT.sha256"
info "wrote $OUT ($img_bytes bytes, release $VERSION)"
info "sha256 $sum"
info "flash it to the microSD card (e.g. dd if=$(basename "$OUT") of=/dev/rdiskN bs=4m on macOS, or balenaEtcher)."
info "first boot: the root password is /boot/root.pass if present, else shown on the OLED (also /etc/kvm/initial.pass) and changed at first login; web user root."
