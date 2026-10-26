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
# The inputs are only read. Needs: debugfs e2fsck sfdisk dd tar find sort cmp
# diff od sha256sum awk (e2fsprogs, util-linux, coreutils).
#
# Usage:
#   scripts/sd-image.sh [--stock-sha256 <hex>] [--sha256 <hex>] \
#       <stock.img> <nanokvm_VERSION.tar.gz> <out.img>
#
#   --sha256        expected sha256 of the release tarball; default: read from
#                   <tarball>.sha256 next to it. One of the two is required.
#   --stock-sha256  expected sha256 of the stock image; default: the
#                   20260610_NanoKVM_Rev1_4_3.img below.
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
Usage: $0 [--stock-sha256 <hex>] [--sha256 <hex>] <stock.img> <nanokvm_VERSION.tar.gz> <out.img>
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
    -*) usage ;;
    *) break ;;
    esac
done
[ $# -eq 3 ] || usage
STOCK=$1
TARBALL=$2
OUT=$3

for t in debugfs e2fsck sfdisk dd tar find sort cmp diff od sha256sum awk grep mktemp wc tr; do
    need_tool "$t"
done

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

pt=$(LC_ALL=C sfdisk -d "$STOCK" 2>&1) || die "sfdisk -d failed on $STOCK: $pt"
SECTOR=$(echo "$pt" | awk -F': *' '$1 == "sector-size" { print $2 }')
SECTOR=${SECTOR:-512}
P2=$(echo "$pt" | awk -v a="${STOCK}2" -v b="${STOCK}p2" '
    $1 == a || $1 == b {
        n = split($0, f, /[ ,=:]+/)
        for (i = 1; i < n; i++) {
            if (f[i] == "start") s = f[i + 1]
            if (f[i] == "size") z = f[i + 1]
        }
        print s, z
    }')
[ -n "$P2" ] || die "partition 2 not found in the partition table of $STOCK"
P2_START=${P2% *}
P2_SIZE=${P2#* }
case "$P2_START$P2_SIZE$SECTOR" in
"" | *[!0-9]*) die "cannot parse partition 2 geometry from sfdisk: '$P2' sector '$SECTOR'" ;;
esac
[ "$P2_START" -gt 0 ] && [ "$P2_SIZE" -gt 0 ] || die "partition 2 has a zero start or size: '$P2'"
img_bytes=$(wc -c <"$STOCK")
[ $(((P2_START + P2_SIZE) * SECTOR)) -le "$img_bytes" ] ||
    die "partition 2 (start $P2_START, size $P2_SIZE sectors) runs past the end of $STOCK"
info "partition 2: start sector $P2_START, $P2_SIZE sectors of $SECTOR bytes"

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

IMG=$T/image.img
cp -- "$STOCK" "$IMG"
dd if="$IMG" of="$T/head.before" bs="$SECTOR" count="$P2_START" status=none || die "dd failed reading the head"
dd if="$FS" of="$IMG" bs="$SECTOR" seek="$P2_START" conv=notrunc status=none || die "dd failed splicing the rootfs back"
[ "$(wc -c <"$IMG")" -eq "$img_bytes" ] || die "image size changed during the splice"
dd if="$IMG" of="$T/head.after" bs="$SECTOR" count="$P2_START" status=none || die "dd failed reading the head"
cmp -s "$T/head.before" "$T/head.after" || die "bytes before partition 2 changed"
cmp -s -n $((P2_START * SECTOR)) "$STOCK" "$IMG" || die "bytes before partition 2 differ from the stock image"
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
