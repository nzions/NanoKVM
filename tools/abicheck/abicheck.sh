#!/bin/sh
# abicheck.sh - ABI check of the SG2002 MMF userspace (sophgo-middleware pins)
# against the kernel uapi headers the 7.3 osdrv port is actually built from.
# See README.md for what each stage proves and does not prove.
#
#   abicheck.sh --osdrv-v2-tgz F --osdrv-v1-tgz F --mw-4e231ef DIR --mw-cd8bb74 DIR [--out DIR]
#   abicheck.sh --osdrv-v2 DIR   --osdrv-v1 DIR   ...                (unverified trees)
#
#   --osdrv-v2-tgz   scpcom/sophgo-osdrv cbd9c582 tarball  (sha256 checked)
#   --osdrv-v1-tgz   sophgo/osdrv 1b3a4964 tarball         (sha256 checked)
#   --osdrv-v2 DIR   that tarball's  interdrv/v2   (instead of the tarball)
#   --osdrv-v1 DIR   that tarball's  interdrv       (instead of the tarball)
#   --mw-4e231ef DIR scpcom/sophgo-middleware 4e231ef clone root (tree is under v2/)
#   --mw-cd8bb74 DIR scpcom/sophgo-middleware cd8bb74 clone root (tree at the root)
#   --kvm-src FILE   kvm_mmf.cpp: boundary diffs are tagged when it names the type
#   --out DIR        results (default ./abicheck-out)
#   --allow FILE     accepted vc_drv-boundary drift: "PIN NAME reason..." per line;
#                    anything else differing fails, and so does an entry that no
#                    longer differs
#   --probe-cc CC    compiler for probe.c, which is EXECUTED (default: CC). Set it to
#                    a host compiler when CC is a cross compiler.
#
# CC (default cc) is the compiler. It runs everything on the build host: the
# structs are LP64 on aarch64 and riscv64 alike, and layout is read from
# compile output, so nothing target-specific is executed.
#
# Exit 0: every selected pin is compatible with the kernel header split.
# Exit 1: at least one mismatch (listed). Exit 2: usage / missing input.
set -u
here=$(cd "$(dirname "$0")" && pwd)
cc=${CC:-cc}

# The pins the kernel build fetches (fleet containers/kernel-nanokvm-sg2002,
# .build.json "osdrv.tar.gz" / "osdrv2.tar.gz").
SHA_V2=fb40b511ca4bd30534a42371fc825a657985cbd8f3c0b95162657d32dc7f3d14
SHA_V1=9913c091e732e301f4dfa5386a91975e33959796546b77d50ee329a74e2743f3

v2dir= v1dir= v2tgz= v1tgz= mw_a= mw_b= kvmsrc= out=./abicheck-out allow= probecc=
while [ $# -gt 0 ]; do
	case $1 in
	--osdrv-v2) v2dir=$2 ;;
	--osdrv-v1) v1dir=$2 ;;
	--osdrv-v2-tgz) v2tgz=$2 ;;
	--osdrv-v1-tgz) v1tgz=$2 ;;
	--mw-4e231ef) mw_a=$2 ;;
	--mw-cd8bb74) mw_b=$2 ;;
	--out) out=$2 ;;
	--kvm-src) kvmsrc=$2 ;;
	--allow) allow=$2 ;;
	--probe-cc) probecc=$2 ;;
	*) echo "abicheck: unknown argument $1" >&2; exit 2 ;;
	esac
	shift 2 || { echo "abicheck: $1 needs a value" >&2; exit 2; }
done

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT INT TERM
mkdir -p "$out"

unpack() { # unpack TGZ EXPECTED_SHA PATH_IN_TAR_AFTER_TOPDIR DEST
	got=$(sha256sum "$1" | cut -d' ' -f1)
	if [ "$got" != "$2" ]; then
		echo "abicheck: $1 sha256 $got != pinned $2" >&2
		exit 2
	fi
	mkdir -p "$4"
	tar -xzf "$1" -C "$4" --strip-components=1 --wildcards "*/$3/include/*" ||
		{ echo "abicheck: cannot extract include/ from $1" >&2; exit 2; }
}
src_v2="$v2dir (UNVERIFIED tree)"
src_v1="$v1dir (UNVERIFIED tree)"
if [ -n "$v2tgz" ]; then
	unpack "$v2tgz" "$SHA_V2" interdrv/v2 "$work/v2"; v2dir=$work/v2/interdrv/v2
	src_v2="$v2tgz sha256 $SHA_V2 (scpcom/sophgo-osdrv cbd9c582)"
fi
if [ -n "$v1tgz" ]; then
	unpack "$v1tgz" "$SHA_V1" interdrv "$work/v1"; v1dir=$work/v1/interdrv
	src_v1="$v1tgz sha256 $SHA_V1 (sophgo/osdrv 1b3a4964)"
fi
[ -d "$v2dir/include/common/uapi/linux" ] || { echo "abicheck: no v2 include/common/uapi/linux under '$v2dir'" >&2; exit 2; }
[ -d "$v1dir/include/chip/cv181x/uapi/linux" ] || { echo "abicheck: no v1 include/chip/cv181x/uapi/linux under '$v1dir'" >&2; exit 2; }

# Chip selection. The 7.3 build passes CHIP_CODE=mars CHIP_ARCH=CV181X; the v2
# tree reaches its chip dir as include/chip/$(CHIP_CODE) and mars -> cv181x is a
# symlink there. Resolve it, and fail if it does not land on cv181x.
mars=$(cd "$v2dir/include/chip/mars" 2>/dev/null && pwd -P) || mars=
case $mars in
*/include/chip/cv181x) ;;
*) echo "abicheck: v2 include/chip/mars does not resolve to include/chip/cv181x (got '$mars')" >&2; exit 2 ;;
esac
DEFS="-D__SOC_MARS__ -D__CV181X__ -DARCH_CV181X"
INC_V2="-I$v2dir/include/common/uapi -I$v2dir/include/chip/mars/uapi"
INC_V1="-I$v1dir/include/common/uapi -I$v1dir/include/chip/cv181x/uapi"

fail=0
say() { printf '%s\n' "$*"; }

say "== source of record"
say "v2 (sys base vi vpss dwa rgn vo ive cif ...): $src_v2"
say "v1 (vcodec cvi_vc_drv jpeg):                  $src_v1"
say "chip: CHIP_CODE=mars -> include/chip/cv181x (symlink checked), CHIP_ARCH=CV181X"
say "compiler: $($cc --version | sed -n 1p)  host: $(uname -m) (LP64; target riscv64 LP64 musl, not cross-run)"

probe() { # probe LABEL "INC" -> $out/probe-LABEL.txt
	${probecc:-$cc} -std=gnu11 -Wall -I"$here" $DEFS $2 "$here/probe.c" -o "$work/probe-$1" 2>"$out/probe-$1.build.log" || {
		say "probe $1: BUILD FAILED (see $out/probe-$1.build.log)"; fail=1; return 1; }
	"$work/probe-$1" > "$out/probe-$1.txt"
}
say
say "== kernel header sets compared with each other (v2 vs v1)"
probe v2 "$INC_V2" && probe v1 "$INC_V1" &&
	if diff "$out/probe-v2.txt" "$out/probe-v1.txt" > "$out/probe-v2-vs-v1.diff"; then
		say "probe (KVM-path ioctl requests, hot structs): identical"
	else
		say "probe (KVM-path ioctl requests, hot structs): DIFFERENT, $(command grep -c '^<' "$out/probe-v2-vs-v1.diff") lines v2 / $(command grep -c '^>' "$out/probe-v2-vs-v1.diff") lines v1"
		command grep '^[<>]' "$out/probe-v2-vs-v1.diff" | sed 's/^/  /'
	fi
CC=$cc "$here/layout.sh" "$out/layout-v2.txt" $DEFS $INC_V2 && CC=$cc "$here/layout.sh" "$out/layout-v1.txt" $DEFS $INC_V1 || { say "layout.sh failed"; exit 2; }
diff "$out/layout-v2.txt" "$out/layout-v1.txt" > "$out/layout-v2-vs-v1.diff" || true
say "layout (every struct/union/typedef/enum): v2 $(wc -l < "$out/layout-v2.txt") facts, v1 $(wc -l < "$out/layout-v1.txt") facts, $(command grep -c '^[<>]' "$out/layout-v2-vs-v1.diff") differing lines -> $out/layout-v2-vs-v1.diff"

# One pin: per-subsystem source compat against the header set the 7.3 kernel
# builds that subsystem's driver from (venc/vdec -> v1 cvi_vc_drv, rest -> v2).
checkpin() { # checkpin LABEL MWROOT
	label=$1; root=$2; bad=0
	say
	say "== pin $label ($root)"
	[ -d "$root/modules" ] || { say "no modules/ under $root"; fail=1; return; }
	have=
	for m in sys vpu vi vpss rgn vo gdc; do [ -d "$root/modules/$m/src" ] && have="$have $m"; done
	say "-- ioctl sources vs the v2 kernel set (modules:$have)"
	MODULES="$have" CC=$cc "$here/srccompat.sh" "$root" $DEFS $INC_V2 | tee "$out/srccompat-$label-v2.txt" | sed 's/^/  /'
	command grep -q '^ERRORS' "$out/srccompat-$label-v2.txt" && bad=1
	say "-- ioctl sources vs the v1 kernel set (modules: venc vdec)"
	MODULES="venc vdec" CC=$cc "$here/srccompat.sh" "$root" $DEFS $INC_V1 | tee "$out/srccompat-$label-v1.txt" | sed 's/^/  /'
	command grep -q '^ERRORS' "$out/srccompat-$label-v1.txt" && bad=1
	say "-- informational: whole pin against the OTHER set (which generation was it written for?)"
	CC=$cc "$here/srccompat.sh" "$root" $DEFS $INC_V1 > "$out/srccompat-$label-allv1.txt"
	CC=$cc "$here/srccompat.sh" "$root" $DEFS $INC_V2 > "$out/srccompat-$label-allv2.txt"
	say "  all modules vs v2: $(tail -n 1 "$out/srccompat-$label-allv2.txt")"
	say "  all modules vs v1: $(tail -n 1 "$out/srccompat-$label-allv1.txt")"
	# A name that exists in both sets can still carry a different VALUE (the
	# VI_SDK_* / RGN_SDK_* sub-command enums are shifted between v1 and v2), and
	# that compiles clean. So: take the generation the pin compiles against
	# (v1 or v2 - the one whose whole-pin compile has no errors), and compare
	# every enumerator the pin's ioctl sources name against the value in the
	# header set the kernel builds that subsystem from (v2, except venc/vdec).
	gen=
	command grep -q ' 0 with errors' "$out/srccompat-$label-allv2.txt" && gen=v2
	command grep -q ' 0 with errors' "$out/srccompat-$label-allv1.txt" && { [ -z "$gen" ] && gen=v1 || gen=either; }
	if [ -z "$gen" ] || [ "$gen" = either ]; then
		say "-- enum VALUES the pin's ioctl sources use: not comparable, pin compiles against '${gen:-neither}' header set"
	else
		for grp in "sys vpu vi vpss rgn vo gdc:v2" "venc vdec:v1"; do
			mods=${grp%:*}; kern=${grp#*:}
			say "-- enum VALUES used by modules [$mods]: pin generation ($gen) vs kernel set ($kern)"
			srcs=$(for m in $mods; do ls "$root"/modules/$m/src/*.c 2>/dev/null; done)
			[ -n "$srcs" ] || { say "  (pin ships none of these modules)"; continue; }
			cat $srcs | command grep -oE '[A-Za-z_][A-Za-z_0-9]*' | LC_ALL=C sort -u > "$work/idents-$label.txt"
			awk '
				FNR == 1 { fi++ }
				fi == 1 { used[$1] = 1; next }
				fi == 2 && $1 == "E" { pv[$2] = $3; next }
				fi == 3 && $1 == "E" { kv[$2] = $3; next }
				END {
					for (n in used) if (n in pv) {
						if (!(n in kv)) printf "  %-34s pin=%s kernel=MISSING\n", n, pv[n]
						else if (pv[n] != kv[n]) printf "  %-34s pin=%s kernel=%s\n", n, pv[n], kv[n]
						else same++
					}
					printf "  (%d enumerators identical)\n", same
				}' "$work/idents-$label.txt" "$out/layout-$gen.txt" "$out/layout-$kern.txt" | LC_ALL=C sort | tee "$out/enums-$label-$kern.txt"
			command grep -q 'pin=' "$out/enums-$label-$kern.txt" && bad=1
		done
	fi
	# The vc_drv boundary: libvenc/libvdec talk to cvi_vc_drv, which the 7.3
	# kernel builds from the v1 headers, while the rest of the stack (and, for
	# 4e231ef, the pin itself) is v2. Every type or enumerator the video-codec
	# ioctls can carry that differs between the two header sets is listed,
	# "carry" = named in the pin's venc/vdec sources, its cvi_venc.h/cvi_vdec.h,
	# or the v2 cvi_comm_venc/vdec/rc.h that define the ioctl argument types.
	if [ "$gen" = v1 ]; then
		say "-- vc_drv boundary: pin is v1-generation, same set as cvi_vc_drv: nothing to cross"
	else
		say "-- vc_drv boundary: venc/vdec types that differ between v2 and v1 headers"
		{ echo -; cat "$root"/modules/venc/src/*.c "$root"/modules/vdec/src/*.c "$root"/include/cvi_venc.h "$root"/include/cvi_vdec.h \
			"$v2dir"/include/common/uapi/linux/cvi_comm_venc.h "$v2dir"/include/common/uapi/linux/cvi_comm_vdec.h \
			"$v2dir"/include/common/uapi/linux/cvi_comm_rc.h 2>/dev/null; } |
			command grep -oE '[A-Za-z_][A-Za-z_0-9]*' | LC_ALL=C sort -u > "$work/vcnames-$label.txt"
		# The "-" line keeps the file non-empty: awk counts files by FNR == 1, which an
		# empty file never reaches (that shifted the file index and hid every difference).
		echo - > "$work/kvmnames.txt"
		if [ -n "$kvmsrc" ]; then command grep -oE '[A-Za-z_][A-Za-z_0-9]*' "$kvmsrc" | LC_ALL=C sort -u >> "$work/kvmnames.txt" || { say "cannot read $kvmsrc"; exit 2; }; fi
		awk '
			FNR == 1 { fi++ }
			fi == 1 { vc[$1] = 1; next }
			fi == 2 { kvm[$1] = 1; next }
			/^[<>] [TMYE] / {
				kind = $2
				if (kind == "E") name = $3
				else if ($3 == "struct" || $3 == "union") { name = $4 }
				else name = $3
				sub(/\..*$/, "", name)
				if (name in vc) {
					side = ($1 == "<" ? "v2" : "v1")
					line = $0; sub(/^[<>] /, "", line)
					seen[name] = seen[name] "\n      " side ": " line
					if (name in kvm) kvmuse[name] = 1
				}
			}
			END { for (n in seen) printf "  %s%s%s\n", n, (n in kvmuse ? "  [named in the KVM source]" : ""), seen[n] }
		' "$work/vcnames-$label.txt" "$work/kvmnames.txt" "$out/layout-v2-vs-v1.diff" | tee "$out/boundary-$label.txt"
		if [ -s "$out/boundary-$label.txt" ]; then
			# Every differing name must be in the allowlist for THIS pin, and every
			# allowlist entry for this pin must still differ (a stale entry means the
			# drift changed and the reason was never re-read: fail closed both ways).
			awk '/^  [^ ]/ { print $1 }' "$out/boundary-$label.txt" | LC_ALL=C sort -u > "$work/bnames-$label.txt"
			if [ -n "$allow" ]; then
				awk -v pin="$label" '$1 !~ /^#/ && NF >= 3 && $1 == pin { print $2 }' "$allow" | LC_ALL=C sort -u > "$work/anames-$label.txt"
			else : > "$work/anames-$label.txt"; fi
			newn=$(LC_ALL=C comm -23 "$work/bnames-$label.txt" "$work/anames-$label.txt")
			stale=$(LC_ALL=C comm -13 "$work/bnames-$label.txt" "$work/anames-$label.txt")
			for n in $(LC_ALL=C comm -12 "$work/bnames-$label.txt" "$work/anames-$label.txt"); do
				say "  ALLOWED $n: $(awk -v pin="$label" -v n="$n" '$1 == pin && $2 == n { $1 = $2 = ""; sub(/^ +/, ""); print; exit }' "$allow")"
			done
			for n in $newn; do say "  NOT ALLOWED (new mismatch): $n"; bad=1; done
			for n in $stale; do say "  STALE allowlist entry (no longer differs): $n"; bad=1; done
		else say "  none"; fi
	fi
	say "-- mirror structs (sys_ion_data2 vs sys_ion_data)"
	if $cc -std=gnu11 -I"$here" -I"$here/shim" $DEFS $INC_V2 -I"$root/include" -I"$root/modules/sys/include" "$here/mirror.c" -o /dev/null 2> "$out/mirror-$label.log"; then
		say "  match"
	else
		say "  MISMATCH:"; command grep -E 'error' "$out/mirror-$label.log" | sed 's/^/    /'; bad=1
	fi
	[ "$bad" -eq 0 ] && say "RESULT $label: compatible with the 7.3 header split" || { say "RESULT $label: MISMATCH (see above)"; fail=1; }
}
[ -n "$mw_a" ] && checkpin 4e231ef "$mw_a/v2"
[ -n "$mw_b" ] && checkpin cd8bb74 "$mw_b"

say
[ "$fail" -eq 0 ] && say "abicheck: OK" || say "abicheck: MISMATCH"
exit "$fail"
