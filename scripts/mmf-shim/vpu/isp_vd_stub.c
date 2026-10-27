/*
 * NanoKVM: CVI_ISP_GetVDTimeOut, called once by libvpu's register dump (dump_register.c).
 * The pin defines it in the closed libisp.so, which NanoKVM does not link (no ISP runs:
 * the LT6911 delivers YUV and the VI pipe bypasses it). The dump logs the failure and
 * carries on. build-mmf.sh copies this file into v2/modules/vpu/src before that module builds.
 */
#include "cvi_base.h"
#include "cvi_isp.h"

CVI_S32 CVI_ISP_GetVDTimeOut(VI_PIPE ViPipe, ISP_VD_TYPE_E enIspVDType, CVI_U32 u32MilliSec)
{
	(void)ViPipe;
	(void)enIspVDType;
	(void)u32MilliSec;
	return CVI_FAILURE;
}
