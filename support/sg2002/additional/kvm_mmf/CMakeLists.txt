
list(APPEND ADD_REQUIREMENTS basic ini)
list(APPEND ADD_INCLUDE "include")

# sophgo-middleware
set(middleware_src_path "${CMAKE_CURRENT_SOURCE_DIR}/../3rd_party/sophgo-middleware/sophgo-middleware")

# Two modes. Unset NANOKVM_MIDDLEWARE_SRC = MaixCDK's copy and its v2/lib prebuilts (the
# old way, kept only so the tree still configures without the forge inputs).
#
# NANOKVM_MIDDLEWARE_SRC set = the pinned scpcom/sophgo-middleware tree
# (scripts/fetch-middleware.sh <dir>; the tree is under <dir>/v2), with:
#   NANOKVM_MMF_LIB    the directory scripts/build-mmf.sh staged: the libraries built from
#                      that same tree plus the closed ISP/3A blobs of that pin. Nothing is
#                      linked from MaixCDK's v2/lib.
#   NANOKVM_MMF_UAPI   ";"-separated kernel uapi include dirs: the pin ships none; they come
#                      from the osdrv v2 tree the 7.3 kernel is built from.

# Headers: v2/include, modules/isp/include/<chip> (the pin keeps the ISP headers there,
# not under v2/include/isp), v2/sample/common, inih, ive, the panel dir, the uapi dirs.
set(nanokvm_pinned FALSE)
if(DEFINED ENV{NANOKVM_MIDDLEWARE_SRC})
    set(nanokvm_pinned TRUE)
    set(middleware_src_path "$ENV{NANOKVM_MIDDLEWARE_SRC}")
    foreach(req NANOKVM_MMF_LIB NANOKVM_MMF_UAPI)
        if(NOT DEFINED ENV{${req}} OR "$ENV{${req}}" STREQUAL "")
            message(FATAL_ERROR "NANOKVM_MIDDLEWARE_SRC is set but ${req} is not")
        endif()
    endforeach()
    foreach(req v2/include v2/modules/isp/include/${CONFIG_SOPHGO_MIDDLEWARE_CHIP} v2/sample/common)
        if(NOT EXISTS "${middleware_src_path}/${req}")
            message(FATAL_ERROR "NANOKVM_MIDDLEWARE_SRC=${middleware_src_path} has no ${req}")
        endif()
    endforeach()
    set(mmf_lib_dir "$ENV{NANOKVM_MMF_LIB}")
    set(mmf_uapi_dirs $ENV{NANOKVM_MMF_UAPI})
    set(mmf_isp_include ${middleware_src_path}/v2/modules/isp/include/${CONFIG_SOPHGO_MIDDLEWARE_CHIP})
else()
    set(mmf_lib_dir ${middleware_src_path}/v2/lib)
    set(mmf_uapi_dirs ${middleware_src_path}/v2/uapi)
    set(mmf_isp_include ${middleware_src_path}/v2/include/isp/${CONFIG_SOPHGO_MIDDLEWARE_CHIP})
endif()


set(source_dir "src")
append_srcs_dir(ADD_SRCS "${source_dir}")

# middleware
set(middleware_include_dir .
    ${middleware_src_path}/v2/component/panel/${CONFIG_SOPHGO_MIDDLEWARE_CHIP}
    ${middleware_src_path}/v2/include
    ${mmf_isp_include}
    ${middleware_src_path}/v2/sample/common
    ${mmf_uapi_dirs}
    ${middleware_src_path}/v2/3rdparty/inih
    ${middleware_src_path}/v2/modules/ive/include/
)
list(APPEND ADD_INCLUDE ${middleware_include_dir})
set_property(SOURCE ${middleware_include_dir} PROPERTY GENERATED 1)

if(nanokvm_pinned)
    # The pin's sample/common, except sample_common_sensor.c: that one is the pin's file plus
    # the two NanoKVM changes (pin/sample_common_sensor.c, diff it against the pin).
    append_srcs_dir(middleware_src_dir  ${middleware_src_path}/v2/sample/common
                                        ${middleware_src_path}/v2/component/isp/sensor/sg200x/lontium_lt6911
                                        )
    list(FILTER middleware_src_dir EXCLUDE REGEX "/sample_common_sensor\\.c$")
    list(APPEND middleware_src_dir "${CMAKE_CURRENT_SOURCE_DIR}/pin/sample_common_sensor.c")
else()
    append_srcs_dir(middleware_src_dir  ${middleware_src_path}/v2/sample/common
                                        ${middleware_src_path}/v2/component/isp/sensor/sg200x/lontium_lt6911
                                        )
endif()
list(APPEND ADD_SRCS        ${middleware_src_dir}
                            "${source_dir}/kvm_mmf.cpp")
set_property(SOURCE ${middleware_src_dir} PROPERTY GENERATED 1)

set(middleware_static_lib_file  "")
    list(APPEND ADD_STATIC_LIB ${middleware_static_lib_file})
set_property(SOURCE ${middleware_static_lib_file} PROPERTY GENERATED 1)

if(CONFIG_SOPHGO_MIDDLEWARE_C_LIBRARY STREQUAL "musl")
    set(middleware_dynamic_lib_file     ${mmf_lib_dir}/libcvi_bin.so
                                        ${mmf_lib_dir}/libae.so
                                        ${mmf_lib_dir}/libaf.so
                                        ${mmf_lib_dir}/libawb.so
                                        ${mmf_lib_dir}/libcvi_bin_isp.so
                                        ${mmf_lib_dir}/libisp_algo.so
                                        ${mmf_lib_dir}/libisp.so
                                        ${mmf_lib_dir}/libsys.so
                                        ${mmf_lib_dir}/libvdec.so
                                        ${mmf_lib_dir}/libvenc.so
                                        ${mmf_lib_dir}/libvpu.so
                                        ${mmf_lib_dir}/3rd/libini.so)
    if(nanokvm_pinned)
        foreach(lib ${middleware_dynamic_lib_file})
            if(NOT EXISTS "${lib}")
                message(FATAL_ERROR "NANOKVM_MMF_LIB lacks ${lib}")
            endif()
        endforeach()
    endif()
elseif(CONFIG_SOPHGO_MIDDLEWARE_C_LIBRARY STREQUAL "glibc")
    set(middleware_dynamic_lib_file     ${mmf_lib_dir}/libcvi_vb.so
                                        ${mmf_lib_dir}/libcvi_sys.so
                                        ${mmf_lib_dir}/libcvi_debug.so
    )
else()
    message(FATAL_ERROR "No sophgo middleware c library support, please check menuoconfig to select c library")
endif()
list(APPEND ADD_DYNAMIC_LIB ${middleware_dynamic_lib_file})
set_property(SOURCE ${middleware_dynamic_lib_file} PROPERTY GENERATED 1)

list(APPEND ADD_DEFINITIONS_PRIVATE -DSENSOR_LONTIUM_LT6911
                                -DSENSOR2_TYPE=LONTIUM_LT6911_2M_60FPS_8BIT)


register_component(DYNAMIC)
