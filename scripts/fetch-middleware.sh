#!/bin/bash
#
# Fetch the sophgo-middleware SOURCE OF RECORD from estate storage, verify it
# against a pinned sha256 and extract it to a fixed path.
#
#   scpcom/sophgo-middleware @ 4e231ef5967f8f7d77cecd65d9e2052a6f92f18c
#   (branch licheervnano; the tree is under v2/)
#
# Usage: fetch-middleware.sh <outdir>      the tree lands in <outdir>/v2/...
#
# The tarball is `git archive` of that commit (gzip -n), kept on the artifacts
# share at <share>/src/$MW_TARBALL. The build never goes to github.com for it:
# $NANOKVM_SRC_MIRROR (forge-build.sh sets it to $ARTIFACTS/src) names the
# directory it must be in, and the script refuses to run without it. A missing
# file or any sha256 other than the one below aborts the build: no fallback, no
# unpinned fetch. Source of the pin: tools/abicheck/README.md and the journey
# 01k5sps7qf decision.
#
# NANOKVM_MIDDLEWARE_SRC (read by support/sg2002/additional/kvm_mmf/CMakeLists.txt)
# is NOT exported by this script; a caller opts in by exporting <outdir>.

set -euo pipefail

MW_COMMIT=4e231ef5967f8f7d77cecd65d9e2052a6f92f18c
MW_TARBALL=sophgo-middleware-4e231ef5967f.tar.gz
MW_SHA256=a431a22dac1ca1fec1bb577d12c305ea1699066d3c36cc6d4a54b0e54b36350b

die() { echo "[fetch-middleware] ERROR: $*" >&2; exit 1; }
log() { echo "[fetch-middleware] $*"; }

[ $# -eq 1 ] || die "usage: $0 <outdir>"
OUT="$1"
case "$OUT" in
    /*) ;;
    *) die "outdir must be absolute: $OUT" ;;
esac
[ -n "${HOME:-}" ] || die "HOME is unset"
case "$OUT" in
    "$HOME"/*) ;;
    *) die "refusing to write $OUT outside HOME ($HOME)" ;;
esac
[ -n "${NANOKVM_SRC_MIRROR:-}" ] || die "NANOKVM_SRC_MIRROR is unset: the middleware source comes from the artifacts share, not from github.com"
FILE="$NANOKVM_SRC_MIRROR/$MW_TARBALL"
[ -f "$FILE" ] || die "$FILE is missing: put the git archive of $MW_COMMIT there (sha256 $MW_SHA256)"
for tool in sha256sum tar; do
    command -v "$tool" >/dev/null 2>&1 || die "'$tool' is missing"
done

got="$(sha256sum "$FILE" | cut -d' ' -f1)"
[ "$got" = "$MW_SHA256" ] || die "$MW_TARBALL: sha256 $got, pinned $MW_SHA256"
log "$MW_TARBALL sha256 ok"

rm -rf "$OUT"
mkdir -p "$OUT"
tar -xzf "$FILE" -C "$OUT"
for d in v2/include v2/sample/common v2/component/panel/cv181x v2/3rdparty/inih \
         v2/modules/ive/include v2/component/isp/sensor/sg200x/lontium_lt6911 v2/lib; do
    [ -d "$OUT/$d" ] || die "extracted tree has no $d"
done
printf '%s\n' "$MW_COMMIT" > "$OUT/.pin"
log "middleware $MW_COMMIT extracted to $OUT"
