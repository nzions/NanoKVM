/*
 * NanoKVM: isp_bin_{getBinSize,getBinParam,setBinParam,setBinParambuf}_p0..p3 and the
 * four header_bin_* handlers of the same table.
 *
 * libcvi_bin's table of per-block handlers (cvi_bin.c) names the ISP parameter block
 * handlers, which the pin only ships inside the closed libcvi_bin_isp.so. NanoKVM runs
 * no ISP (the LT6911 delivers YUV, the VI pipe bypasses it), so the ISP blocks have no
 * parameters to get or set: every handler reports failure, and every size is 0.
 * build-mmf.sh copies this file into v2/modules/bin/src before that module builds.
 * Prototypes: v2/modules/isp/include/cv181x/isp_bin.h.
 */
#include <stdio.h>
#include <linux/cvi_type.h>

#define ISP_BIN_STUB(pfx, p) \
	CVI_S32 pfx##_getBinSize##p(CVI_U32 *binSize) \
	{ \
		if (binSize) \
			*binSize = 0; \
		return CVI_FAILURE; \
	} \
	CVI_S32 pfx##_getBinParam##p(CVI_U8 *addr, CVI_U32 binSize) \
	{ \
		(void)addr; \
		(void)binSize; \
		return CVI_FAILURE; \
	} \
	CVI_S32 pfx##_setBinParam##p(FILE *fp) \
	{ \
		(void)fp; \
		return CVI_FAILURE; \
	} \
	CVI_S32 pfx##_setBinParambuf##p(CVI_U8 *buffer) \
	{ \
		(void)buffer; \
		return CVI_FAILURE; \
	}

/* The header block handlers (isp_bin.h declares the first two; the other two have the
 * same function-pointer types in cvi_bin.c's table). */
ISP_BIN_STUB(header_bin, )
ISP_BIN_STUB(isp_bin, _p0)
ISP_BIN_STUB(isp_bin, _p1)
ISP_BIN_STUB(isp_bin, _p2)
ISP_BIN_STUB(isp_bin, _p3)
