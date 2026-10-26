/*
 * probe.c - print the ABI facts of one header set, one fact per line.
 *
 *   I <NAME> 0x<request> dir=<d> type=<c> nr=<n> size=<s>   ioctl request (ioctls.def)
 *   I <NAME> MISSING                                        no such macro in this header set
 *   S <struct> <sizeof> <alignof>
 *   F <struct>.<field> <offsetof> <sizeof field>
 *
 * Built once per header set (see Makefile); the two outputs are diffed. A line
 * that differs, or is present on one side only, is a mismatch.
 *
 * ioctls.inc is generated from ioctls.def (one #ifdef per request, so a name an
 * enum or a typo hides reports MISSING instead of breaking the build);
 * structs.inc is hand-kept: the structs and fields the KVM video path passes.
 * Every struct in the headers, not just these, is covered by layout.sh.
 *
 * Plain C11. LP64 only (asserted below).
 */
#include <stddef.h>
#include <stdio.h>
#include "umbrella.h"

_Static_assert(sizeof(long) == 8 && sizeof(void *) == 8, "abicheck assumes LP64 (riscv64 musl)");

static void ioctl_line(const char *name, unsigned long v)
{
	printf("I %s 0x%08lx dir=%lu type=0x%02lx nr=%lu size=%lu\n", name, v,
	       (unsigned long)_IOC_DIR(v), (unsigned long)_IOC_TYPE(v),
	       (unsigned long)_IOC_NR(v), (unsigned long)_IOC_SIZE(v));
}

static void missing_line(const char *name)
{
	printf("I %s MISSING\n", name);
}

#define S(T) printf("S " #T " %zu %zu\n", sizeof(T), _Alignof(T))
#define F(T, f) printf("F " #T "." #f " %zu %zu\n", offsetof(T, f), sizeof(((T *)0)->f))

int main(void)
{
#include "ioctls.inc"
#include "structs.inc"
	return 0;
}
