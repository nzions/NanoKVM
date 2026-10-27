/*
 * Copyright (C) Cvitek Co., Ltd. 2019-2020. All rights reserved.
 *
 * File Name: sample/common/sample_common_isp.c
 * Description:
 *   Common ctrl code for isp.
 *
 * NanoKVM: copy of sophgo-middleware 4e231ef v2/sample/common/sample_common_isp.c.
 * The LT6911 delivers YUV and the VI pipe bypasses the ISP, so the closed ISP
 * libraries (libisp, libae, libaf, libawb, libcvi_bin_isp, libisp_algo) are not
 * linked. Every change from the original is marked "NanoKVM change".
 */

#include <stdint.h>
#include <string.h>
#include <stdio.h>
#include <stdlib.h>
#include <errno.h>
#include <unistd.h>
#include <sys/time.h>

#include <linux/cvi_defines.h>
#include "sample_comm.h"
#include "cvi_awb.h"
#include "cvi_af.h"

#include "cvi_sns_ctrl.h"
#include "cvi_ae.h"
#include "cvi_isp.h"

/* NanoKVM change: no ISP thread, no PQ-tool daemon, no g_IspPid. */
static CVI_U32 g_au32IspSnsId[ISP_MAX_DEV_NUM] = { 0, 1, 2, 4, 5};
/* NanoKVM change: which ISP devs had their sensor initialised by SAMPLE_COMM_ISP_Run */
static CVI_BOOL g_bSnsRunning[VI_MAX_DEV_NUM];

SAMPLE_SNS_TYPE_E g_enSnsType[VI_MAX_DEV_NUM] = {
	SONY_IMX327_MIPI_2M_30FPS_12BIT,
	SONY_IMX290_MIPI_1M_30FPS_12BIT,
};

static ISP_INIT_ATTR_S gstInitAttr[ISP_MAX_DEV_NUM];

CVI_S32 SAMPLE_COMM_ISP_GetIspAttrBySns(SAMPLE_SNS_TYPE_E enSnsType, ISP_PUB_ATTR_S *pstPubAttr)
{
	CVI_S32 s32Ret = CVI_SUCCESS;

	s32Ret = SAMPLE_COMM_SNS_GetIspAttrBySns(enSnsType, pstPubAttr);
	if (s32Ret != CVI_SUCCESS) {
		CVI_TRACE_LOG(CVI_DBG_ERR, "SAMPLE_COMM_SNS_GetIspAttrBySns failed with %#x\n", s32Ret);
		return s32Ret;
	}
	return s32Ret;
}

/*
 * NanoKVM change: replaces the ISP thread (CVI_ISP_Run -> CVI_ISP_Init). The closed libisp
 * ran the sensor object's pfn_cmos_sensor_global_init and then pfn_cmos_sensor_init
 * (lt6911_init: opens the I2C fd that the EDID and resolution reads use); do exactly that.
 */
CVI_S32 SAMPLE_COMM_ISP_Run(CVI_U8 IspDev)
{
	ISP_SENSOR_EXP_FUNC_S stSnsrSensorFunc;
	const ISP_SNS_OBJ_S *pstSnsObj;

	if (IspDev >= VI_MAX_DEV_NUM) {
		CVI_TRACE_LOG(CVI_DBG_ERR, "invalid isp dev %d\n", IspDev);
		return CVI_FAILURE;
	}
	/* the original thread returned at once for IspDev > 0 */
	if (IspDev > 0)
		return CVI_SUCCESS;
	if (g_bSnsRunning[IspDev])
		return CVI_SUCCESS;

	pstSnsObj = (ISP_SNS_OBJ_S *)SAMPLE_COMM_ISP_GetSnsObj(g_au32IspSnsId[IspDev]);
	if (pstSnsObj == CVI_NULL || pstSnsObj->pfnExpSensorCb == CVI_NULL) {
		CVI_TRACE_LOG(CVI_DBG_ERR, "no sensor object for isp dev %d\n", IspDev);
		return CVI_FAILURE;
	}
	memset(&stSnsrSensorFunc, 0, sizeof(stSnsrSensorFunc));
	if (pstSnsObj->pfnExpSensorCb(&stSnsrSensorFunc) != CVI_SUCCESS) {
		CVI_TRACE_LOG(CVI_DBG_ERR, "sensor exp function failed for isp dev %d\n", IspDev);
		return CVI_FAILURE;
	}
	if (stSnsrSensorFunc.pfn_cmos_sensor_global_init)
		stSnsrSensorFunc.pfn_cmos_sensor_global_init(IspDev);
	if (stSnsrSensorFunc.pfn_cmos_sensor_init)
		stSnsrSensorFunc.pfn_cmos_sensor_init(IspDev);
	g_bSnsRunning[IspDev] = CVI_TRUE;

	return CVI_SUCCESS;
}

/******************************************************************************
 * funciton : stop ISP (NanoKVM change: exit the sensor, nothing else to stop)
 ******************************************************************************/
CVI_VOID SAMPLE_COMM_ISP_Stop(CVI_U8 IspDev)
{
	ISP_SENSOR_EXP_FUNC_S stSnsrSensorFunc;
	const ISP_SNS_OBJ_S *pstSnsObj;

	if (IspDev >= VI_MAX_DEV_NUM || !g_bSnsRunning[IspDev])
		return;

	pstSnsObj = (ISP_SNS_OBJ_S *)SAMPLE_COMM_ISP_GetSnsObj(g_au32IspSnsId[IspDev]);
	if (pstSnsObj != CVI_NULL && pstSnsObj->pfnExpSensorCb != CVI_NULL) {
		memset(&stSnsrSensorFunc, 0, sizeof(stSnsrSensorFunc));
		if (pstSnsObj->pfnExpSensorCb(&stSnsrSensorFunc) == CVI_SUCCESS &&
		    stSnsrSensorFunc.pfn_cmos_sensor_exit)
			stSnsrSensorFunc.pfn_cmos_sensor_exit(IspDev);
	}
	g_bSnsRunning[IspDev] = CVI_FALSE;
	SAMPLE_COMM_ISP_Sensor_UnRegiter_callback(IspDev);
	SAMPLE_COMM_ISP_Aelib_UnCallback(IspDev);
	SAMPLE_COMM_ISP_Awblib_UnCallback(IspDev);
	#if ENABLE_AF_LIB
	SAMPLE_COMM_ISP_Aflib_UnCallback(IspDev);
	#endif
}

CVI_VOID SAMPLE_COMM_All_ISP_Stop(CVI_VOID)
{
	for (ISP_DEV IspDev = 0; IspDev < VI_MAX_DEV_NUM; IspDev++)
		SAMPLE_COMM_ISP_Stop(IspDev);
}

/*
 * NanoKVM change: the AE/AWB/AF algorithm libraries are not linked. These stay exported
 * (the symbol list of libkvm_mmf is unchanged) and register nothing.
 */
CVI_S32 SAMPLE_COMM_ISP_Awblib_Callback(ISP_DEV IspDev)
{
	(void)IspDev;
	return CVI_SUCCESS;
}

CVI_S32 SAMPLE_COMM_ISP_Awblib_UnCallback(ISP_DEV IspDev)
{
	(void)IspDev;
	return CVI_SUCCESS;
}

CVI_S32 SAMPLE_COMM_ISP_Aelib_Callback(ISP_DEV IspDev)
{
	(void)IspDev;
	return CVI_SUCCESS;
}

CVI_S32 SAMPLE_COMM_ISP_Aelib_UnCallback(ISP_DEV IspDev)
{
	(void)IspDev;
	return CVI_SUCCESS;
}

CVI_S32 SAMPLE_COMM_ISP_Aflib_Callback(ISP_DEV IspDev)
{
	(void)IspDev;
	return CVI_SUCCESS;
}

CVI_S32 SAMPLE_COMM_ISP_Aflib_UnCallback(ISP_DEV IspDev)
{
	(void)IspDev;
	return CVI_SUCCESS;
}


CVI_S32 SAMPLE_COMM_ISP_SetSnsObj(CVI_U32 u32SnsId, SAMPLE_SNS_TYPE_E enSnsType)
{
	if (u32SnsId >= ARRAY_SIZE(g_enSnsType))
		return CVI_FAILURE;

	g_enSnsType[u32SnsId] = enSnsType;
	return CVI_SUCCESS;
}

CVI_S32 SAMPLE_COMM_ISP_SetSnsInit(CVI_U32 u32SnsId, CVI_U8 u8HwSync)
{
	if (u32SnsId >= ARRAY_SIZE(g_enSnsType))
		return CVI_FAILURE;

	gstInitAttr[u32SnsId].u16UseHwSync = u8HwSync;

	return CVI_SUCCESS;
}

CVI_VOID *SAMPLE_COMM_ISP_GetSnsObj(CVI_U32 u32SnsId)
{
	SAMPLE_SNS_TYPE_E enSnsType;

	enSnsType = g_enSnsType[u32SnsId];
	return SAMPLE_COMM_SNS_GetSnsObj(enSnsType);
}

CVI_S32 SAMPLE_COMM_ISP_PatchSnsObj(CVI_U32 u32SnsId, SAMPLE_SENSOR_INFO_S *pstSnsInfo)
{
	ISP_SNS_OBJ_S *pstSnsObj = (ISP_SNS_OBJ_S *)SAMPLE_COMM_ISP_GetSnsObj(u32SnsId);
	RX_INIT_ATTR_S stRxInitAttr;
	unsigned int i;

	if (pstSnsObj == CVI_NULL) {
		CVI_TRACE_LOG(CVI_DBG_ERR, "fail to find pstSnsObj\n");
		return CVI_FAILURE;
	}

	memset(&stRxInitAttr, 0, sizeof(RX_INIT_ATTR_S));

	stRxInitAttr.MipiDev = pstSnsInfo->MipiDev;
	if (pstSnsInfo->stMclkAttr.bMclkEn) {
		stRxInitAttr.stMclkAttr.bMclkEn = CVI_TRUE;
		stRxInitAttr.stMclkAttr.u8Mclk  = pstSnsInfo->stMclkAttr.u8Mclk;
	}

	for (i = 0; i < sizeof(stRxInitAttr.as16LaneId)/sizeof(CVI_S16); i++) {
		stRxInitAttr.as16LaneId[i] = pstSnsInfo->as16LaneId[i];
	}
	for (i = 0; i < sizeof(stRxInitAttr.as8PNSwap)/sizeof(CVI_S8); i++) {
		stRxInitAttr.as8PNSwap[i] = pstSnsInfo->as8PNSwap[i];
	}

	return (pstSnsObj->pfnPatchRxAttr) ? pstSnsObj->pfnPatchRxAttr(&stRxInitAttr) : CVI_SUCCESS;
}

#if 0
CVI_S32 SAMPLE_COMM_ISP_GetRxAttr(SAMPLE_VI_CONFIG_S *pstViConfig)
{
	CVI_S32 s32Ret, i;
	ISP_SENSOR_EXP_FUNC_S stSnsrSensorFunc;
	SNS_COMBO_DEV_ATTR_S stDevAttr;
	const ISP_SNS_OBJ_S *pstSnsObj;

	for (i = 0; i < pstViConfig->s32WorkingViNum; i++) {
		pstViInfo = &pstViConfig->astViInfo[i];
		ViPipe = pstViInfo->stPipeInfo.aPipe[i];
		u32SnsId = pstViInfo->stSnsInfo.s32SnsId;

		pstSnsObj = (ISP_SNS_OBJ_S *)SAMPLE_COMM_ISP_GetSnsObj(u32SnsId);
		pstSnsObj.pfnGetRxAttr(ViPipe, &stDevAttr);
	}
}
#endif

CVI_S32 SAMPLE_COMM_ISP_SetSensorMode(SAMPLE_VI_CONFIG_S *pstViConfig)
{
	CVI_S32 s32Ret = CVI_SUCCESS, i;
	CVI_U32 u32SnsId;
	VI_PIPE ViPipe;
	WDR_MODE_E wdrMode;
	ISP_PUB_ATTR_S stPubAttr;
	ISP_SENSOR_EXP_FUNC_S stSnsrSensorFunc;
	ISP_CMOS_SENSOR_IMAGE_MODE_S stSnsrMode;
	SAMPLE_VI_INFO_S *pstViInfo = CVI_NULL;
	const ISP_SNS_OBJ_S *pstSnsObj;

	for (i = 0; i < pstViConfig->s32WorkingViNum; i++) {
		pstViInfo = &pstViConfig->astViInfo[i];
		ViPipe = pstViInfo->stPipeInfo.aPipe[0];
		wdrMode = pstViInfo->stDevInfo.enWDRMode;
		u32SnsId = pstViInfo->stSnsInfo.s32SnsId;

		pstSnsObj = (ISP_SNS_OBJ_S *)SAMPLE_COMM_ISP_GetSnsObj(u32SnsId);
		if (SAMPLE_COMM_ISP_GetIspAttrBySns(pstViInfo->stSnsInfo.enSnsType, &stPubAttr) != CVI_SUCCESS) {
			CVI_TRACE_LOG(CVI_DBG_ERR, "Can't get sns attr!\n");
			return s32Ret;
		}
		stSnsrMode.u16Width = stPubAttr.stSnsSize.u32Width;
		stSnsrMode.u16Height = stPubAttr.stSnsSize.u32Height;
		stSnsrMode.f32Fps = stPubAttr.f32FrameRate;
		printf("stSnsrMode.u16Width %d stSnsrMode.u16Height %d %f wdrMode %d pstSnsObj %p\n",
		       stSnsrMode.u16Width, stSnsrMode.u16Height, stSnsrMode.f32Fps, wdrMode, pstSnsObj);
		pstSnsObj->pfnExpSensorCb(&stSnsrSensorFunc);

		if (stSnsrSensorFunc.pfn_cmos_set_image_mode) {
			s32Ret = stSnsrSensorFunc.pfn_cmos_set_image_mode(ViPipe, &stSnsrMode);
			if (s32Ret != CVI_SUCCESS) {
				CVI_TRACE_LOG(CVI_DBG_ERR, "sensor set image mode failed!\n");
				return s32Ret;
			}
		}

		if (stSnsrSensorFunc.pfn_cmos_set_wdr_mode) {
			s32Ret = stSnsrSensorFunc.pfn_cmos_set_wdr_mode(ViPipe, wdrMode);
			if (s32Ret != CVI_SUCCESS) {
				CVI_TRACE_LOG(CVI_DBG_ERR, "sensor set wdr mode failed!\n");
				return s32Ret;
			}
		}
	}
	return s32Ret;
}

static SNS_BDG_MUX_MODE_E SAMPLE_COMM_ISP_GetSnsBdgMode(SAMPLE_SNS_TYPE_E enSnsType)
{
	VI_DEV_ATTR_S       stViDevAttr;
	SNS_BDG_MUX_MODE_E  MuxMode;

	SAMPLE_COMM_VI_GetDevAttrBySns(enSnsType, &stViDevAttr);
	switch (stViDevAttr.enWorkMode) {
	case VI_WORK_MODE_1Multiplex:
		MuxMode = SNS_BDG_MUX_NONE;
		break;
	case VI_WORK_MODE_2Multiplex:
		MuxMode = SNS_BDG_MUX_2;
		break;
	case VI_WORK_MODE_3Multiplex:
		MuxMode = SNS_BDG_MUX_3;
		break;
	case VI_WORK_MODE_4Multiplex:
		MuxMode = SNS_BDG_MUX_4;
		break;
	default:
		MuxMode = SNS_BDG_MUX_NONE;
		break;
	}

	return MuxMode;
}

CVI_S32 SAMPLE_COMM_ISP_Sensor_Regiter_callback(ISP_DEV IspDev, CVI_U32 u32SnsId, CVI_S32 s32BusId,
						CVI_S32 s32I2cAddr)
{
	CVI_S32 s32Ret = -1;
	SAMPLE_SNS_TYPE_E enSnsType = g_enSnsType[u32SnsId];
	ALG_LIB_S stAeLib;
	ALG_LIB_S stAwbLib;
	const ISP_SNS_OBJ_S *pstSnsObj;
	ISP_INIT_ATTR_S *pstInitAttr = &gstInitAttr[u32SnsId];
	ISP_SNS_COMMBUS_U unSnsrBusInfo = {
		.s8I2cDev = 3,
	};

	#define SNSBUS_VLD(x)		(x >= 0)

	if (u32SnsId > VI_MAX_DEV_NUM) {
		CVI_TRACE_LOG(CVI_DBG_ERR, "invalid sensor id: %d\n", u32SnsId);
		return CVI_FAILURE;
	}

	pstSnsObj = (ISP_SNS_OBJ_S *)SAMPLE_COMM_ISP_GetSnsObj(u32SnsId);
	if (pstSnsObj == CVI_NULL) {
		CVI_TRACE_LOG(CVI_DBG_ERR, "sensor %d not exist!\n", u32SnsId);
		return CVI_FAILURE;
	}

	pstInitAttr->enGainMode = SNS_GAIN_MODE_SHARE;
	if ((enSnsType == SOI_F35_MIPI_2M_30FPS_10BIT_WDR2TO1) ||
		(enSnsType == SOI_F35_SLAVE_MIPI_2M_30FPS_10BIT_WDR2TO1) ||
		(enSnsType == OV_OS08A20_MIPI_8M_30FPS_10BIT_WDR2TO1) ||
		(enSnsType == OV_OS08A20_MIPI_5M_30FPS_10BIT_WDR2TO1) ||
		(enSnsType == OV_OS08A20_MIPI_4M_30FPS_10BIT_WDR2TO1) ||
		(enSnsType == OV_OS08A20_SLAVE_MIPI_4M_30FPS_10BIT_WDR2TO1)) {
		pstInitAttr->enL2SMode = SNS_L2S_MODE_FIX;
	}
	pstInitAttr->enSnsBdgMuxMode = SAMPLE_COMM_ISP_GetSnsBdgMode(enSnsType);
	if (pstSnsObj->pfnSetInit) {
		s32Ret = pstSnsObj->pfnSetInit(IspDev, pstInitAttr);
		if (s32Ret < 0) {
			CVI_TRACE_LOG(CVI_DBG_ERR, "pfnSetInit error id: %d s32Ret %d\n", IspDev, s32Ret);
			return CVI_FAILURE;
		}
	}
	/* set i2c bus info */
	if (SNSBUS_VLD(s32BusId))
		unSnsrBusInfo.s8I2cDev = (CVI_S8)s32BusId;
	s32Ret = pstSnsObj->pfnSetBusInfo(IspDev, unSnsrBusInfo);
	if (s32Ret < 0) {
		CVI_TRACE_LOG(CVI_DBG_ERR, "pfnSetBusInfo error id: %d s32Ret %d\n", IspDev, s32Ret);
		return CVI_FAILURE;
	}
	if (pstSnsObj->pfnPatchI2cAddr)
		pstSnsObj->pfnPatchI2cAddr(s32I2cAddr);

	/* NanoKVM change: no AE/AWB algorithm libraries; the sensor ignores the names. */
	memset(&stAeLib, 0, sizeof(stAeLib));
	memset(&stAwbLib, 0, sizeof(stAwbLib));
	stAeLib.s32Id = IspDev;
	stAwbLib.s32Id = IspDev;

	if (pstSnsObj->pfnRegisterCallback != CVI_NULL) {
		s32Ret = pstSnsObj->pfnRegisterCallback(IspDev, &stAeLib, &stAwbLib);

		if (s32Ret != CVI_SUCCESS) {
			CVI_TRACE_LOG(CVI_DBG_ERR, "sensor_register_callback failed with %#x!\n", s32Ret);
			return s32Ret;
		}
	} else {
		CVI_TRACE_LOG(CVI_DBG_ERR, "sensor_register_callback failed with CVI_NULL!\n");
		return CVI_FAILURE;
	}

	g_au32IspSnsId[IspDev] = u32SnsId;

	return CVI_SUCCESS;
}

CVI_S32 SAMPLE_COMM_ISP_Sensor_UnRegiter_callback(ISP_DEV IspDev)
{
	ALG_LIB_S stAeLib;
	ALG_LIB_S stAwbLib;
	CVI_U32 u32SnsId;
	const ISP_SNS_OBJ_S *pstSnsObj;
	CVI_S32 s32Ret = -1;

	u32SnsId = g_au32IspSnsId[IspDev];

	if (u32SnsId > VI_MAX_DEV_NUM) {
		SAMPLE_PRT("%s: invalid sensor id: %d\n", __func__, u32SnsId);
		return CVI_FAILURE;
	}

	pstSnsObj = (ISP_SNS_OBJ_S *)SAMPLE_COMM_ISP_GetSnsObj(u32SnsId);

	if (pstSnsObj == CVI_NULL) {
		return CVI_FAILURE;
	}

	/* NanoKVM change: no AE/AWB algorithm libraries; the sensor ignores the names. */
	memset(&stAeLib, 0, sizeof(stAeLib));
	memset(&stAwbLib, 0, sizeof(stAwbLib));
	stAeLib.s32Id = IspDev;
	stAwbLib.s32Id = IspDev;

	if (pstSnsObj->pfnUnRegisterCallback != CVI_NULL) {
		s32Ret = pstSnsObj->pfnUnRegisterCallback(IspDev, &stAeLib, &stAwbLib);

		if (s32Ret != CVI_SUCCESS) {
			SAMPLE_PRT("sensor_unregister_callback failed with %#x!\n", s32Ret);
			return s32Ret;
		}
	} else {
		SAMPLE_PRT("sensor_unregister_callback failed with CVI_NULL!\n");
	}

	return CVI_SUCCESS;
}
