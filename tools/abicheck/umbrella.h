/*
 * umbrella.h - one include order for every uapi header the KVM video path
 * (and the neighbours that share its structs) touches. The vendor headers are
 * not self-contained, so the order matters; both sides use this same file so
 * that a difference in the result is a difference in the headers, not in the
 * include order.
 *
 * VB_POOL is a kapi typedef (kernel side) and cvi_comm_vb.h (middleware
 * side); it is a plain u32 in both, so it is declared once here.
 */
#ifndef ABICHECK_UMBRELLA_H
#define ABICHECK_UMBRELLA_H

#include <stdint.h>
#include <stdio.h>
#include <string.h>
#include <linux/types.h>
#include <sys/ioctl.h>

typedef uint32_t VB_POOL;

#include <linux/cvi_type.h>
#include <linux/cvi_common.h>
#include <linux/cvi_comm_video.h>
#include <linux/cvi_comm_sys.h>
#include <linux/cvi_buffer.h>
#include <linux/cvi_comm_vi.h>
#include <linux/cvi_comm_vpss.h>
#include <linux/cvi_comm_region.h>
#include <linux/cvi_comm_vo.h>
#include <linux/cvi_comm_gdc.h>
#include <linux/cvi_comm_rc.h>
#include <linux/cvi_comm_venc.h>
#include <linux/cvi_comm_vdec.h>
#include <linux/sys_uapi.h>
#include <linux/vb_uapi.h>
#include <linux/cvi_vc_drv_ioctl.h>
#include <linux/vi_uapi.h>
#include <linux/vo_uapi.h>
#include <linux/vpss_uapi.h>
#include <linux/rgn_uapi.h>
#include <linux/dwa_uapi.h>
#include <linux/cvi_base.h>
#include <linux/cvi_tpu_ioctl.h>

#endif
