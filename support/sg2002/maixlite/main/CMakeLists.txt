# maixlite (not upstream): kvm_system's own sources (support/sg2002/kvm_system/main), in the
# order MaixCDK's append_srcs_dir gave. The include list is kvm_system/main/CMakeLists.txt's
# ADD_INCLUDE, then ADD_PRIVATE_INCLUDE (additional/kvm/include), then the dependencies'.
set(_m "${KVM_SYSTEM_MAIN}")
add_library(main STATIC
    ${_m}/src/main.cpp
    ${_m}/lib/libqr/qr.c
    ${_m}/lib/libqr/qrcmd.c
    ${_m}/lib/libqr/qrcnv.c
    ${_m}/lib/libqr/qrcnv_bmp.c
    ${_m}/lib/system_ctrl/system_ctrl.cpp
    ${_m}/lib/system_state/system_state.cpp
    ${_m}/lib/system_init/system_init.cpp
    ${_m}/lib/oled_ctrl/oled_ctrl.cpp
    ${_m}/lib/oled_ui/oled_ui.cpp)
target_include_directories(main PRIVATE
    ${INC_MAIN} "${KVM_INCLUDE}" "${CFG_DIR}" "${INC_BASIC}" "${INC_INI}" "${INC_YAML_SRC}"
    "${INC_YAML_INC}" "${INC_NTP}" "${INC_PERIPH}")
