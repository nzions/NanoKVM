#!/bin/bash
#
# Compare two riscv64 musl gcc toolchains byte for byte: the one MaixCDK downloads
# (host-tools-2025.7.28) and the Sophgo one baked into the build image.
#
# Usage: scripts/toolchain-compare.sh <toolchain-root-A> <toolchain-root-B> <outdir>
#   a toolchain root is the directory holding bin/ and sysroot/
#   (.../gcc/riscv64-linux-musl-x86_64)
#
# Writes <outdir>/toolchains.txt (side-by-side sha256 table), <outdir>/gcc-v-A.txt and
# gcc-v-B.txt, and prints one verdict line:
#   [evidence] toolchains: IDENTICAL
#   [evidence] toolchains: DIFFER (<n> files): <names>
# Informational: a difference is NOT an error. A tool or file that cannot be found IS
# (fail closed), as is an empty output.

set -eu
set -o pipefail

die() {
    echo "[toolchain-compare] ERROR: $*" >&2
    exit 1
}

[ $# -eq 3 ] || die "usage: $0 <toolchain-root-A> <toolchain-root-B> <outdir>"
TC_A="$1"
TC_B="$2"
OUT="$3"
for tc in "$TC_A" "$TC_B"; do
    [ -x "$tc/bin/riscv64-unknown-linux-musl-gcc" ] || die "$tc/bin/riscv64-unknown-linux-musl-gcc is missing"
    [ -d "$tc/sysroot" ] || die "$tc/sysroot is missing"
done
mkdir -p "$OUT"

# real path of what <toolchain> resolves <kind> <name> to; kind: bin (<root>/bin/<name>),
# prog (gcc -print-prog-name) or file (gcc -print-file-name). gcc echoes the bare name
# back when it finds nothing.
resolve() {
    local tc="$1" kind="$2" name="$3" path
    if [ "$kind" = bin ]; then
        path="$tc/bin/$name"
    else
        path="$("$tc/bin/riscv64-unknown-linux-musl-gcc" "-print-$kind-name=$name")" \
            || die "$tc: gcc -print-$kind-name=$name failed"
    fi
    case "$path" in
        /*) ;;
        *) die "$tc: gcc does not know $kind '$name' (got '$path')" ;;
    esac
    path="$(readlink -f "$path")" || die "$tc: cannot resolve $path"
    [ -f "$path" ] || die "$tc: $name resolves to $path, which is not a file"
    printf '%s\n' "$path"
}

CC="riscv64-unknown-linux-musl-gcc"
ROWS="$OUT/toolchains.rows"
: > "$ROWS"
differ=0
differing=""

row() {
    local label="$1" kind="$2" name="$3" pa pb sa sb verdict
    pa="$(resolve "$TC_A" "$kind" "$name")"
    pb="$(resolve "$TC_B" "$kind" "$name")"
    sa="$(sha256sum "$pa" | cut -d' ' -f1)"
    sb="$(sha256sum "$pb" | cut -d' ' -f1)"
    if [ "$sa" = "$sb" ]; then
        verdict=same
    else
        verdict=DIFFER
        differ=$((differ + 1))
        differing="$differing $label"
    fi
    printf '%-14s %s  %s  %s  %s | %s\n' "$label" "$verdict" "$sa" "$sb" "${pa#"$TC_A"/}" "${pb#"$TC_B"/}" >> "$ROWS"
}

# the compiler driver and the real compiler / assembler / linker it runs
row gcc bin "$CC"
row g++ bin riscv64-unknown-linux-musl-g++
row cc1 prog cc1
row cc1plus prog cc1plus
row collect2 prog collect2
row as bin riscv64-unknown-linux-musl-as
row ld bin riscv64-unknown-linux-musl-ld
# the runtime the binaries are linked against and run with
row libc.so file libc.so
row libstdc++.so.6 file libstdc++.so.6
row libatomic.so.1 file libatomic.so.1
row libgcc_s.so.1 file libgcc_s.so.1

for which in A B; do
    tc="$TC_A"
    [ "$which" = B ] && tc="$TC_B"
    "$tc/bin/$CC" -v > "$OUT/gcc-v-$which.txt" 2>&1 || die "$tc: gcc -v failed"
    [ -s "$OUT/gcc-v-$which.txt" ] || die "$tc: gcc -v printed nothing"
done

{
    echo "A = $TC_A"
    echo "B = $TC_B"
    echo "columns: file, verdict, sha256 A, sha256 B, real path in A | real path in B"
    cat "$ROWS"
    if [ "$differ" -eq 0 ]; then
        echo "[evidence] toolchains: IDENTICAL"
    else
        echo "[evidence] toolchains: DIFFER ($differ files):$differing"
    fi
} > "$OUT/toolchains.txt"
rm -f "$ROWS"
[ -s "$OUT/toolchains.txt" ] || die "toolchains.txt is empty"

cat "$OUT/toolchains.txt"
