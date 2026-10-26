/*
 * mirror.c - structs the middleware re-declares by hand next to the kernel's.
 * Compiled against ONE pin's include/ plus ONE driver header set; the build
 * fails (static assert) if a mirror drifts from the kernel struct it shadows.
 *
 * sys_ion_data2 (cvi_sys.h, middleware) shadows sys_ion_data (sys_uapi.h,
 * kernel): ionFree2() passes it straight to SYS_ION_FREE.
 */
#include <stddef.h>
#include "umbrella.h"
#include "cvi_sys.h"

#define SAME(a, b, f) \
	_Static_assert(offsetof(a, f) == offsetof(b, f) && sizeof(((a *)0)->f) == sizeof(((b *)0)->f), \
		       #a "." #f " differs from " #b "." #f)

_Static_assert(sizeof(struct sys_ion_data2) == sizeof(struct sys_ion_data), "sys_ion_data2 size differs from sys_ion_data");
SAME(struct sys_ion_data2, struct sys_ion_data, size);
SAME(struct sys_ion_data2, struct sys_ion_data, cached);
SAME(struct sys_ion_data2, struct sys_ion_data, dmabuf_fd);
SAME(struct sys_ion_data2, struct sys_ion_data, addr_p);
SAME(struct sys_ion_data2, struct sys_ion_data, name);

int main(void) { return 0; }
