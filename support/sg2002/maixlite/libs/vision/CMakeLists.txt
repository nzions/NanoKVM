# maixlite (not upstream): NanoKVM's cut of MaixCDK's vision component
# (support/sg2002/additional/vision), static, for platform maixcam. The sources stay there.
# MaixCDK's ADD_REQUIREMENTS: basic peripheral kvm_mmf.
add_library(vision STATIC
    ${VISION_DIR}/src/maix_camera.cpp
    ${VISION_DIR}/src/maix_image.cpp)
target_include_directories(vision PRIVATE
    "${VISION_DIR}/include" "${VISION_DIR}/include/base" "${VISION_DIR}/port/maixcam"
    "${CFG_DIR}" ${INC_BASIC_DEPS} "${INC_PERIPH}"
    "${KVM_MMF_DIR}/include" "${KVM_MMF_DIR}" ${INC_MW})
target_link_libraries(vision PUBLIC basic peripheral kvm_mmf)
