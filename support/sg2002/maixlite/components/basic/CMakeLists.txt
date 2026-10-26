# maixlite (not upstream): MaixCDK components/basic for platform maixcam. Source order is
# the order of MaixCDK's archive (append_srcs_dir "src", then "port/maixcam").
add_library(basic STATIC
    src/maix_app.cpp
    src/maix_err.cpp
    src/maix_fs.cpp
    src/maix_i18n.cpp
    src/maix_log.cpp
    src/maix_protocol.cpp
    src/maix_sys.cpp
    src/maix_time.cpp
    src/maix_util.cpp
    port/maixcam/maix_sys_maixcam.cpp
    port/maixcam/maix_thread_maixcam.cpp
    port/maixcam/maix_util_maixcam.cpp)
target_include_directories(basic PRIVATE
    "${INC_BASIC}" "${CFG_DIR}" "${INC_INI}" "${INC_YAML_SRC}" "${INC_YAML_INC}" "${INC_NTP}")
