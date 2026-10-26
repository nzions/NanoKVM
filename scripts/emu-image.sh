#!/bin/bash
#
# Turn Sipeed's stock NanoKVM SD image into a QEMU-bootable emulator image.
#
# The stock image boots on an SG2002 (T-Head C906 + RVV 0.7 + the cvi video
# hardware). QEMU's thead-c906 lacks the vector instructions Sipeed's xthead
# musl loaders use, and there is no /dev/cvi-* for kvm_system. This script
# edits a COPY of the image's rootfs (partition 2) so it boots and serves the
# web UI under `qemu-system-riscv64 -M virt -cpu thead-c906`:
#
#   1. the hard-float xthead musl loaders in /lib (-> usr/lib) become
#      symlinks to the plain loader
#   2. our riscv64 server is installed as /kvmapp/server/NanoKVM-Server
#   3. /etc/init.d/S95nanokvm loses its kvm_system line (original kept as
#      /etc/S95nanokvm.orig, outside init.d so rcS does not run it)
#   4. /etc/inittab becomes a busybox inittab (original kept as inittab.orig)
#   5. /usr/bin/init becomes a symlink to /bin/busybox
#   6. /etc/kvm/ssh_stop is removed so sshd starts
#   7. /etc/kvm/emulator is written: our S95nanokvm (the supervised one an OTA
#      brings) then does not start kvm_system, which the step-3 edit cannot
#      remove from it
#
# No root and no loop mount: the rootfs is cut out with dd, edited with
# debugfs -w, checked with e2fsck and spliced back with dd conv=notrunc.
# The stock image is only read. See scripts/README.md for the qemu command.
#
# An online update (OTA) replaces /kvmapp wholesale: it brings back the
# hardware (cgo) server and an S95nanokvm that starts kvm_system, and both
# segfault under qemu. --refresh re-applies the two edits that matter after an
# OTA (steps 2 and 3) IN PLACE on an image this script already processed:
# it reinstalls the nokvm server, copies /kvmapp/system/init.d/S95nanokvm over
# /etc/init.d/S95nanokvm (what kvm_system does on a device; it never runs under
# qemu, so the boot copy would otherwise stay stale), and strips the kvm_system
# start line from both
# S95nanokvm copies that run (/etc/init.d/S95nanokvm, which rcS runs at boot,
# and /kvmapp/system/init.d/S95nanokvm, which the OTA runs for `restart`).
# Run it with the qemu VM STOPPED; it refuses an image without
# /etc/S95nanokvm.orig. Same safety style: dd cut-out, debugfs -w, e2fsck,
# dd conv=notrunc splice back.
#
# Usage:
#   scripts/emu-image.sh [--sha256 <hex>] [--authorized-key <pubfile>] <stock.img> <NanoKVM-Server> <out.img>
#   scripts/emu-image.sh --refresh <img> <NanoKVM-Server>
#
# --authorized-key <pubfile>: authorise the OpenSSH public key(s) in <pubfile>
# for root in the image: /root/.ssh (0700) and /root/.ssh/authorized_keys (0600),
# both root:root. The image otherwise carries no credential.
#
# NanoKVM-Server: CGO_ENABLED=0 GOOS=linux GOARCH=riscv64 go build -tags nokvm
# (from server/). <out.img> must not exist.
#
# The kernel is NOT made here: it is forge's fleet native build
# kernel-nanokvm-emu (riscv64, CONFIG_ERRATA_THEAD=n, netfilter built in),
# landing at <share>/fleet/artifacts/kernel-nanokvm-emu/<version>/Image. See
# scripts/README.md.

set -euo pipefail

QEMU_CMD() {
    cat <<EOF
qemu-system-riscv64 -M virt -cpu thead-c906 -smp 1 -m 512 -bios default \\
  -kernel <kernel-nanokvm-emu Image> \\
  -append 'root=/dev/vda2 rw console=ttyS0 init=/usr/bin/init' \\
  -drive file=$1,format=raw,if=virtio \\
  -netdev user,id=n0,hostfwd=tcp::18443-:443,hostfwd=tcp::18080-:80 \\
  -device virtio-net-device,netdev=n0 -nographic
EOF
}

die() {
    echo "[ERROR] $*" >&2
    exit 1
}

info() {
    echo "[emu-image] $*"
}

usage() {
    cat >&2 <<EOF
Usage: $0 [--sha256 <hex>] [--authorized-key <pubfile>] <stock.img> <NanoKVM-Server> <out.img>
       $0 --refresh <img> <NanoKVM-Server>   (in place, after an OTA; VM stopped)
EOF
    exit 1
}

need_tool() {
    command -v "$1" >/dev/null 2>&1 || die "required tool not found on PATH: $1 ($2)"
}

sha_of() {
    local out
    out=$(sha256sum "$1") || die "sha256sum failed on $1"
    echo "${out%% *}"
}

# ------------------------------------------------------------------ arguments

WANT_SHA=""
AUTH_KEY=""
MODE=build
if [ "${1:-}" = "--refresh" ]; then
    [ $# -eq 3 ] || usage
    MODE=refresh
    # In refresh mode STOCK is the already processed image, edited in place.
    STOCK="$2"
    SERVER="$3"
    OUT=""
else
    while [ $# -gt 0 ]; do
        case "$1" in
        --sha256)
            [ $# -ge 2 ] || usage
            WANT_SHA="$2"
            shift 2
            ;;
        --authorized-key)
            [ $# -ge 2 ] || usage
            AUTH_KEY="$2"
            shift 2
            ;;
        *) break ;;
        esac
    done
    [ $# -eq 3 ] || usage
    STOCK="$1"
    SERVER="$2"
    OUT="$3"
fi

if [ -n "$WANT_SHA" ]; then
    echo "$WANT_SHA" | grep -qE '^[0-9a-fA-F]{64}$' || die "--sha256 must be 64 hex digits"
fi
if [ -n "$AUTH_KEY" ]; then
    [ -f "$AUTH_KEY" ] || die "--authorized-key file not found: $AUTH_KEY"
    ! grep -q 'PRIVATE KEY' "$AUTH_KEY" || die "--authorized-key $AUTH_KEY holds a private key; give the .pub file"
    grep -qE '^(ssh-(ed25519|rsa)|ecdsa-sha2-[a-z0-9]+|sk-[a-z0-9@.-]+) [A-Za-z0-9+/=]+' "$AUTH_KEY" ||
        die "--authorized-key $AUTH_KEY holds no OpenSSH public key line"
fi
for t in debugfs e2fsck sfdisk dd od sha256sum cmp awk grep mktemp cp diff; do
    case "$t" in
    debugfs | e2fsck) need_tool "$t" "e2fsprogs" ;;
    sfdisk) need_tool "$t" "util-linux" ;;
    *) need_tool "$t" "base system" ;;
    esac
done

if [ "$MODE" = refresh ]; then
    [ -f "$STOCK" ] || die "image not found: $STOCK"
else
    [ -f "$STOCK" ] || die "stock image not found: $STOCK"
fi
[ -f "$SERVER" ] || die "server binary not found: $SERVER"
if [ "$MODE" = refresh ]; then
    OUT_DIR="$(cd "$(dirname "$STOCK")" 2>/dev/null && pwd)" || die "image directory does not exist: $(dirname "$STOCK")"
else
    [ ! -e "$OUT" ] || die "refusing to overwrite existing file: $OUT"
    OUT_DIR="$(cd "$(dirname "$OUT")" 2>/dev/null && pwd)" || die "output directory does not exist: $(dirname "$OUT")"
    OUT="$OUT_DIR/$(basename "$OUT")"
fi

# The server must be a riscv64 ELF: magic 7f 45 4c 46, class 2 (64-bit),
# little-endian, e_machine 0xF3 (243). Anything else would boot to nothing.
elf=$(od -An -tx1 -N20 "$SERVER") || die "cannot read $SERVER"
elf=$(echo $elf)
case "$elf" in
"7f 45 4c 46 02 01 "*" f3 00") ;;
*) die "$SERVER is not a riscv64 ELF (header: $elf)" ;;
esac

if [ -n "$WANT_SHA" ]; then
    have=$(sha_of "$STOCK")
    want=$(echo "$WANT_SHA" | tr 'A-F' 'a-f')
    [ "$have" = "$want" ] || die "stock image sha256 is $have, expected $want"
    info "stock image sha256 verified"
fi

# --------------------------------------------------------------- partition p2

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

T=$(mktemp -d "$OUT_DIR/.emu-image.XXXXXX") || die "cannot create a temp dir in $OUT_DIR"
cleanup() {
    rm -rf -- "$T"
}
trap cleanup EXIT

FS="$T/rootfs.ext4"
IMG="$T/image.img"

extract_p2() { # <image> <dest>
    dd if="$1" of="$2" bs="$SECTOR" skip="$P2_START" count="$P2_SIZE" status=none ||
        die "dd failed extracting partition 2"
}

# debugfs helpers. debugfs exits 0 even when a command failed, so every write
# is verified afterwards by reading it back; nothing here is trusted.
dfs() { # read-only command, prints its output
    debugfs -R "$1" "$FS" 2>/dev/null
}
dfsw() { # write command
    debugfs -w -R "$1" "$FS" >"$T/dfsw.out" 2>&1 || die "debugfs '$1' failed: $(cat "$T/dfsw.out")"
}
exists() {
    local o
    o=$(dfs "stat $1") || return 1
    case "$o" in *"Inode:"*) return 0 ;; *) return 1 ;; esac
}
stat_of() {
    local o
    o=$(dfs "stat $1")
    case "$o" in *"Inode:"*) echo "$o" ;; *) die "$1 not found in the image" ;; esac
}
dump_to() { # <fs path> <host file>
    rm -f -- "$2"
    dfs "dump $1 $2" >/dev/null
    [ -f "$2" ] || die "could not read $1 out of the image"
}
mode_is() { # <fs path> <perm digits, e.g. 755>
    local o
    o=$(stat_of "$1")
    [[ $o =~ Mode:\ +0*$2([^0-9]|$) ]] || die "$1 does not have mode $2 after the edit"
}
# Replace a regular file: rm, write, set mode, root:root, then read it back.
put_file() { # <host file> <fs path> <perm digits>
    if exists "$2"; then dfsw "rm $2"; fi
    dfsw "write $1 $2"
    dfsw "sif $2 mode 0100$3"
    dfsw "sif $2 uid 0"
    dfsw "sif $2 gid 0"
    dump_to "$2" "$T/readback"
    cmp -s "$1" "$T/readback" || die "$2: contents read back from the image differ from what was written (out of space?)"
    mode_is "$2" "$3"
}
# Replace a path with a symlink, then check type and destination.
put_symlink() { # <fs path> <target>
    if exists "$1"; then dfsw "rm $1"; fi
    dfsw "symlink $1 $2"
    local o
    o=$(stat_of "$1")
    [[ $o == *"Type: symlink"* ]] || die "$1 is not a symlink after the edit"
    [[ $o == *"\"$2\""* ]] || die "$1 does not point at $2 after the edit: $o"
}

extract_p2 "$STOCK" "$FS"
root_stat=$(dfs "stat /")
[[ $root_stat == *"Type: directory"* ]] || die "partition 2 of $STOCK is not an ext2/3/4 filesystem"

# A previously booted image carries needs_recovery; replaying the journal later
# would silently revert debugfs edits. Settle the filesystem first.
fsck() {
    local rc=0
    e2fsck -fy "$FS" >"$T/fsck.out" 2>&1 || rc=$?
    [ "$rc" -le 1 ] || die "e2fsck -fy ($1) exited $rc: $(cat "$T/fsck.out")"
    info "e2fsck ($1): exit $rc"
}
fsck "before edits"

# Drop the kvm_system start line from the S95nanokvm at <fs path> (shared by
# the build step 3 and --refresh). Only lines whose command is kvm_system (bare
# or by path, optional trailing args/&) go; "killall kvm_system" style lines
# stay. With <allow_none>=1 a file that has no such line counts as success
# (already refreshed); with 0 it is fatal.
KVM_SYSTEM_RE='^[[:space:]]*(/[^[:space:]]*/)?kvm_system([[:space:]]|$)'
strip_kvm_system() { # <fs path> <allow_none 0|1>
    local path="$1" allow_none="$2"
    exists "$path" || die "$path not found in the image"
    dump_to "$path" "$T/s95.in"
    grep -vE "$KVM_SYSTEM_RE" "$T/s95.in" >"$T/s95.out" || true
    if cmp -s "$T/s95.in" "$T/s95.out"; then
        [ "$allow_none" = 1 ] || die "no line starting kvm_system found in $path; edit did not apply"
        info "$path has no kvm_system start line: already refreshed, nothing to do"
        return 0
    fi
    if grep -qE "$KVM_SYSTEM_RE" "$T/s95.out"; then
        die "kvm_system start line still present in $path after edit"
    fi
    sh -n "$T/s95.out" || die "edited $path does not parse (an emptied if/case body?)"
    put_file "$T/s95.out" "$path" 755
    info "$path diff (original -> edited):"
    diff -u "$T/s95.in" "$T/s95.out" || true
}

# Write /etc/kvm/emulator (shared by step 7 and --refresh): our supervised
# S95nanokvm starts kvm_system through a function strip_kvm_system cannot see,
# and skips it when this marker exists.
mark_emulator() {
    exists /etc/kvm || die "/etc/kvm does not exist in the image"
    : >"$T/emulator"
    put_file "$T/emulator" /etc/kvm/emulator 644
    exists /etc/kvm/emulator || die "/etc/kvm/emulator missing after write"
    info "wrote /etc/kvm/emulator"
}

# Splice the edited rootfs ($FS) into <target image> over partition 2 and verify:
# same size, bytes before p2 untouched, p2 reads back equal to $FS.
splice_back() { # <target image>
    local target="$1" size_before
    size_before=$(wc -c <"$target")
    dd if="$target" of="$T/head.before" bs="$SECTOR" count="$P2_START" status=none ||
        die "dd failed reading the bytes before partition 2"
    dd if="$FS" of="$target" bs="$SECTOR" seek="$P2_START" conv=notrunc status=none ||
        die "dd failed splicing the rootfs back"
    [ "$(wc -c <"$target")" -eq "$size_before" ] || die "output image size changed during the splice"
    # Everything before p2 (MBR, bootloader partition) must be untouched.
    dd if="$target" of="$T/head.after" bs="$SECTOR" count="$P2_START" status=none ||
        die "dd failed reading the bytes before partition 2"
    cmp -s "$T/head.before" "$T/head.after" || die "bytes before partition 2 changed"
    extract_p2 "$target" "$T/p2.check"
    cmp -s "$FS" "$T/p2.check" || die "partition 2 read back from $target differs from the edited rootfs"
}

# ------------------------------------------------------------- --refresh mode

if [ "$MODE" = refresh ]; then
    exists /etc/S95nanokvm.orig ||
        die "/etc/S95nanokvm.orig not found: $STOCK does not look processed by emu-image.sh; build it from the stock image instead"
    exists /kvmapp/server || die "/kvmapp/server does not exist in the image: not a NanoKVM rootfs"
    put_file "$SERVER" /kvmapp/server/NanoKVM-Server 755
    info "server installed ($(wc -c <"$SERVER") bytes)"
    # kvm_system never runs under qemu, so nothing refreshes /etc/init.d/S95nanokvm
    # from the OTA's copy. Do what kvm_system's system_init does on a device:
    # install /kvmapp/system/init.d/S95nanokvm as /etc/init.d/S95nanokvm.
    exists /kvmapp/system/init.d/S95nanokvm ||
        die "/kvmapp/system/init.d/S95nanokvm not found in the image"
    dump_to /kvmapp/system/init.d/S95nanokvm "$T/s95.new"
    put_file "$T/s95.new" /etc/init.d/S95nanokvm 755
    info "/etc/init.d/S95nanokvm installed from /kvmapp/system/init.d/S95nanokvm"
    # Both copies run: /etc/init.d/S95nanokvm at boot (rcS), and
    # /kvmapp/system/init.d/S95nanokvm for the `restart` an OTA issues.
    strip_kvm_system /etc/init.d/S95nanokvm 1
    strip_kvm_system /kvmapp/system/init.d/S95nanokvm 1
    mark_emulator
    fsck "after edits"
    exists /kvmapp/server/NanoKVM-Server || die "server vanished during the final e2fsck"
    splice_back "$STOCK"
    info "refreshed $STOCK in place"
    info "sha256 $(sha_of "$STOCK")  $STOCK"
    exit 0
fi

# -------------------------------------------------- 1. xthead loaders -> musl

PLAIN=/lib/ld-musl-riscv64.so.1
exists "$PLAIN" || die "$PLAIN not found in the image (or /lib is a symlink); cannot redirect the xthead loaders"
plain_stat=$(stat_of "$PLAIN")
if [[ $plain_stat == *"Type: symlink"* ]]; then
    info "$PLAIN is a symlink: $(echo "$plain_stat" | grep -o 'Fast link dest: .*' || echo '(slow link)')"
fi
# The stock rootfs has /lib -> usr/lib; debugfs ls does not follow a symlink.
LIBDIR=/lib
lib_stat=$(stat_of /lib)
if [[ $lib_stat == *"Type: symlink"* ]]; then
    [[ $lib_stat =~ Fast\ link\ dest:\ \"([^\"]+)\" ]] || die "/lib is a symlink whose destination could not be read: $lib_stat"
    LIBDIR=${BASH_REMATCH[1]}
    [[ $LIBDIR == /* ]] || LIBDIR=/$LIBDIR
    info "/lib -> $LIBDIR"
fi
lib_ls=$(dfs "ls -l $LIBDIR")
LOADERS=()
while read -r name; do
    case "$name" in
    # -sf = the soft-float ABI; the plain loader is hard-float, leave it alone.
    ld-musl-riscv64*xthead*-sf.so.1) ;;
    ld-musl-riscv64*xthead*) LOADERS+=("$name") ;;
    esac
done < <(echo "$lib_ls" | awk 'NF >= 7 { print $NF }')
[ "${#LOADERS[@]}" -gt 0 ] || die "no ld-musl-riscv64*xthead* loader found in $LIBDIR; listing was: $lib_ls"
info "xthead loaders found: ${LOADERS[*]}"
for l in "${LOADERS[@]}"; do
    put_symlink "$LIBDIR/$l" "$PLAIN"
done

# -------------------------------------------------------- 2. our server

exists /kvmapp/server || die "/kvmapp/server does not exist in the image: not a NanoKVM rootfs"
put_file "$SERVER" /kvmapp/server/NanoKVM-Server 755
info "server installed ($(wc -c <"$SERVER") bytes)"

# ------------------------------------------------ 3. S95nanokvm sans kvm_system

S95=/etc/init.d/S95nanokvm
# The original is kept outside init.d: rcS runs every S??* there, .orig included.
S95ORIG=/etc/S95nanokvm.orig
exists "$S95" || die "$S95 not found in the image"
exists "$S95ORIG" && die "$S95ORIG already exists: this image looks already processed; start from the stock image"
dump_to "$S95" "$T/S95.orig"
put_file "$T/S95.orig" "$S95ORIG" 644
strip_kvm_system "$S95" 0

# ---------------------------------------------------------- 4. busybox inittab

exists /etc/inittab || die "/etc/inittab not found in the image"
exists /etc/inittab.orig && die "/etc/inittab.orig already exists: this image looks already processed"
dump_to /etc/inittab "$T/inittab.orig"
dump_to /bin/busybox "$T/busybox"
# The stock inittab's si0..si10 sysinit lines, in busybox syntax: rcS assumes
# /proc, /sys, /run, /tmp are mounted (the server's os.Executable reads
# /proc/self/exe, sshd needs /var/lock).
exists /sbin/getty || die "/sbin/getty not found in the image"
CONSOLE="console::respawn:/sbin/getty -L console 0 vt100"
if exists /etc/console_handler.sh; then CONSOLE="$CONSOLE -l /etc/console_handler.sh"; fi
{
    echo "# busybox-init inittab for emulation (sysvinit stalls after its first sysinit child under QEMU)"
    echo "::sysinit:/bin/mount -t proc proc /proc"
    echo "::sysinit:/bin/mount -o remount,rw /"
    echo "::sysinit:/bin/mkdir -p /dev/pts /dev/shm"
    echo "::sysinit:/bin/mount -a"
    echo "::sysinit:/bin/mkdir -p /run/lock/subsys /var/lock"
    echo "::sysinit:/bin/ln -sf /proc/self/fd /dev/fd"
    echo "::sysinit:/bin/hostname -F /etc/hostname"
    echo "::sysinit:/etc/init.d/rcS"
    echo "$CONSOLE"
    echo "::ctrlaltdel:/sbin/reboot"
    if exists /etc/init.d/rcK; then echo "::shutdown:/etc/init.d/rcK"; fi
    echo "::shutdown:/bin/umount -a -r"
} >"$T/inittab.new"
put_file "$T/inittab.orig" /etc/inittab.orig 644
put_file "$T/inittab.new" /etc/inittab 644
info "/etc/inittab:"
cat "$T/inittab.new"

# --------------------------------------------------- 5. /usr/bin/init -> busybox

grep -aq 'sysinit' "$T/busybox" || die "/bin/busybox has no init applet (no 'sysinit' string); cannot use it as init"
if exists /usr/bin/init; then
    info "/usr/bin/init exists ($(stat_of /usr/bin/init | grep -o 'Type: [a-z]*')); replacing"
else
    info "/usr/bin/init does not exist; creating"
    exists /usr/bin || die "/usr/bin does not exist in the image"
fi
put_symlink /usr/bin/init /bin/busybox

# ------------------------------------------------------------ 6. ssh_stop

if exists /etc/kvm/ssh_stop; then
    dfsw "rm /etc/kvm/ssh_stop"
    ! exists /etc/kvm/ssh_stop || die "/etc/kvm/ssh_stop is still present after rm"
    info "removed /etc/kvm/ssh_stop"
else
    info "/etc/kvm/ssh_stop not present (nothing to remove)"
fi

# ------------------------------------------------------------ 7. emulator mark

mark_emulator

# ------------------------------------------------ 8. root's authorized key

if [ -n "$AUTH_KEY" ]; then
    exists /root || die "/root does not exist in the image"
    if ! exists /root/.ssh; then
        dfsw "mkdir /root/.ssh"
    fi
    dfsw "sif /root/.ssh mode 040700"
    dfsw "sif /root/.ssh uid 0"
    dfsw "sif /root/.ssh gid 0"
    mode_is /root/.ssh 700
    put_file "$AUTH_KEY" /root/.ssh/authorized_keys 600
    info "authorised $(grep -cE '^(ssh-|ecdsa-|sk-)' "$AUTH_KEY") key(s) from $AUTH_KEY for root"
fi

# ------------------------------------------------- check, splice back, verify

fsck "after edits"
# Re-assert one of the edits after fsck: fsck must not have undone them.
exists /usr/bin/init || die "/usr/bin/init vanished during the final e2fsck"

cp -- "$STOCK" "$IMG"
splice_back "$IMG"
[ "$(wc -c <"$IMG")" -eq "$img_bytes" ] || die "output image size changed during the splice"
cmp -s -n $((P2_START * SECTOR)) "$STOCK" "$IMG" || die "bytes before partition 2 differ from the stock image"

mv -- "$IMG" "$OUT"
info "wrote $OUT"
info "sha256 $(sha_of "$OUT")  $OUT"
info "boot it with (kernel: forge's kernel-nanokvm-emu, see scripts/README.md):"
QEMU_CMD "$OUT"
