# NanoKVM's replacement for MaixCDK's components/3rd_party/opencv/CMakeLists.txt
# (support/sg2002/build sync_components copies it over the stock file).
#
# The stock component compiles every dependent against MaixCDK's PREBUILT OpenCV
# headers and links MaixCDK's prebuilt .so files. The package ships OpenCV 4.9.0
# built by scripts/build-deps.sh from pinned source (same soname, other build
# configuration), so libkvm.so must be compiled and linked against that build:
# the headers and libraries below are its output, nothing is taken from MaixCDK.
#
# NANOKVM_OPENCV_DIR is build-deps.sh's <outdir> (include/opencv4 and lib/). It is
# REQUIRED: unset, or missing a header or library, is a configure error. There is
# no fallback to the MaixCDK prebuilt.
if(NOT PLATFORM_MAIXCAM)
    message(FATAL_ERROR "the NanoKVM opencv component supports PLATFORM_MAIXCAM only")
endif()
if(NOT DEFINED ENV{NANOKVM_OPENCV_DIR} OR "$ENV{NANOKVM_OPENCV_DIR}" STREQUAL "")
    message(FATAL_ERROR "NANOKVM_OPENCV_DIR is unset: libkvm must build against the OpenCV of scripts/build-deps.sh, not MaixCDK's prebuilt")
endif()
set(nanokvm_opencv_dir "$ENV{NANOKVM_OPENCV_DIR}")
set(nanokvm_opencv_inc "${nanokvm_opencv_dir}/include/opencv4")
if(NOT EXISTS "${nanokvm_opencv_inc}/opencv2/opencv.hpp")
    message(FATAL_ERROR "NANOKVM_OPENCV_DIR=${nanokvm_opencv_dir} has no include/opencv4/opencv2/opencv.hpp")
endif()

set(opencv_libs "${nanokvm_opencv_dir}/lib/libopencv_core.so.409"
                "${nanokvm_opencv_dir}/lib/libopencv_imgproc.so.409"
                "${nanokvm_opencv_dir}/lib/libopencv_imgcodecs.so.409")
foreach(lib ${opencv_libs})
    if(NOT EXISTS "${lib}")
        message(FATAL_ERROR "NANOKVM_OPENCV_DIR lacks ${lib}")
    endif()
    set_property(SOURCE ${lib} PROPERTY GENERATED 1)
endforeach()
message(STATUS "NanoKVM OpenCV: headers ${nanokvm_opencv_inc}, libs ${opencv_libs}")

list(APPEND ADD_INCLUDE "${nanokvm_opencv_inc}")
list(APPEND ADD_DYNAMIC_LIB ${opencv_libs})
# the package ships these three itself (scripts/build-in-container.sh stages them)
list(APPEND ADD_DIST_LIB_IGNORE ${opencv_libs})
list(APPEND ADD_REQUIREMENTS pthread dl)
list(APPEND ADD_DEFINITIONS -fopenmp)

register_component()
