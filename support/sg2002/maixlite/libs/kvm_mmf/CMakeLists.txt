# maixlite (not upstream): libkvm_mmf.so, NanoKVM's MaixCDK component
# support/sg2002/additional/kvm_mmf as it was built with the pinned middleware
# (NANOKVM_MIDDLEWARE_SRC, linked against NANOKVM_MMF_LIB, the uapi dirs of NANOKVM_MMF_UAPI).
# Source order is MaixCDK's: src/, then the pin's sample/common and the lt6911 sensor
# directory (alphabetical), with the pin's sample_common_sensor.c replaced by ours, last.
set(_c "${MW_SRC}/v2/sample/common")
set(_s "${MW_SRC}/v2/component/isp/sensor/sg200x/lontium_lt6911")
add_library(kvm_mmf SHARED
    ${KVM_MMF_DIR}/src/kvm_mmf.cpp
    ${_c}/loadbmp.c
    ${_c}/md5sum.c
    ${_c}/sample_common_bin.c
    ${_c}/sample_common_isp.c
    ${_c}/sample_common_peripheral.c
    ${_c}/sample_common_platform.c
    ${_c}/sample_common_region.c
    ${_c}/sample_common_sys.c
    ${_c}/sample_common_vdec.c
    ${_c}/sample_common_venc.c
    ${_c}/sample_common_vi.c
    ${_c}/sample_common_vo.c
    ${_c}/sample_common_vpss.c
    ${_s}/lt6911_cmos.c
    ${_s}/lt6911_sensor_ctl.c
    ${KVM_MMF_DIR}/pin/sample_common_sensor.c)
target_include_directories(kvm_mmf PRIVATE
    "${KVM_MMF_DIR}/include" "${KVM_MMF_DIR}" ${INC_MW} "${CFG_DIR}" ${INC_BASIC_DEPS})
# MaixCDK's ADD_DEFINITIONS_PRIVATE: a compile option and a link option
target_compile_options(kvm_mmf PRIVATE -DSENSOR_LONTIUM_LT6911 -DSENSOR2_TYPE=LONTIUM_LT6911_2M_60FPS_8BIT)
target_link_options(kvm_mmf PRIVATE -DSENSOR_LONTIUM_LT6911 -DSENSOR2_TYPE=LONTIUM_LT6911_2M_60FPS_8BIT)
# MaixCDK's ADD_DYNAMIC_LIB: -L<dir> <file> for each of the 12 libraries, then ADD_REQUIREMENTS
# basic ini.
set(_l "-L${MMF_LIB}")
target_link_libraries(kvm_mmf PUBLIC
    ${_l} libcvi_bin.so ${_l} libae.so ${_l} libaf.so ${_l} libawb.so ${_l} libcvi_bin_isp.so
    ${_l} libisp_algo.so ${_l} libisp.so ${_l} libsys.so ${_l} libvdec.so ${_l} libvenc.so
    ${_l} libvpu.so -L${MMF_LIB}/3rd libini.so
    basic ini)
