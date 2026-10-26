/* structs.inc - hand-kept: structs/fields the KVM video path hands to the kernel.
 * included inside main() of probe.c */

/* sys: ION alloc/free, cache ops, bind */
S(struct sys_ion_data);
F(struct sys_ion_data, size);
F(struct sys_ion_data, cached);
F(struct sys_ion_data, dmabuf_fd);
F(struct sys_ion_data, addr_p);
F(struct sys_ion_data, name);
S(struct sys_cache_op);
F(struct sys_cache_op, addr_v);
F(struct sys_cache_op, addr_p);
F(struct sys_cache_op, size);
F(struct sys_cache_op, dma_fd);
S(struct sys_ext_control);
F(struct sys_ext_control, id);
F(struct sys_ext_control, reserved);
F(struct sys_ext_control, value);
F(struct sys_ext_control, value64);
F(struct sys_ext_control, ptr);
S(struct sys_bind_cfg);
F(struct sys_bind_cfg, is_bind);
F(struct sys_bind_cfg, get_by_src);
F(struct sys_bind_cfg, mmf_chn_src);
F(struct sys_bind_cfg, mmf_chn_dst);
F(struct sys_bind_cfg, bind_dst);

/* vb */
S(struct cvi_vb_pool_cfg);
F(struct cvi_vb_pool_cfg, blk_size);
F(struct cvi_vb_pool_cfg, blk_cnt);
F(struct cvi_vb_pool_cfg, remap_mode);
F(struct cvi_vb_pool_cfg, pool_name);
F(struct cvi_vb_pool_cfg, pool_id);
F(struct cvi_vb_pool_cfg, mem_base);
S(struct cvi_vb_pool_ex_cfg);
F(struct cvi_vb_pool_ex_cfg, blk_cnt);
F(struct cvi_vb_pool_ex_cfg, au64PhyAddr);
F(struct cvi_vb_pool_ex_cfg, pool_id);
S(struct cvi_vb_cfg);
F(struct cvi_vb_cfg, comm_pool_cnt);
F(struct cvi_vb_cfg, comm_pool);
S(struct cvi_vb_blk_cfg);
F(struct cvi_vb_blk_cfg, pool_id);
F(struct cvi_vb_blk_cfg, blk_size);
F(struct cvi_vb_blk_cfg, blk);
S(struct cvi_vb_blk_info);
F(struct cvi_vb_blk_info, blk);
F(struct cvi_vb_blk_info, pool_id);
F(struct cvi_vb_blk_info, phy_addr);
F(struct cvi_vb_blk_info, usr_cnt);

/* frame / buffer types that cross every video ioctl */
S(VIDEO_FRAME_S);
F(VIDEO_FRAME_S, u32Width);
F(VIDEO_FRAME_S, u32Height);
F(VIDEO_FRAME_S, enPixelFormat);
F(VIDEO_FRAME_S, u64PhyAddr);
F(VIDEO_FRAME_S, u32Stride);
F(VIDEO_FRAME_S, u32Length);
F(VIDEO_FRAME_S, pu8VirAddr);
S(VIDEO_FRAME_INFO_S);
F(VIDEO_FRAME_INFO_S, stVFrame);
F(VIDEO_FRAME_INFO_S, u32PoolId);
S(MMF_CHN_S);
S(MMF_BIND_DEST_S);

/* vpss */
S(VPSS_GRP_ATTR_S);
S(VPSS_CHN_ATTR_S);
S(VPSS_CROP_INFO_S);
F(VPSS_CROP_INFO_S, bEnable);
F(VPSS_CROP_INFO_S, enCropCoordinate);
F(VPSS_CROP_INFO_S, stCropRect);

/* venc / vdec attribute blocks passed whole to the vc driver */
S(VENC_CHN_ATTR_S);
S(VENC_RECV_PIC_PARAM_S);
S(VENC_CHN_STATUS_S);
S(VENC_STREAM_S);
S(VENC_PACK_S);
S(VENC_RC_PARAM_S);
S(VENC_JPEG_PARAM_S);
S(VENC_FRAMELOST_S);
S(VENC_CHN_PARAM_S);
S(VENC_PARAM_MOD_S);
S(VDEC_CHN_ATTR_S);
S(VDEC_STREAM_S);
S(VDEC_CHN_STATUS_S);
S(VDEC_CHN_PARAM_S);
S(VDEC_MOD_PARAM_S);
