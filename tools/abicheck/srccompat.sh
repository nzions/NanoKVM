#!/bin/sh
# srccompat.sh - does the pinned middleware's ioctl-issuing source still compile
# against a given driver header set?  (-fsyntax-only: every macro, struct and
# field the userspace code names must exist with a type the code can use.)
#
#   srccompat.sh MW_ROOT DRV_INC_FLAGS...
#
# MW_ROOT is the middleware tree root that holds modules/ and include/
# (cd8bb74: the clone root; 4e231ef: <clone>/v2). Flags are the -I roots and
# -D chip defs of the driver header set under test.
#
# MODULES="venc vdec" limits it to those modules/<m>/src dirs (default: sys vpu
# vi vpss rgn vo venc vdec gdc; a pin lacks the ones it does not ship).
#
# Prints, per source file, "ok" or the compiler errors, then a one-line
# summary "srccompat: <n> files, <m> with errors". Exit 1 if any has errors.
# Errors are NOT all ABI: a missing build-time define is reported too (the
# script supplies SDK_VER/MMF_VERSION); read them, then judge.
set -u
mw=$1; shift
here=$(cd "$(dirname "$0")" && pwd)
cc=${CC:-cc}
inc="-I$here/shim"
for d in "$mw"/include "$mw"/include/isp/cv181x "$mw"/modules/*/include "$mw"/modules/sys/src; do
	[ -d "$d" ] && inc="$inc -I$d"
done
files=$(for m in ${MODULES:-sys vpu vi vpss rgn vo venc vdec gdc}; do
	[ -d "$mw/modules/$m/src" ] && command grep -lE 'ioctl[[:space:]]*\(' "$mw"/modules/$m/src/*.c 2>/dev/null
done | LC_ALL=C sort)
n=0; bad=0
for f in $files; do
	n=$((n + 1))
	rel=${f#$mw/}
	if out=$("$cc" -std=gnu11 -w -fsyntax-only -DSDK_VER='"abicheck"' -DMMF_VERSION='"abicheck"' \
		"$@" $inc "$f" 2>&1); then
		echo "ok      $rel"
	else
		bad=$((bad + 1))
		echo "ERRORS  $rel"
		printf '%s\n' "$out" | command grep -E 'error|fatal' | sed 's/^/          /'
	fi
done
echo "srccompat: $n files, $bad with errors"
[ "$bad" -eq 0 ]
