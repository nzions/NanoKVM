#ifdef CVITPU_SUBMIT_PIO
	ioctl_line("CVITPU_SUBMIT_PIO", (unsigned long)(CVITPU_SUBMIT_PIO));
#else
	missing_line("CVITPU_SUBMIT_PIO");
#endif
#ifdef CVITPU_WAIT_PIO
	ioctl_line("CVITPU_WAIT_PIO", (unsigned long)(CVITPU_WAIT_PIO));
#else
	missing_line("CVITPU_WAIT_PIO");
#endif
#ifdef IOCTL_READ_CHIP_ID
	ioctl_line("IOCTL_READ_CHIP_ID", (unsigned long)(IOCTL_READ_CHIP_ID));
#else
	missing_line("IOCTL_READ_CHIP_ID");
#endif
#ifdef IOCTL_READ_CHIP_PWR_ON_REASON
	ioctl_line("IOCTL_READ_CHIP_PWR_ON_REASON", (unsigned long)(IOCTL_READ_CHIP_PWR_ON_REASON));
#else
	missing_line("IOCTL_READ_CHIP_PWR_ON_REASON");
#endif
#ifdef IOCTL_READ_CHIP_VERSION
	ioctl_line("IOCTL_READ_CHIP_VERSION", (unsigned long)(IOCTL_READ_CHIP_VERSION));
#else
	missing_line("IOCTL_READ_CHIP_VERSION");
#endif
#ifdef IOCTL_VB_CMD
	ioctl_line("IOCTL_VB_CMD", (unsigned long)(IOCTL_VB_CMD));
#else
	missing_line("IOCTL_VB_CMD");
#endif
#ifdef RGN_IOC_G_CTRL
	ioctl_line("RGN_IOC_G_CTRL", (unsigned long)(RGN_IOC_G_CTRL));
#else
	missing_line("RGN_IOC_G_CTRL");
#endif
#ifdef RGN_IOC_S_CTRL
	ioctl_line("RGN_IOC_S_CTRL", (unsigned long)(RGN_IOC_S_CTRL));
#else
	missing_line("RGN_IOC_S_CTRL");
#endif
#ifdef SYS_CACHE_FLUSH
	ioctl_line("SYS_CACHE_FLUSH", (unsigned long)(SYS_CACHE_FLUSH));
#else
	missing_line("SYS_CACHE_FLUSH");
#endif
#ifdef SYS_CACHE_INVLD
	ioctl_line("SYS_CACHE_INVLD", (unsigned long)(SYS_CACHE_INVLD));
#else
	missing_line("SYS_CACHE_INVLD");
#endif
#ifdef SYS_GET_BINDCFG
	ioctl_line("SYS_GET_BINDCFG", (unsigned long)(SYS_GET_BINDCFG));
#else
	missing_line("SYS_GET_BINDCFG");
#endif
#ifdef SYS_IOC_G_CTRL
	ioctl_line("SYS_IOC_G_CTRL", (unsigned long)(SYS_IOC_G_CTRL));
#else
	missing_line("SYS_IOC_G_CTRL");
#endif
#ifdef SYS_IOC_S_CTRL
	ioctl_line("SYS_IOC_S_CTRL", (unsigned long)(SYS_IOC_S_CTRL));
#else
	missing_line("SYS_IOC_S_CTRL");
#endif
#ifdef SYS_ION_ALLOC
	ioctl_line("SYS_ION_ALLOC", (unsigned long)(SYS_ION_ALLOC));
#else
	missing_line("SYS_ION_ALLOC");
#endif
#ifdef SYS_ION_FREE
	ioctl_line("SYS_ION_FREE", (unsigned long)(SYS_ION_FREE));
#else
	missing_line("SYS_ION_FREE");
#endif
#ifdef SYS_ION_G_ION_MM_STATICS
	ioctl_line("SYS_ION_G_ION_MM_STATICS", (unsigned long)(SYS_ION_G_ION_MM_STATICS));
#else
	missing_line("SYS_ION_G_ION_MM_STATICS");
#endif
#ifdef SYS_SET_BINDCFG
	ioctl_line("SYS_SET_BINDCFG", (unsigned long)(SYS_SET_BINDCFG));
#else
	missing_line("SYS_SET_BINDCFG");
#endif
#ifdef VI_IOC_G_CTRL
	ioctl_line("VI_IOC_G_CTRL", (unsigned long)(VI_IOC_G_CTRL));
#else
	missing_line("VI_IOC_G_CTRL");
#endif
#ifdef VI_IOC_S_CTRL
	ioctl_line("VI_IOC_S_CTRL", (unsigned long)(VI_IOC_S_CTRL));
#else
	missing_line("VI_IOC_S_CTRL");
#endif
#ifdef VO_IOC_G_CTRL
	ioctl_line("VO_IOC_G_CTRL", (unsigned long)(VO_IOC_G_CTRL));
#else
	missing_line("VO_IOC_G_CTRL");
#endif
#ifdef VO_IOC_S_CTRL
	ioctl_line("VO_IOC_S_CTRL", (unsigned long)(VO_IOC_S_CTRL));
#else
	missing_line("VO_IOC_S_CTRL");
#endif
#ifdef CVI_VC_VDEC_ATTACH_VBPOOL
	ioctl_line("CVI_VC_VDEC_ATTACH_VBPOOL", (unsigned long)(CVI_VC_VDEC_ATTACH_VBPOOL));
#else
	missing_line("CVI_VC_VDEC_ATTACH_VBPOOL");
#endif
#ifdef CVI_VC_VDEC_CREATE_CHN
	ioctl_line("CVI_VC_VDEC_CREATE_CHN", (unsigned long)(CVI_VC_VDEC_CREATE_CHN));
#else
	missing_line("CVI_VC_VDEC_CREATE_CHN");
#endif
#ifdef CVI_VC_VDEC_DESTROY_CHN
	ioctl_line("CVI_VC_VDEC_DESTROY_CHN", (unsigned long)(CVI_VC_VDEC_DESTROY_CHN));
#else
	missing_line("CVI_VC_VDEC_DESTROY_CHN");
#endif
#ifdef CVI_VC_VDEC_DETACH_VBPOOL
	ioctl_line("CVI_VC_VDEC_DETACH_VBPOOL", (unsigned long)(CVI_VC_VDEC_DETACH_VBPOOL));
#else
	missing_line("CVI_VC_VDEC_DETACH_VBPOOL");
#endif
#ifdef CVI_VC_VDEC_GET_CHN_ATTR
	ioctl_line("CVI_VC_VDEC_GET_CHN_ATTR", (unsigned long)(CVI_VC_VDEC_GET_CHN_ATTR));
#else
	missing_line("CVI_VC_VDEC_GET_CHN_ATTR");
#endif
#ifdef CVI_VC_VDEC_GET_CHN_PARAM
	ioctl_line("CVI_VC_VDEC_GET_CHN_PARAM", (unsigned long)(CVI_VC_VDEC_GET_CHN_PARAM));
#else
	missing_line("CVI_VC_VDEC_GET_CHN_PARAM");
#endif
#ifdef CVI_VC_VDEC_GET_FRAME
	ioctl_line("CVI_VC_VDEC_GET_FRAME", (unsigned long)(CVI_VC_VDEC_GET_FRAME));
#else
	missing_line("CVI_VC_VDEC_GET_FRAME");
#endif
#ifdef CVI_VC_VDEC_GET_MOD_PARAM
	ioctl_line("CVI_VC_VDEC_GET_MOD_PARAM", (unsigned long)(CVI_VC_VDEC_GET_MOD_PARAM));
#else
	missing_line("CVI_VC_VDEC_GET_MOD_PARAM");
#endif
#ifdef CVI_VC_VDEC_QUERY_STATUS
	ioctl_line("CVI_VC_VDEC_QUERY_STATUS", (unsigned long)(CVI_VC_VDEC_QUERY_STATUS));
#else
	missing_line("CVI_VC_VDEC_QUERY_STATUS");
#endif
#ifdef CVI_VC_VDEC_RELEASE_FRAME
	ioctl_line("CVI_VC_VDEC_RELEASE_FRAME", (unsigned long)(CVI_VC_VDEC_RELEASE_FRAME));
#else
	missing_line("CVI_VC_VDEC_RELEASE_FRAME");
#endif
#ifdef CVI_VC_VDEC_RESET_CHN
	ioctl_line("CVI_VC_VDEC_RESET_CHN", (unsigned long)(CVI_VC_VDEC_RESET_CHN));
#else
	missing_line("CVI_VC_VDEC_RESET_CHN");
#endif
#ifdef CVI_VC_VDEC_SEND_STREAM
	ioctl_line("CVI_VC_VDEC_SEND_STREAM", (unsigned long)(CVI_VC_VDEC_SEND_STREAM));
#else
	missing_line("CVI_VC_VDEC_SEND_STREAM");
#endif
#ifdef CVI_VC_VDEC_SET_CHN_ATTR
	ioctl_line("CVI_VC_VDEC_SET_CHN_ATTR", (unsigned long)(CVI_VC_VDEC_SET_CHN_ATTR));
#else
	missing_line("CVI_VC_VDEC_SET_CHN_ATTR");
#endif
#ifdef CVI_VC_VDEC_SET_CHN_PARAM
	ioctl_line("CVI_VC_VDEC_SET_CHN_PARAM", (unsigned long)(CVI_VC_VDEC_SET_CHN_PARAM));
#else
	missing_line("CVI_VC_VDEC_SET_CHN_PARAM");
#endif
#ifdef CVI_VC_VDEC_SET_MOD_PARAM
	ioctl_line("CVI_VC_VDEC_SET_MOD_PARAM", (unsigned long)(CVI_VC_VDEC_SET_MOD_PARAM));
#else
	missing_line("CVI_VC_VDEC_SET_MOD_PARAM");
#endif
#ifdef CVI_VC_VDEC_START_RECV_STREAM
	ioctl_line("CVI_VC_VDEC_START_RECV_STREAM", (unsigned long)(CVI_VC_VDEC_START_RECV_STREAM));
#else
	missing_line("CVI_VC_VDEC_START_RECV_STREAM");
#endif
#ifdef CVI_VC_VDEC_STOP_RECV_STREAM
	ioctl_line("CVI_VC_VDEC_STOP_RECV_STREAM", (unsigned long)(CVI_VC_VDEC_STOP_RECV_STREAM));
#else
	missing_line("CVI_VC_VDEC_STOP_RECV_STREAM");
#endif
#ifdef CVI_VC_VENC_ATTACH_VBPOOL
	ioctl_line("CVI_VC_VENC_ATTACH_VBPOOL", (unsigned long)(CVI_VC_VENC_ATTACH_VBPOOL));
#else
	missing_line("CVI_VC_VENC_ATTACH_VBPOOL");
#endif
#ifdef CVI_VC_VENC_CALC_FRAME_PARAM
	ioctl_line("CVI_VC_VENC_CALC_FRAME_PARAM", (unsigned long)(CVI_VC_VENC_CALC_FRAME_PARAM));
#else
	missing_line("CVI_VC_VENC_CALC_FRAME_PARAM");
#endif
#ifdef CVI_VC_VENC_CREATE_CHN
	ioctl_line("CVI_VC_VENC_CREATE_CHN", (unsigned long)(CVI_VC_VENC_CREATE_CHN));
#else
	missing_line("CVI_VC_VENC_CREATE_CHN");
#endif
#ifdef CVI_VC_VENC_DESTROY_CHN
	ioctl_line("CVI_VC_VENC_DESTROY_CHN", (unsigned long)(CVI_VC_VENC_DESTROY_CHN));
#else
	missing_line("CVI_VC_VENC_DESTROY_CHN");
#endif
#ifdef CVI_VC_VENC_DETACH_VBPOOL
	ioctl_line("CVI_VC_VENC_DETACH_VBPOOL", (unsigned long)(CVI_VC_VENC_DETACH_VBPOOL));
#else
	missing_line("CVI_VC_VENC_DETACH_VBPOOL");
#endif
#ifdef CVI_VC_VENC_ENABLE_SVC
	ioctl_line("CVI_VC_VENC_ENABLE_SVC", (unsigned long)(CVI_VC_VENC_ENABLE_SVC));
#else
	missing_line("CVI_VC_VENC_ENABLE_SVC");
#endif
#ifdef CVI_VC_VENC_GET_CHN_ATTR
	ioctl_line("CVI_VC_VENC_GET_CHN_ATTR", (unsigned long)(CVI_VC_VENC_GET_CHN_ATTR));
#else
	missing_line("CVI_VC_VENC_GET_CHN_ATTR");
#endif
#ifdef CVI_VC_VENC_GET_CHN_PARAM
	ioctl_line("CVI_VC_VENC_GET_CHN_PARAM", (unsigned long)(CVI_VC_VENC_GET_CHN_PARAM));
#else
	missing_line("CVI_VC_VENC_GET_CHN_PARAM");
#endif
#ifdef CVI_VC_VENC_GET_CUPREDICTION
	ioctl_line("CVI_VC_VENC_GET_CUPREDICTION", (unsigned long)(CVI_VC_VENC_GET_CUPREDICTION));
#else
	missing_line("CVI_VC_VENC_GET_CUPREDICTION");
#endif
#ifdef CVI_VC_VENC_GET_FRAMELOST_STRATEGY
	ioctl_line("CVI_VC_VENC_GET_FRAMELOST_STRATEGY", (unsigned long)(CVI_VC_VENC_GET_FRAMELOST_STRATEGY));
#else
	missing_line("CVI_VC_VENC_GET_FRAMELOST_STRATEGY");
#endif
#ifdef CVI_VC_VENC_GET_FRAME_PARAM
	ioctl_line("CVI_VC_VENC_GET_FRAME_PARAM", (unsigned long)(CVI_VC_VENC_GET_FRAME_PARAM));
#else
	missing_line("CVI_VC_VENC_GET_FRAME_PARAM");
#endif
#ifdef CVI_VC_VENC_GET_H264_Dblk
	ioctl_line("CVI_VC_VENC_GET_H264_Dblk", (unsigned long)(CVI_VC_VENC_GET_H264_Dblk));
#else
	missing_line("CVI_VC_VENC_GET_H264_Dblk");
#endif
#ifdef CVI_VC_VENC_GET_H264_ENTROPY
	ioctl_line("CVI_VC_VENC_GET_H264_ENTROPY", (unsigned long)(CVI_VC_VENC_GET_H264_ENTROPY));
#else
	missing_line("CVI_VC_VENC_GET_H264_ENTROPY");
#endif
#ifdef CVI_VC_VENC_GET_H264_INTRA_PRED
	ioctl_line("CVI_VC_VENC_GET_H264_INTRA_PRED", (unsigned long)(CVI_VC_VENC_GET_H264_INTRA_PRED));
#else
	missing_line("CVI_VC_VENC_GET_H264_INTRA_PRED");
#endif
#ifdef CVI_VC_VENC_GET_H264_SLICE_SPLIT
	ioctl_line("CVI_VC_VENC_GET_H264_SLICE_SPLIT", (unsigned long)(CVI_VC_VENC_GET_H264_SLICE_SPLIT));
#else
	missing_line("CVI_VC_VENC_GET_H264_SLICE_SPLIT");
#endif
#ifdef CVI_VC_VENC_GET_H264_TRANS
	ioctl_line("CVI_VC_VENC_GET_H264_TRANS", (unsigned long)(CVI_VC_VENC_GET_H264_TRANS));
#else
	missing_line("CVI_VC_VENC_GET_H264_TRANS");
#endif
#ifdef CVI_VC_VENC_GET_H264_VUI
	ioctl_line("CVI_VC_VENC_GET_H264_VUI", (unsigned long)(CVI_VC_VENC_GET_H264_VUI));
#else
	missing_line("CVI_VC_VENC_GET_H264_VUI");
#endif
#ifdef CVI_VC_VENC_GET_H265_Dblk
	ioctl_line("CVI_VC_VENC_GET_H265_Dblk", (unsigned long)(CVI_VC_VENC_GET_H265_Dblk));
#else
	missing_line("CVI_VC_VENC_GET_H265_Dblk");
#endif
#ifdef CVI_VC_VENC_GET_H265_SLICE_SPLIT
	ioctl_line("CVI_VC_VENC_GET_H265_SLICE_SPLIT", (unsigned long)(CVI_VC_VENC_GET_H265_SLICE_SPLIT));
#else
	missing_line("CVI_VC_VENC_GET_H265_SLICE_SPLIT");
#endif
#ifdef CVI_VC_VENC_GET_H265_TRANS
	ioctl_line("CVI_VC_VENC_GET_H265_TRANS", (unsigned long)(CVI_VC_VENC_GET_H265_TRANS));
#else
	missing_line("CVI_VC_VENC_GET_H265_TRANS");
#endif
#ifdef CVI_VC_VENC_GET_H265_VUI
	ioctl_line("CVI_VC_VENC_GET_H265_VUI", (unsigned long)(CVI_VC_VENC_GET_H265_VUI));
#else
	missing_line("CVI_VC_VENC_GET_H265_VUI");
#endif
#ifdef CVI_VC_VENC_GET_JPEG_PARAM
	ioctl_line("CVI_VC_VENC_GET_JPEG_PARAM", (unsigned long)(CVI_VC_VENC_GET_JPEG_PARAM));
#else
	missing_line("CVI_VC_VENC_GET_JPEG_PARAM");
#endif
#ifdef CVI_VC_VENC_GET_MOD_PARAM
	ioctl_line("CVI_VC_VENC_GET_MOD_PARAM", (unsigned long)(CVI_VC_VENC_GET_MOD_PARAM));
#else
	missing_line("CVI_VC_VENC_GET_MOD_PARAM");
#endif
#ifdef CVI_VC_VENC_GET_RC_PARAM
	ioctl_line("CVI_VC_VENC_GET_RC_PARAM", (unsigned long)(CVI_VC_VENC_GET_RC_PARAM));
#else
	missing_line("CVI_VC_VENC_GET_RC_PARAM");
#endif
#ifdef CVI_VC_VENC_GET_REF_PARAM
	ioctl_line("CVI_VC_VENC_GET_REF_PARAM", (unsigned long)(CVI_VC_VENC_GET_REF_PARAM));
#else
	missing_line("CVI_VC_VENC_GET_REF_PARAM");
#endif
#ifdef CVI_VC_VENC_GET_ROI_ATTR
	ioctl_line("CVI_VC_VENC_GET_ROI_ATTR", (unsigned long)(CVI_VC_VENC_GET_ROI_ATTR));
#else
	missing_line("CVI_VC_VENC_GET_ROI_ATTR");
#endif
#ifdef CVI_VC_VENC_GET_STREAM
	ioctl_line("CVI_VC_VENC_GET_STREAM", (unsigned long)(CVI_VC_VENC_GET_STREAM));
#else
	missing_line("CVI_VC_VENC_GET_STREAM");
#endif
#ifdef CVI_VC_VENC_GET_SUPERFRAME_STRATEGY
	ioctl_line("CVI_VC_VENC_GET_SUPERFRAME_STRATEGY", (unsigned long)(CVI_VC_VENC_GET_SUPERFRAME_STRATEGY));
#else
	missing_line("CVI_VC_VENC_GET_SUPERFRAME_STRATEGY");
#endif
#ifdef CVI_VC_VENC_GET_SVC_PARAM
	ioctl_line("CVI_VC_VENC_GET_SVC_PARAM", (unsigned long)(CVI_VC_VENC_GET_SVC_PARAM));
#else
	missing_line("CVI_VC_VENC_GET_SVC_PARAM");
#endif
#ifdef CVI_VC_VENC_INSERT_USERDATA
	ioctl_line("CVI_VC_VENC_INSERT_USERDATA", (unsigned long)(CVI_VC_VENC_INSERT_USERDATA));
#else
	missing_line("CVI_VC_VENC_INSERT_USERDATA");
#endif
#ifdef CVI_VC_VENC_QUERY_STATUS
	ioctl_line("CVI_VC_VENC_QUERY_STATUS", (unsigned long)(CVI_VC_VENC_QUERY_STATUS));
#else
	missing_line("CVI_VC_VENC_QUERY_STATUS");
#endif
#ifdef CVI_VC_VENC_RELEASE_STREAM
	ioctl_line("CVI_VC_VENC_RELEASE_STREAM", (unsigned long)(CVI_VC_VENC_RELEASE_STREAM));
#else
	missing_line("CVI_VC_VENC_RELEASE_STREAM");
#endif
#ifdef CVI_VC_VENC_REQUEST_IDR
	ioctl_line("CVI_VC_VENC_REQUEST_IDR", (unsigned long)(CVI_VC_VENC_REQUEST_IDR));
#else
	missing_line("CVI_VC_VENC_REQUEST_IDR");
#endif
#ifdef CVI_VC_VENC_RESET_CHN
	ioctl_line("CVI_VC_VENC_RESET_CHN", (unsigned long)(CVI_VC_VENC_RESET_CHN));
#else
	missing_line("CVI_VC_VENC_RESET_CHN");
#endif
#ifdef CVI_VC_VENC_SEND_FRAME
	ioctl_line("CVI_VC_VENC_SEND_FRAME", (unsigned long)(CVI_VC_VENC_SEND_FRAME));
#else
	missing_line("CVI_VC_VENC_SEND_FRAME");
#endif
#ifdef CVI_VC_VENC_SEND_FRAMEEX
	ioctl_line("CVI_VC_VENC_SEND_FRAMEEX", (unsigned long)(CVI_VC_VENC_SEND_FRAMEEX));
#else
	missing_line("CVI_VC_VENC_SEND_FRAMEEX");
#endif
#ifdef CVI_VC_VENC_SET_CHN_ATTR
	ioctl_line("CVI_VC_VENC_SET_CHN_ATTR", (unsigned long)(CVI_VC_VENC_SET_CHN_ATTR));
#else
	missing_line("CVI_VC_VENC_SET_CHN_ATTR");
#endif
#ifdef CVI_VC_VENC_SET_CHN_PARAM
	ioctl_line("CVI_VC_VENC_SET_CHN_PARAM", (unsigned long)(CVI_VC_VENC_SET_CHN_PARAM));
#else
	missing_line("CVI_VC_VENC_SET_CHN_PARAM");
#endif
#ifdef CVI_VC_VENC_SET_CUPREDICTION
	ioctl_line("CVI_VC_VENC_SET_CUPREDICTION", (unsigned long)(CVI_VC_VENC_SET_CUPREDICTION));
#else
	missing_line("CVI_VC_VENC_SET_CUPREDICTION");
#endif
#ifdef CVI_VC_VENC_SET_FRAMELOST_STRATEGY
	ioctl_line("CVI_VC_VENC_SET_FRAMELOST_STRATEGY", (unsigned long)(CVI_VC_VENC_SET_FRAMELOST_STRATEGY));
#else
	missing_line("CVI_VC_VENC_SET_FRAMELOST_STRATEGY");
#endif
#ifdef CVI_VC_VENC_SET_FRAME_PARAM
	ioctl_line("CVI_VC_VENC_SET_FRAME_PARAM", (unsigned long)(CVI_VC_VENC_SET_FRAME_PARAM));
#else
	missing_line("CVI_VC_VENC_SET_FRAME_PARAM");
#endif
#ifdef CVI_VC_VENC_SET_H264_Dblk
	ioctl_line("CVI_VC_VENC_SET_H264_Dblk", (unsigned long)(CVI_VC_VENC_SET_H264_Dblk));
#else
	missing_line("CVI_VC_VENC_SET_H264_Dblk");
#endif
#ifdef CVI_VC_VENC_SET_H264_ENTROPY
	ioctl_line("CVI_VC_VENC_SET_H264_ENTROPY", (unsigned long)(CVI_VC_VENC_SET_H264_ENTROPY));
#else
	missing_line("CVI_VC_VENC_SET_H264_ENTROPY");
#endif
#ifdef CVI_VC_VENC_SET_H264_INTRA_PRED
	ioctl_line("CVI_VC_VENC_SET_H264_INTRA_PRED", (unsigned long)(CVI_VC_VENC_SET_H264_INTRA_PRED));
#else
	missing_line("CVI_VC_VENC_SET_H264_INTRA_PRED");
#endif
#ifdef CVI_VC_VENC_SET_H264_SLICE_SPLIT
	ioctl_line("CVI_VC_VENC_SET_H264_SLICE_SPLIT", (unsigned long)(CVI_VC_VENC_SET_H264_SLICE_SPLIT));
#else
	missing_line("CVI_VC_VENC_SET_H264_SLICE_SPLIT");
#endif
#ifdef CVI_VC_VENC_SET_H264_TRANS
	ioctl_line("CVI_VC_VENC_SET_H264_TRANS", (unsigned long)(CVI_VC_VENC_SET_H264_TRANS));
#else
	missing_line("CVI_VC_VENC_SET_H264_TRANS");
#endif
#ifdef CVI_VC_VENC_SET_H264_VUI
	ioctl_line("CVI_VC_VENC_SET_H264_VUI", (unsigned long)(CVI_VC_VENC_SET_H264_VUI));
#else
	missing_line("CVI_VC_VENC_SET_H264_VUI");
#endif
#ifdef CVI_VC_VENC_SET_H265_Dblk
	ioctl_line("CVI_VC_VENC_SET_H265_Dblk", (unsigned long)(CVI_VC_VENC_SET_H265_Dblk));
#else
	missing_line("CVI_VC_VENC_SET_H265_Dblk");
#endif
#ifdef CVI_VC_VENC_SET_H265_SLICE_SPLIT
	ioctl_line("CVI_VC_VENC_SET_H265_SLICE_SPLIT", (unsigned long)(CVI_VC_VENC_SET_H265_SLICE_SPLIT));
#else
	missing_line("CVI_VC_VENC_SET_H265_SLICE_SPLIT");
#endif
#ifdef CVI_VC_VENC_SET_H265_TRANS
	ioctl_line("CVI_VC_VENC_SET_H265_TRANS", (unsigned long)(CVI_VC_VENC_SET_H265_TRANS));
#else
	missing_line("CVI_VC_VENC_SET_H265_TRANS");
#endif
#ifdef CVI_VC_VENC_SET_H265_VUI
	ioctl_line("CVI_VC_VENC_SET_H265_VUI", (unsigned long)(CVI_VC_VENC_SET_H265_VUI));
#else
	missing_line("CVI_VC_VENC_SET_H265_VUI");
#endif
#ifdef CVI_VC_VENC_SET_JPEG_PARAM
	ioctl_line("CVI_VC_VENC_SET_JPEG_PARAM", (unsigned long)(CVI_VC_VENC_SET_JPEG_PARAM));
#else
	missing_line("CVI_VC_VENC_SET_JPEG_PARAM");
#endif
#ifdef CVI_VC_VENC_SET_MOD_PARAM
	ioctl_line("CVI_VC_VENC_SET_MOD_PARAM", (unsigned long)(CVI_VC_VENC_SET_MOD_PARAM));
#else
	missing_line("CVI_VC_VENC_SET_MOD_PARAM");
#endif
#ifdef CVI_VC_VENC_SET_RC_PARAM
	ioctl_line("CVI_VC_VENC_SET_RC_PARAM", (unsigned long)(CVI_VC_VENC_SET_RC_PARAM));
#else
	missing_line("CVI_VC_VENC_SET_RC_PARAM");
#endif
#ifdef CVI_VC_VENC_SET_REF_PARAM
	ioctl_line("CVI_VC_VENC_SET_REF_PARAM", (unsigned long)(CVI_VC_VENC_SET_REF_PARAM));
#else
	missing_line("CVI_VC_VENC_SET_REF_PARAM");
#endif
#ifdef CVI_VC_VENC_SET_ROI_ATTR
	ioctl_line("CVI_VC_VENC_SET_ROI_ATTR", (unsigned long)(CVI_VC_VENC_SET_ROI_ATTR));
#else
	missing_line("CVI_VC_VENC_SET_ROI_ATTR");
#endif
#ifdef CVI_VC_VENC_SET_SUPERFRAME_STRATEGY
	ioctl_line("CVI_VC_VENC_SET_SUPERFRAME_STRATEGY", (unsigned long)(CVI_VC_VENC_SET_SUPERFRAME_STRATEGY));
#else
	missing_line("CVI_VC_VENC_SET_SUPERFRAME_STRATEGY");
#endif
#ifdef CVI_VC_VENC_SET_SVC_PARAM
	ioctl_line("CVI_VC_VENC_SET_SVC_PARAM", (unsigned long)(CVI_VC_VENC_SET_SVC_PARAM));
#else
	missing_line("CVI_VC_VENC_SET_SVC_PARAM");
#endif
#ifdef CVI_VC_VENC_START_RECV_FRAME
	ioctl_line("CVI_VC_VENC_START_RECV_FRAME", (unsigned long)(CVI_VC_VENC_START_RECV_FRAME));
#else
	missing_line("CVI_VC_VENC_START_RECV_FRAME");
#endif
#ifdef CVI_VC_VENC_STOP_RECV_FRAME
	ioctl_line("CVI_VC_VENC_STOP_RECV_FRAME", (unsigned long)(CVI_VC_VENC_STOP_RECV_FRAME));
#else
	missing_line("CVI_VC_VENC_STOP_RECV_FRAME");
#endif
#ifdef CVI_DWA_ADD_LDC_TASK
	ioctl_line("CVI_DWA_ADD_LDC_TASK", (unsigned long)(CVI_DWA_ADD_LDC_TASK));
#else
	missing_line("CVI_DWA_ADD_LDC_TASK");
#endif
#ifdef CVI_DWA_ADD_ROT_TASK
	ioctl_line("CVI_DWA_ADD_ROT_TASK", (unsigned long)(CVI_DWA_ADD_ROT_TASK));
#else
	missing_line("CVI_DWA_ADD_ROT_TASK");
#endif
#ifdef CVI_DWA_BEGIN_JOB
	ioctl_line("CVI_DWA_BEGIN_JOB", (unsigned long)(CVI_DWA_BEGIN_JOB));
#else
	missing_line("CVI_DWA_BEGIN_JOB");
#endif
#ifdef CVI_DWA_CANCEL_JOB
	ioctl_line("CVI_DWA_CANCEL_JOB", (unsigned long)(CVI_DWA_CANCEL_JOB));
#else
	missing_line("CVI_DWA_CANCEL_JOB");
#endif
#ifdef CVI_DWA_END_JOB
	ioctl_line("CVI_DWA_END_JOB", (unsigned long)(CVI_DWA_END_JOB));
#else
	missing_line("CVI_DWA_END_JOB");
#endif
#ifdef CVI_DWA_GET_BUF_WRAP
	ioctl_line("CVI_DWA_GET_BUF_WRAP", (unsigned long)(CVI_DWA_GET_BUF_WRAP));
#else
	missing_line("CVI_DWA_GET_BUF_WRAP");
#endif
#ifdef CVI_DWA_SET_BUF_WRAP
	ioctl_line("CVI_DWA_SET_BUF_WRAP", (unsigned long)(CVI_DWA_SET_BUF_WRAP));
#else
	missing_line("CVI_DWA_SET_BUF_WRAP");
#endif
#ifdef CVI_VPSS_ATTACH_VB_POOL
	ioctl_line("CVI_VPSS_ATTACH_VB_POOL", (unsigned long)(CVI_VPSS_ATTACH_VB_POOL));
#else
	missing_line("CVI_VPSS_ATTACH_VB_POOL");
#endif
#ifdef CVI_VPSS_CREATE_GROUP
	ioctl_line("CVI_VPSS_CREATE_GROUP", (unsigned long)(CVI_VPSS_CREATE_GROUP));
#else
	missing_line("CVI_VPSS_CREATE_GROUP");
#endif
#ifdef CVI_VPSS_DESTROY_GROUP
	ioctl_line("CVI_VPSS_DESTROY_GROUP", (unsigned long)(CVI_VPSS_DESTROY_GROUP));
#else
	missing_line("CVI_VPSS_DESTROY_GROUP");
#endif
#ifdef CVI_VPSS_DETACH_VB_POOL
	ioctl_line("CVI_VPSS_DETACH_VB_POOL", (unsigned long)(CVI_VPSS_DETACH_VB_POOL));
#else
	missing_line("CVI_VPSS_DETACH_VB_POOL");
#endif
#ifdef CVI_VPSS_DISABLE_CHN
	ioctl_line("CVI_VPSS_DISABLE_CHN", (unsigned long)(CVI_VPSS_DISABLE_CHN));
#else
	missing_line("CVI_VPSS_DISABLE_CHN");
#endif
#ifdef CVI_VPSS_ENABLE_CHN
	ioctl_line("CVI_VPSS_ENABLE_CHN", (unsigned long)(CVI_VPSS_ENABLE_CHN));
#else
	missing_line("CVI_VPSS_ENABLE_CHN");
#endif
#ifdef CVI_VPSS_GET_ALL_AMP
	ioctl_line("CVI_VPSS_GET_ALL_AMP", (unsigned long)(CVI_VPSS_GET_ALL_AMP));
#else
	missing_line("CVI_VPSS_GET_ALL_AMP");
#endif
#ifdef CVI_VPSS_GET_AMP_CFG
	ioctl_line("CVI_VPSS_GET_AMP_CFG", (unsigned long)(CVI_VPSS_GET_AMP_CFG));
#else
	missing_line("CVI_VPSS_GET_AMP_CFG");
#endif
#ifdef CVI_VPSS_GET_AMP_CTRL
	ioctl_line("CVI_VPSS_GET_AMP_CTRL", (unsigned long)(CVI_VPSS_GET_AMP_CTRL));
#else
	missing_line("CVI_VPSS_GET_AMP_CTRL");
#endif
#ifdef CVI_VPSS_GET_AVAIL_GROUP
	ioctl_line("CVI_VPSS_GET_AVAIL_GROUP", (unsigned long)(CVI_VPSS_GET_AVAIL_GROUP));
#else
	missing_line("CVI_VPSS_GET_AVAIL_GROUP");
#endif
#ifdef CVI_VPSS_GET_CHN_ALIGN
	ioctl_line("CVI_VPSS_GET_CHN_ALIGN", (unsigned long)(CVI_VPSS_GET_CHN_ALIGN));
#else
	missing_line("CVI_VPSS_GET_CHN_ALIGN");
#endif
#ifdef CVI_VPSS_GET_CHN_ATTR
	ioctl_line("CVI_VPSS_GET_CHN_ATTR", (unsigned long)(CVI_VPSS_GET_CHN_ATTR));
#else
	missing_line("CVI_VPSS_GET_CHN_ATTR");
#endif
#ifdef CVI_VPSS_GET_CHN_BUF_WRAP
	ioctl_line("CVI_VPSS_GET_CHN_BUF_WRAP", (unsigned long)(CVI_VPSS_GET_CHN_BUF_WRAP));
#else
	missing_line("CVI_VPSS_GET_CHN_BUF_WRAP");
#endif
#ifdef CVI_VPSS_GET_CHN_CROP
	ioctl_line("CVI_VPSS_GET_CHN_CROP", (unsigned long)(CVI_VPSS_GET_CHN_CROP));
#else
	missing_line("CVI_VPSS_GET_CHN_CROP");
#endif
#ifdef CVI_VPSS_GET_CHN_FRAME
	ioctl_line("CVI_VPSS_GET_CHN_FRAME", (unsigned long)(CVI_VPSS_GET_CHN_FRAME));
#else
	missing_line("CVI_VPSS_GET_CHN_FRAME");
#endif
#ifdef CVI_VPSS_GET_CHN_LDC
	ioctl_line("CVI_VPSS_GET_CHN_LDC", (unsigned long)(CVI_VPSS_GET_CHN_LDC));
#else
	missing_line("CVI_VPSS_GET_CHN_LDC");
#endif
#ifdef CVI_VPSS_GET_CHN_ROTATION
	ioctl_line("CVI_VPSS_GET_CHN_ROTATION", (unsigned long)(CVI_VPSS_GET_CHN_ROTATION));
#else
	missing_line("CVI_VPSS_GET_CHN_ROTATION");
#endif
#ifdef CVI_VPSS_GET_CHN_SCALE_COEFF_LEVEL
	ioctl_line("CVI_VPSS_GET_CHN_SCALE_COEFF_LEVEL", (unsigned long)(CVI_VPSS_GET_CHN_SCALE_COEFF_LEVEL));
#else
	missing_line("CVI_VPSS_GET_CHN_SCALE_COEFF_LEVEL");
#endif
#ifdef CVI_VPSS_GET_CHN_YRATIO
	ioctl_line("CVI_VPSS_GET_CHN_YRATIO", (unsigned long)(CVI_VPSS_GET_CHN_YRATIO));
#else
	missing_line("CVI_VPSS_GET_CHN_YRATIO");
#endif
#ifdef CVI_VPSS_GET_GRP_ATTR
	ioctl_line("CVI_VPSS_GET_GRP_ATTR", (unsigned long)(CVI_VPSS_GET_GRP_ATTR));
#else
	missing_line("CVI_VPSS_GET_GRP_ATTR");
#endif
#ifdef CVI_VPSS_GET_GRP_CROP
	ioctl_line("CVI_VPSS_GET_GRP_CROP", (unsigned long)(CVI_VPSS_GET_GRP_CROP));
#else
	missing_line("CVI_VPSS_GET_GRP_CROP");
#endif
#ifdef CVI_VPSS_GET_SCENE
	ioctl_line("CVI_VPSS_GET_SCENE", (unsigned long)(CVI_VPSS_GET_SCENE));
#else
	missing_line("CVI_VPSS_GET_SCENE");
#endif
#ifdef CVI_VPSS_HIDE_CHN
	ioctl_line("CVI_VPSS_HIDE_CHN", (unsigned long)(CVI_VPSS_HIDE_CHN));
#else
	missing_line("CVI_VPSS_HIDE_CHN");
#endif
#ifdef CVI_VPSS_RELEASE_CHN_FRAME
	ioctl_line("CVI_VPSS_RELEASE_CHN_FRAME", (unsigned long)(CVI_VPSS_RELEASE_CHN_FRAME));
#else
	missing_line("CVI_VPSS_RELEASE_CHN_FRAME");
#endif
#ifdef CVI_VPSS_RESET_GROUP
	ioctl_line("CVI_VPSS_RESET_GROUP", (unsigned long)(CVI_VPSS_RESET_GROUP));
#else
	missing_line("CVI_VPSS_RESET_GROUP");
#endif
#ifdef CVI_VPSS_SEND_CHN_FRAME
	ioctl_line("CVI_VPSS_SEND_CHN_FRAME", (unsigned long)(CVI_VPSS_SEND_CHN_FRAME));
#else
	missing_line("CVI_VPSS_SEND_CHN_FRAME");
#endif
#ifdef CVI_VPSS_SEND_FRAME
	ioctl_line("CVI_VPSS_SEND_FRAME", (unsigned long)(CVI_VPSS_SEND_FRAME));
#else
	missing_line("CVI_VPSS_SEND_FRAME");
#endif
#ifdef CVI_VPSS_SET_CHN_ALIGN
	ioctl_line("CVI_VPSS_SET_CHN_ALIGN", (unsigned long)(CVI_VPSS_SET_CHN_ALIGN));
#else
	missing_line("CVI_VPSS_SET_CHN_ALIGN");
#endif
#ifdef CVI_VPSS_SET_CHN_ATTR
	ioctl_line("CVI_VPSS_SET_CHN_ATTR", (unsigned long)(CVI_VPSS_SET_CHN_ATTR));
#else
	missing_line("CVI_VPSS_SET_CHN_ATTR");
#endif
#ifdef CVI_VPSS_SET_CHN_BUF_WRAP
	ioctl_line("CVI_VPSS_SET_CHN_BUF_WRAP", (unsigned long)(CVI_VPSS_SET_CHN_BUF_WRAP));
#else
	missing_line("CVI_VPSS_SET_CHN_BUF_WRAP");
#endif
#ifdef CVI_VPSS_SET_CHN_CROP
	ioctl_line("CVI_VPSS_SET_CHN_CROP", (unsigned long)(CVI_VPSS_SET_CHN_CROP));
#else
	missing_line("CVI_VPSS_SET_CHN_CROP");
#endif
#ifdef CVI_VPSS_SET_CHN_LDC
	ioctl_line("CVI_VPSS_SET_CHN_LDC", (unsigned long)(CVI_VPSS_SET_CHN_LDC));
#else
	missing_line("CVI_VPSS_SET_CHN_LDC");
#endif
#ifdef CVI_VPSS_SET_CHN_ROTATION
	ioctl_line("CVI_VPSS_SET_CHN_ROTATION", (unsigned long)(CVI_VPSS_SET_CHN_ROTATION));
#else
	missing_line("CVI_VPSS_SET_CHN_ROTATION");
#endif
#ifdef CVI_VPSS_SET_CHN_SCALE_COEFF_LEVEL
	ioctl_line("CVI_VPSS_SET_CHN_SCALE_COEFF_LEVEL", (unsigned long)(CVI_VPSS_SET_CHN_SCALE_COEFF_LEVEL));
#else
	missing_line("CVI_VPSS_SET_CHN_SCALE_COEFF_LEVEL");
#endif
#ifdef CVI_VPSS_SET_CHN_YRATIO
	ioctl_line("CVI_VPSS_SET_CHN_YRATIO", (unsigned long)(CVI_VPSS_SET_CHN_YRATIO));
#else
	missing_line("CVI_VPSS_SET_CHN_YRATIO");
#endif
#ifdef CVI_VPSS_SET_GRP_ATTR
	ioctl_line("CVI_VPSS_SET_GRP_ATTR", (unsigned long)(CVI_VPSS_SET_GRP_ATTR));
#else
	missing_line("CVI_VPSS_SET_GRP_ATTR");
#endif
#ifdef CVI_VPSS_SET_GRP_CROP
	ioctl_line("CVI_VPSS_SET_GRP_CROP", (unsigned long)(CVI_VPSS_SET_GRP_CROP));
#else
	missing_line("CVI_VPSS_SET_GRP_CROP");
#endif
#ifdef CVI_VPSS_SET_GRP_CSC_CFG
	ioctl_line("CVI_VPSS_SET_GRP_CSC_CFG", (unsigned long)(CVI_VPSS_SET_GRP_CSC_CFG));
#else
	missing_line("CVI_VPSS_SET_GRP_CSC_CFG");
#endif
#ifdef CVI_VPSS_SHOW_CHN
	ioctl_line("CVI_VPSS_SHOW_CHN", (unsigned long)(CVI_VPSS_SHOW_CHN));
#else
	missing_line("CVI_VPSS_SHOW_CHN");
#endif
#ifdef CVI_VPSS_START_GROUP
	ioctl_line("CVI_VPSS_START_GROUP", (unsigned long)(CVI_VPSS_START_GROUP));
#else
	missing_line("CVI_VPSS_START_GROUP");
#endif
#ifdef CVI_VPSS_STOP_GROUP
	ioctl_line("CVI_VPSS_STOP_GROUP", (unsigned long)(CVI_VPSS_STOP_GROUP));
#else
	missing_line("CVI_VPSS_STOP_GROUP");
#endif
#ifdef CVI_VPSS_TRIGGER_SNAP_FRAME
	ioctl_line("CVI_VPSS_TRIGGER_SNAP_FRAME", (unsigned long)(CVI_VPSS_TRIGGER_SNAP_FRAME));
#else
	missing_line("CVI_VPSS_TRIGGER_SNAP_FRAME");
#endif
