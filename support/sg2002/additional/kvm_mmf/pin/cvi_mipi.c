/*
 * NanoKVM: the CVI_MIPI_* calls of the MIPI RX driver (/dev/cvi-mipi-rx), taken from the
 * open source of the middleware branch maix_mmf-cvisdk (cd8bb74)
 * modules/isp/cv181x/isp/src/cvi_mipi.c, Copyright (C) Cvitek Co., Ltd. 2019-2021.
 *
 * At the 4e231ef pin these live inside the closed libisp.so, which NanoKVM does not link
 * (the LT6911 delivers YUV and the VI pipe bypasses the ISP). pin/sample_common_vi.c still
 * resets the MIPI RX, sets the sensor clock and the MIPI attributes through them, so the
 * driver ioctls are issued here. Changes from the original: plain open() instead of the
 * ISP's open_device(), no ISP_LOG_ERR (perror-style fprintf), no CVI_MIPI_SetSnsMclk (not
 * called), and the symbols are hidden so the export list of libkvm_mmf.so is unchanged.
 */
#include <errno.h>
#include <fcntl.h>
#include <stdint.h>
#include <stdio.h>
#include <unistd.h>
#include <sys/ioctl.h>

#include <linux/cvi_type.h>
#include <linux/vi_snsr.h>
#include "cvi_sns_ctrl.h"
#include "cvi_isp.h"

#define MIPI_DEV_NODE "/dev/cvi-mipi-rx"

static CVI_S32 fd_mipi = -1;

#pragma GCC visibility push(hidden)

static CVI_S32 mipi_open_dev(CVI_VOID)
{
	fd_mipi = open(MIPI_DEV_NODE, O_RDWR | O_CLOEXEC);
	if (fd_mipi < 0) {
		fprintf(stderr, "open %s failed: errno %d\n", MIPI_DEV_NODE, errno);
		return CVI_FAILURE;
	}

	return CVI_SUCCESS;
}

CVI_S32 CVI_MIPI_SetMipiReset(CVI_S32 devno, CVI_U32 reset)
{
	CVI_S32 s32Ret = CVI_SUCCESS;
	CVI_S32 s32Devno = devno;

	if (fd_mipi < 0) {
		s32Ret = mipi_open_dev();
		if (s32Ret != CVI_SUCCESS)
			return s32Ret;
	}

	if (reset == 0) {
		if (ioctl(fd_mipi, CVI_MIPI_UNRESET_MIPI, (void *)(uintptr_t)&s32Devno) < 0) {
			fprintf(stderr, "CVI_MIPI_UNRESET_MIPI - %d NG\n", s32Devno);
			return errno;
		}
	} else {
		if (ioctl(fd_mipi, CVI_MIPI_RESET_MIPI, (void *)(uintptr_t)&s32Devno) < 0) {
			fprintf(stderr, "CVI_MIPI_RESET_MIPI - %d NG\n", s32Devno);
			return errno;
		}
	}
	return s32Ret;
}

CVI_S32 CVI_MIPI_SetSensorClock(CVI_S32 devno, CVI_U32 enable)
{
	CVI_S32 s32Ret = CVI_SUCCESS;
	CVI_S32 s32Devno = devno;

	if (fd_mipi < 0) {
		s32Ret = mipi_open_dev();
		if (s32Ret != CVI_SUCCESS)
			return s32Ret;
	}

	if (enable == 0) {
		if (ioctl(fd_mipi, CVI_MIPI_DISABLE_SENSOR_CLOCK, (void *)(uintptr_t)&s32Devno) < 0) {
			fprintf(stderr, "CVI_MIPI_DISABLE_SENSOR_CLOCK - %d NG\n", s32Devno);
			return errno;
		}
	} else {
		if (ioctl(fd_mipi, CVI_MIPI_ENABLE_SENSOR_CLOCK, (void *)(uintptr_t)&s32Devno) < 0) {
			fprintf(stderr, "CVI_MIPI_ENABLE_SENSOR_CLOCK - %d NG\n", s32Devno);
			return errno;
		}
	}
	return s32Ret;
}

CVI_S32 CVI_MIPI_SetSensorReset(CVI_S32 devno, CVI_U32 reset)
{
	CVI_S32 s32Ret = CVI_SUCCESS;
	CVI_S32 s32Devno = devno;

	if (fd_mipi < 0) {
		s32Ret = mipi_open_dev();
		if (s32Ret != CVI_SUCCESS)
			return s32Ret;
	}

	if (reset == 0) {
		if (ioctl(fd_mipi, CVI_MIPI_UNRESET_SENSOR, (void *)(uintptr_t)&s32Devno) < 0) {
			fprintf(stderr, "CVI_MIPI_UNRESET_SENSOR - %d NG\n", s32Devno);
			return errno;
		}
	} else {
		if (ioctl(fd_mipi, CVI_MIPI_RESET_SENSOR, (void *)(uintptr_t)&s32Devno) < 0) {
			fprintf(stderr, "CVI_MIPI_RESET_SENSOR - %d NG\n", s32Devno);
			return errno;
		}
	}
	return s32Ret;
}

CVI_S32 CVI_MIPI_SetMipiAttr(CVI_S32 ViPipe, const CVI_VOID *devAttr)
{
	CVI_S32 s32Ret = 0;
	SNS_COMBO_DEV_ATTR_S *comboAttr;

	if ((ViPipe < 0) || (ViPipe >= VI_MAX_PIPE_NUM)) {
		fprintf(stderr, "ViPipe %d value error\n", ViPipe);
		return -ENODEV;
	}

	if (devAttr == CVI_NULL)
		return CVI_FAILURE;

	if (fd_mipi < 0) {
		s32Ret = mipi_open_dev();
		if (s32Ret != CVI_SUCCESS)
			return s32Ret;
	}

	comboAttr = (SNS_COMBO_DEV_ATTR_S *)devAttr;
	if (ioctl(fd_mipi, CVI_MIPI_SET_DEV_ATTR, (void *)(uintptr_t)comboAttr) < 0) {
		fprintf(stderr, "CVI_MIPI_SET_DEV_ATTR NG\n");
		return errno;
	}
	return s32Ret;
}

CVI_S32 CVI_MIPI_SetClkEdge(CVI_S32 devno, CVI_U32 is_up)
{
	CVI_S32 s32Ret = CVI_SUCCESS;
	struct clk_edge_s clk;

	if (fd_mipi < 0) {
		s32Ret = mipi_open_dev();
		if (s32Ret != CVI_SUCCESS)
			return s32Ret;
	}

	clk.devno = devno;
	clk.edge = is_up ? CLK_UP_EDGE : CLK_DOWN_EDGE;
	if (ioctl(fd_mipi, CVI_MIPI_SET_OUTPUT_CLK_EDGE, (void *)(uintptr_t)&clk) < 0) {
		fprintf(stderr, "CVI_MIPI_SET_OUTPUT_CLK_EDGE - %d NG\n", devno);
		return errno;
	}
	return s32Ret;
}

#pragma GCC visibility pop
