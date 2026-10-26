#!/bin/sh
# layout.sh - dump the layout of EVERY struct, union, typedef and enum that the
# umbrella header pulls in, from the compiler's own DWARF (no hand-kept list).
#
#   layout.sh OUT.txt [cc flags...]      (flags carry the -I roots and -D chip defs)
#
# Lines (sorted, one fact each, so two outputs can be diffed):
#   T <struct X|union X|typedef-name>  <sizeof>
#   M <type>.<member>  <byte offset>  <member sizeof> [bits=<width>@<bit offset>]
#   Y <typedef>  <sizeof>
#   E <enumerator>  <value>
# Anonymous struct/union members are flattened into their parent the way C
# names them; members whose type is an anonymous aggregate recurse as
# <type>.<member>.<inner>.
#
# Needs: $CC (default cc, any DWARF-5 gcc/clang), readelf, awk. Runs on the
# host: layout is read from debug info, nothing is executed.
set -eu
out=$1; shift
here=$(cd "$(dirname "$0")" && pwd)
cc=${CC:-cc}
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM

"$cc" -std=gnu11 -g -gdwarf-5 -fno-eliminate-unused-debug-types -c -x c \
	-I"$here" "$@" "$here/umbrella.h" -o "$tmp/u.o"
readelf --debug-dump=info "$tmp/u.o" > "$tmp/info.txt"

awk '
function id_of(s) { sub(/^<0x/, "", s); sub(/^<</, "", s); sub(/>$/, "", s); return s }
function val(line,   v) { v = line; sub(/^[^:]*:[ \t]*/, "", v); return v }
function nameof(line,   v) {
	v = line; sub(/^.*DW_AT_name[ \t]*:[ \t]*/, "", v)
	sub(/^\((indirect|strp)[a-z ]*(, offset: 0x[0-9a-f]+)?\): /, "", v)
	return v
}
function num(s) { sub(/[ \t].*$/, "", s); return s + 0 }
function strip(t) { # follow cv/typedef to the underlying DIE id
	while (t != "" && (tag[t] == "typedef" || tag[t] == "const_type" || tag[t] == "volatile_type" || tag[t] == "restrict_type" || tag[t] == "atomic_type")) t = type[t]
	return t
}
function tsize(t,   u, s, i, n) {
	if (t == "") return 0
	u = tag[t]
	if (u == "typedef" || u == "const_type" || u == "volatile_type" || u == "restrict_type" || u == "atomic_type") return tsize(type[t])
	if (u == "pointer_type") return 8
	if (u == "array_type") {
		s = tsize(type[t]); n = ndim[t]
		for (i = 1; i <= n; i++) s *= dim[t, i]
		return s
	}
	return size[t]
}
function aggr(t) { return tag[t] == "structure_type" || tag[t] == "union_type" }
function members(label, t, base,   i, m, mt, r, nm) {
	for (i = 1; i <= nmem[t]; i++) {
		m = mem[t, i]; mt = type[m]; r = strip(mt); nm = name[m]
		if (nm == "" && aggr(r)) { members(label, r, base + loc[m]); continue }
		printf "M %s.%s %d %d%s\n", label, nm, base + loc[m], tsize(mt), (bsz[m] != "" ? " bits=" bsz[m] "@" (dbo[m] != "" ? dbo[m] + base * 8 : "?") : "")
		if (aggr(r) && name[r] == "" && !(r in label_of)) members(label "." nm, r, base + loc[m])
	}
}
/^ <[0-9]+><[0-9a-f]+>: Abbrev Number: [1-9]/ {
	d = $1; sub(/^<([0-9]+)>.*/, "&", d)
	match($0, /<[0-9]+>/); depth = substr($0, RSTART + 1, RLENGTH - 2) + 0
	match($0, /<[0-9a-f]+>:/); cur = substr($0, RSTART + 1, RLENGTH - 3)
	t = $0; sub(/^.*\(DW_TAG_/, "", t); sub(/\)$/, "", t); tag[cur] = t
	par[cur] = stack[depth - 1]; stack[depth] = cur
	order[++nd] = cur
	p = par[cur]
	if (t == "member" && p != "") { mem[p, ++nmem[p]] = cur }
	if (t == "subrange_type" && p != "") { sub_of[cur] = p }
	if (t == "enumerator") { enum_order[++ne] = cur }
	next
}
/DW_AT_name/ { name[cur] = nameof($0); next }
/DW_AT_byte_size/ { size[cur] = num(val($0)); next }
/DW_AT_type/ { type[cur] = id_of(val($0)); next }
/DW_AT_data_member_location/ { loc[cur] = num(val($0)); next }
/DW_AT_bit_size/ { bsz[cur] = num(val($0)); next }
/DW_AT_data_bit_offset/ { dbo[cur] = num(val($0)); next }
/DW_AT_upper_bound/ { up[cur] = num(val($0)); hasup[cur] = 1; next }
/DW_AT_count/ { up[cur] = num(val($0)) - 1; hasup[cur] = 1; next }
/DW_AT_const_value/ { cv[cur] = val($0); next }
END {
	for (i = 1; i <= nd; i++) {
		c = order[i]
		if (tag[c] == "subrange_type") { a = sub_of[c]; ndim[a]++; dim[a, ndim[a]] = (hasup[c] ? up[c] + 1 : 0) }
	}
	# labels: struct/union by tag name, anonymous ones by their first typedef
	for (i = 1; i <= nd; i++) {
		c = order[i]
		if (aggr(tag[c]) || 1) {}
		if (aggr(c)) {
			if (name[c] != "") label_of[c] = (tag[c] == "structure_type" ? "struct " : "union ") name[c]
		}
	}
	for (i = 1; i <= nd; i++) {
		c = order[i]
		if (tag[c] == "typedef" && name[c] != "") {
			r = type[c]
			while (r != "" && (tag[r] == "const_type" || tag[r] == "volatile_type")) r = type[r]
			if (aggr(r) && !(r in label_of)) label_of[r] = name[c]
			printf "Y %s %d\n", name[c], tsize(c)
		}
	}
	for (i = 1; i <= nd; i++) {
		c = order[i]
		if (aggr(c) && (c in label_of) && !(c in done) && !(size[c] == 0 && nmem[c] == 0)) {
			done[c] = 1
			printf "T %s %d\n", label_of[c], size[c]
			members(label_of[c], c, 0)
		}
	}
	for (i = 1; i <= ne; i++) { c = enum_order[i]; printf "E %s %s\n", name[c], cv[c] }
}
' "$tmp/info.txt" | LC_ALL=C sort -u > "$out"
