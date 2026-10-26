#!/bin/bash
#
# Fail-closed link closure of a set of riscv64 shared libraries.
#
#   check-mmf-closure.sh --lib <file>... --system <file>...
#
# --lib     the libraries that ship together (ours + the pin's closed blobs, and libkvm_mmf.so)
# --system  the libraries the device image provides (musl libc.so, libstdc++, libgcc_s)
#
# 1. readelf -d: every NEEDED of every --lib must be the basename of another --lib or of a
#    --system file. A MaixCDK library (or anything else) that sneaks back in as a NEEDED
#    entry is not in either list, so this fails.
# 2. readelf --dyn-syms: every undefined GLOBAL dynamic symbol of every --lib must be defined
#    by some --lib or --system file. WEAK undefined symbols are optional by definition
#    and are not required.
#
# Prints the NEEDED entries (the evidence) and exits non-zero on any violation.

set -euo pipefail

die() { echo "[closure] ERROR: $*" >&2; exit 1; }

libs=() system=() mode=
for a in "$@"; do
    case "$a" in
        --lib) mode=lib ;;
        --system) mode=system ;;
        *) case "$mode" in
               lib) libs+=("$a") ;;
               system) system+=("$a") ;;
               *) die "usage: $0 --lib <file>... --system <file>..." ;;
           esac ;;
    esac
done
[ "${#libs[@]}" -gt 0 ] || die "no --lib given"
for f in "${libs[@]}" "${system[@]}"; do
    [ -f "$f" ] || die "$f does not exist"
    readelf -h "$f" | command grep -q 'Machine:.*RISC-V' || die "$f is not RISC-V"
done

declare -A known=()
for f in "${libs[@]}" "${system[@]}"; do known["$(basename "$f")"]=1; done

fail=0
for f in "${libs[@]}"; do
    needed="$(readelf -d "$f" | sed -n 's/.*(NEEDED).*Shared library: \[\(.*\)\]/\1/p')"
    echo "[closure] $(basename "$f") NEEDED: $(printf '%s ' $needed)"
    for n in $needed; do
        if [ -z "${known[$n]:-}" ]; then
            echo "[closure] FAIL: $(basename "$f") needs $n, which is neither in the shipped set nor a system library" >&2
            fail=1
        fi
    done
done

defs="$(mktemp)"
trap 'rm -f "$defs"' EXIT
for f in "${libs[@]}" "${system[@]}"; do
    readelf --dyn-syms -W "$f" | awk '$7 != "UND" && $8 != "" { sub(/@.*/, "", $8); print $8 }'
done | LC_ALL=C sort -u > "$defs"
[ -s "$defs" ] || die "no defined symbols found: refusing to call an empty closure complete"

for f in "${libs[@]}"; do
    und="$(readelf --dyn-syms -W "$f" | awk '$7 == "UND" && $5 == "GLOBAL" && $8 != "" { sub(/@.*/, "", $8); print $8 }' | LC_ALL=C sort -u)"
    missing="$(comm -23 <(printf '%s\n' "$und") "$defs" | sed '/^$/d')"
    if [ -n "$missing" ]; then
        echo "[closure] FAIL: $(basename "$f") has undefined symbols no shipped or system library defines:" >&2
        printf '    %s\n' $missing >&2
        fail=1
    else
        echo "[closure] $(basename "$f"): $(printf '%s\n' "$und" | sed '/^$/d' | wc -l | tr -d ' ') undefined symbols, all defined in the set"
    fi
done
[ "$fail" -eq 0 ] || die "closure check failed"
echo "[closure] ok: ${#libs[@]} libraries"
