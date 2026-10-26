#!/bin/bash
#
# Golden ABI manifest check: hold a build's binaries to the interface of the 2.6.38
# MaixCDK-built ones (kvm_system, libkvm.so, libkvm_mmf.so).
#
# Usage:
#   tools/binabi/check.sh <binprint-dir>            compare against tools/binabi/golden
#   tools/binabi/check.sh --extract <binprint-dir> <outdir>
#                                                   write the golden fields of every binary
# <binprint-dir> is scripts/binprint.sh's output (<bin>.dynamic, .exports, .imports,
# .attributes, .comment, .size). Fields per binary (golden/<bin>.<field>):
#   needed      DT_NEEDED, in link order      soname   DT_SONAME or "none"
#   rpath       DT_RPATH/RUNPATH or "none"    exports  sorted defined dynamic symbols
#   imports     sorted undefined symbols      attributes  readelf -A Tag_ lines
#   comment     .comment string (offset stripped)
#   sections    ".text", ".rodata", ".data", ".bss" sizes from size -A (exact; the
#               tolerance is an allow.txt entry per binary, with a reason)
#
# allow.txt: "<binary> <field> <line> # <reason>". <line> is one differing line, literal
# (a golden line that is gone, or a line the build has that the golden lacks). Anything
# not listed fails; so does an entry that matches no difference, and so does a missing
# tool or an empty input. Nothing is skipped.

set -u
set -o pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
GOLDEN="${BINABI_GOLDEN:-$HERE/golden}"
ALLOW="${BINABI_ALLOW:-$HERE/allow.txt}"
FIELDS="needed soname rpath exports imports attributes comment sections"

die() {
    echo "[binabi] ERROR: $*" >&2
    exit 1
}

for t in awk sed sort diff mktemp; do
    command -v "$t" >/dev/null 2>&1 || die "'$t' is missing (a skipped check is a failed one)"
done

need() { # need <file>: must exist and be non-empty
    [ -f "$1" ] || die "$1 is missing"
    [ -s "$1" ] || die "$1 is empty"
}

# extract <dir> <bin> <outdir>: write the eight fields of one binary.
extract() {
    local d="$1" b="$2" o="$3" f
    for f in dynamic exports imports attributes comment size; do need "$d/$b.$f"; done
    mkdir -p "$o" || die "mkdir $o failed"
    sed -n 's/.*(NEEDED) *Shared library: \[\(.*\)\]$/\1/p' "$d/$b.dynamic" > "$o/$b.needed" || die "$b: needed"
    awk '/\(SONAME\)/ { s = $0; sub(/.*\[/, "", s); sub(/\]$/, "", s); print s; n++ }
         END { if (!n) print "none" }' "$d/$b.dynamic" > "$o/$b.soname" || die "$b: soname"
    awk '/\((RPATH|RUNPATH)\)/ { s = $0; sub(/.*\[/, "", s); sub(/\]$/, "", s); print s; n++ }
         END { if (!n) print "none" }' "$d/$b.dynamic" > "$o/$b.rpath" || die "$b: rpath"
    LC_ALL=C sort "$d/$b.exports" > "$o/$b.exports" || die "$b: exports"
    LC_ALL=C sort "$d/$b.imports" > "$o/$b.imports" || die "$b: imports"
    sed -n 's/^ *\(Tag_[A-Za-z_]*: .*\)$/\1/p' "$d/$b.attributes" > "$o/$b.attributes" || die "$b: attributes"
    sed -n 's/^ *\[ *[0-9a-fx]*\] *\(.*\)$/\1/p' "$d/$b.comment" > "$o/$b.comment" || die "$b: comment"
    awk '$1 == ".text" || $1 == ".rodata" || $1 == ".data" || $1 == ".bss" { print $1, $2 }' \
        "$d/$b.size" > "$o/$b.sections" || die "$b: sections"
    for f in $FIELDS; do
        [ -s "$o/$b.$f" ] || die "$b: field '$f' came out empty (input $d/$b.* not what binprint writes?)"
    done
    [ "$(wc -l < "$o/$b.sections")" -eq 4 ] || die "$b: size -A lacks one of .text .rodata .data .bss"
}

if [ "${1:-}" = "--extract" ]; then
    [ $# -eq 3 ] || die "usage: $0 --extract <binprint-dir> <outdir>"
    for b in kvm_system libkvm.so libkvm_mmf.so; do
        extract "$2" "$b" "$3"
    done
    echo "[binabi] golden fields written to $3"
    exit 0
fi

[ $# -eq 1 ] || die "usage: $0 <binprint-dir> | --extract <binprint-dir> <outdir>"
IN="$1"
[ -d "$IN" ] || die "$IN is not a directory"
[ -d "$GOLDEN" ] || die "$GOLDEN is missing"
need "$ALLOW"

BINS="$(cd "$GOLDEN" && ls ./*.needed 2>/dev/null)" || die "no golden files in $GOLDEN"
BINS="$(printf '%s\n' "$BINS" | sed 's#^\./##; s#\.needed$##')"
[ -n "$BINS" ] || die "no golden binaries in $GOLDEN"

T="$(mktemp -d)" || die "mktemp failed"
trap 'rm -rf "$T"' EXIT
: > "$T/used"

# Parse allow.txt: one "bin field line" per entry, kept in $T/allow.list as bin<TAB>field<TAB>line.
: > "$T/allow.list"
n=0
while IFS= read -r l || [ -n "$l" ]; do
    n=$((n + 1))
    case "$l" in ''|'#'*) continue ;; esac
    case "$l" in *' # '*) ;; *) die "$ALLOW:$n: entry without ' # <reason>'" ;; esac
    reason="${l#* # }"
    [ -n "${reason// /}" ] || die "$ALLOW:$n: empty reason"
    e="${l%% # *}"
    ab="${e%% *}"; rest="${e#* }"
    [ "$rest" != "$e" ] || die "$ALLOW:$n: need '<binary> <field> <line>'"
    af="${rest%% *}"; at="${rest#* }"
    [ "$at" != "$rest" ] || die "$ALLOW:$n: need '<binary> <field> <line>'"
    case " $FIELDS " in *" $af "*) ;; *) die "$ALLOW:$n: unknown field '$af'" ;; esac
    printf '%s\t%s\t%s\n' "$ab" "$af" "$at" >> "$T/allow.list"
done < "$ALLOW"

fail=0
for b in $BINS; do
    extract "$IN" "$b" "$T/act" || exit 1
    count=0
    bfail=0
    for f in $FIELDS; do
        need "$GOLDEN/$b.$f"
        diff "$GOLDEN/$b.$f" "$T/act/$b.$f" > "$T/d" 2> "$T/derr"
        rc=$?
        [ "$rc" -le 1 ] || die "diff failed on $b.$f: $(cat "$T/derr")"
        count=$((count + 1))
        [ "$rc" -eq 1 ] || continue
        # differing lines: golden-only "< x" and build-only "> x"
        sed -n 's/^[<>] //p' "$T/d" > "$T/dl"
        : > "$T/bad"
        while IFS= read -r dl; do
            if awk -F'\t' -v b="$b" -v f="$f" -v t="$dl" '$1 == b && $2 == f && $3 == t { found = 1 } END { exit !found }' "$T/allow.list"; then
                printf '%s\t%s\t%s\n' "$b" "$f" "$dl" >> "$T/used"
            else
                printf '%s\n' "$dl" >> "$T/bad"
            fi
        done < "$T/dl"
        if [ -s "$T/bad" ]; then
            bfail=1
            echo "[binabi] $b: field '$f' differs from golden (unlisted lines below; '<' golden, '>' build):" >&2
            command grep -E '^[<>] ' "$T/d" | while IFS= read -r x; do
                if command grep -Fxq -- "${x#??}" "$T/bad"; then echo "    $x" >&2; fi
            done
        fi
    done
    if [ "$bfail" -eq 0 ]; then
        echo "[binabi] $b: OK ($count fields)"
    else
        fail=1
    fi
done

# an allow entry that matched no difference is stale: fatal
while IFS="$(printf '\t')" read -r ab af at; do
    if ! awk -F'\t' -v b="$ab" -v f="$af" -v t="$at" '$1 == b && $2 == f && $3 == t { found = 1 } END { exit !found }' "$T/used"; then
        echo "[binabi] STALE allow entry (no longer differs): $ab $af $at" >&2
        fail=1
    fi
done < "$T/allow.list"

[ "$fail" -eq 0 ] || die "ABI differs from the 2.6.38 golden manifest (tools/binabi); fix the build or add a reasoned allow.txt entry"
echo "[binabi] all binaries match the golden manifest"
