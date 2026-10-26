# maixlite (not upstream): libkvm.so, NanoKVM's MaixCDK component support/sg2002/additional/kvm.
# MaixCDK's ADD_REQUIREMENTS: vision basic peripheral, then atomic; ADD_LINK_DEFINITIONS_PRIVATE
# -Wl,--as-needed.
add_library(kvm SHARED
    ${KVM_DIR}/src/kvm_vision.cpp
    ${KVM_DIR}/src/lt6911.c
    ${KVM_DIR}/src/vi_state_shared.cpp)
target_include_directories(kvm PRIVATE
    "${KVM_DIR}/include" "${CFG_DIR}" "${VISION_DIR}/include" "${VISION_DIR}/include/base"
    ${INC_BASIC_DEPS} "${INC_PERIPH}" "${KVM_MMF_DIR}/include" "${KVM_MMF_DIR}" ${INC_MW})
target_link_options(kvm PRIVATE -Wl,--as-needed)
target_link_libraries(kvm PUBLIC vision basic peripheral atomic)
