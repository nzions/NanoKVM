#!/bin/bash
#
# Fingerprint one riscv64 ELF binary, for ABI-equivalence evidence between two builds
# (the MaixCDK-built binaries of today and whatever replaces that build).
#
# Usage: scripts/binprint.sh <binary> <outdir>
#
# Writes <outdir>/<basename>.<kind> for each kind below; the first seven are required
# and the script exits non-zero when any of them is empty:
#   dynamic      readelf -d             (NEEDED, SONAME, RPATH, flags)
#   exports      defined dynamic symbols, sorted (the helpers of build-in-container.sh)
#   imports      undefined dynamic symbols, sorted (an empty export set of an executable
#                is written as "(none)"; an empty import set is an error)
#   dynsyms      type, bind, visibility, section index and name of every dynamic symbol
#   attributes   readelf -A             (the riscv arch / ABI attributes)
#   comment      readelf -p .comment    (which compiler built it)
#   size         size -A
#   sha256       sha256sum
# and, informational only (may legitimately be empty, then "(none)"):
#   notes        readelf -n             (build id)
#   stamps       date-like strings found inside the file (build timestamps)
#
# Binutils: the riscv64 musl cross tools when on PATH (scripts/forge-build.sh puts the
# Sophgo toolchain there), otherwise the host's. A missing tool is an error, never a skip.

set -eu
set -o pipefail

die() {
    echo "[binprint] ERROR: $*" >&2
    exit 1
}

[ $# -eq 2 ] || die "usage: $0 <binary> <outdir>"
BIN="$1"
OUT="$2"
[ -f "$BIN" ] || die "$BIN is not a file"
[ -s "$BIN" ] || die "$BIN is empty"
mkdir -p "$OUT"
NAME="$(basename "$BIN")"

CROSS=riscv64-unknown-linux-musl-
tool() {
    if command -v "$CROSS$1" >/dev/null 2>&1; then
        command -v "$CROSS$1"
    elif command -v "$1" >/dev/null 2>&1; then
        command -v "$1"
    else
        die "neither $CROSS$1 nor $1 is on PATH (a skipped fingerprint is a failed one)"
    fi
}
READELF="$(tool readelf)"
SIZE="$(tool size)"
for t in awk sed sort sha256sum; do
    command -v "$t" >/dev/null 2>&1 || die "'$t' is missing"
done

exported() {
    "$READELF" --dyn-syms --wide "$1" \
        | awk '$7 != "UND" && $8 != "" { print $8 }' \
        | sed 's/@.*//' | LC_ALL=C sort -u
}
imported() {
    "$READELF" --dyn-syms --wide "$1" \
        | awk '$7 == "UND" && $8 != "" { print $8 }' \
        | sed 's/@.*//' | LC_ALL=C sort -u
}

# Write stdin to <outdir>/<name>.<kind>; "required" kinds must not be empty.
put() {
    local kind="$1" required="$2" data
    data="$(cat)"
    if [ -z "$data" ]; then
        [ "$required" = optional ] || die "$NAME: $kind is empty"
        data="(none)"
    fi
    printf '%s\n' "$data" > "$OUT/$NAME.$kind"
}

"$READELF" -d "$BIN" | put dynamic required
exported "$BIN" | put exports optional
imported "$BIN" | put imports required
"$READELF" --dyn-syms --wide "$BIN" \
    | awk '$8 != "" { print $4, $5, $6, $7, $8 }' | sed 's/@.*//' | LC_ALL=C sort -u | put dynsyms required
"$READELF" -A "$BIN" | put attributes required
# a missing section is reported on stderr and leaves stdout empty; keep that message
"$READELF" -p .comment "$BIN" 2>&1 | put comment required
"$SIZE" -A "$BIN" | put size required
(cd "$(dirname "$BIN")" && sha256sum "$NAME") | put sha256 required

"$READELF" -n "$BIN" | put notes optional
# Date-like strings (__DATE__/__TIME__, ISO stamps): evidence of build-time embedding.
{ LC_ALL=C command grep -aoE '(Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec) [ 0-9][0-9] 20[0-9]{2}|20[0-9]{2}-[0-9]{2}-[0-9]{2}[ T][0-9]{2}:[0-9]{2}:[0-9]{2}|[0-9]{2}:[0-9]{2}:[0-9]{2}' "$BIN" || true; } \
    | LC_ALL=C sort -u | put stamps optional

echo "[binprint] $NAME: $(cut -d' ' -f1 "$OUT/$NAME.sha256") ($READELF)"
