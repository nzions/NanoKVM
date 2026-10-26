# maixlite: the NanoKVM riscv64 programs and libraries without MaixCDK.
#
# The same sources, defines, include order, flags and link line MaixCDK's CMake produced
# (commit 30f4b8b7e3f6, platform maixcam, MinSizeRel), as plain CMake. Two projects, each in
# its own build tree, chosen by -DMAIXLITE_PROJECT=:
#   kvm_system       (default) support/sg2002/kvm_system, the OLED and button daemon
#   kvm_vision_test  libkvm_mmf.so and libkvm.so (support/sg2002/additional/{kvm_mmf,kvm,
#                    vision}); needs the pinned middleware: NANOKVM_MIDDLEWARE_SRC,
#                    NANOKVM_MMF_LIB and NANOKVM_MMF_UAPI in the environment, as for
#                    scripts/forge-build.sh. The name is MaixCDK's carrier project's.
# tools/binabi holds the results to the MaixCDK-built binaries and tools/binabi/flags.sh
# holds the generated flag files to the MaixCDK ones.
#
#   MAIXLITE_TOOLCHAIN_BIN=<toolchain bin> cmake -G "Unix Makefiles" \
#       -DCMAKE_TOOLCHAIN_FILE=toolchain.cmake -DCMAKE_BUILD_TYPE=MinSizeRel <this dir>
#
# or scripts/build-maixlite.sh, which also strips and stages the binaries.

cmake_minimum_required(VERSION 3.17)

if(NOT CMAKE_BUILD_TYPE STREQUAL "MinSizeRel")
    message(FATAL_ERROR "CMAKE_BUILD_TYPE must be MinSizeRel (MaixCDK's release build): '${CMAKE_BUILD_TYPE}'")
endif()

if(NOT DEFINED MAIXLITE_PROJECT)
    set(MAIXLITE_PROJECT kvm_system)
endif()
if(NOT MAIXLITE_PROJECT STREQUAL "kvm_system" AND NOT MAIXLITE_PROJECT STREQUAL "kvm_vision_test")
    message(FATAL_ERROR "MAIXLITE_PROJECT must be kvm_system or kvm_vision_test: '${MAIXLITE_PROJECT}'")
endif()

project(${MAIXLITE_PROJECT} LANGUAGES C CXX)

set(CMAKE_EXPORT_COMPILE_COMMANDS ON)

# --- flags: MaixCDK's platforms/maixcam.yaml c_flags/cxx_flags, then its compile_flags.cmake
set(_arch "-mcpu=c906fdv -march=rv64imafdcv0p7xthead -mcmodel=medany -mabi=lp64d")
set(CMAKE_C_FLAGS "${_arch} -Wall -fPIC")
set(CMAKE_CXX_FLAGS "${_arch} -Wall -fPIC -std=c++17")
if(MAIXLITE_PROJECT STREQUAL "kvm_system")
    # $ORIGIN is literal: the loader expands it.
    set(CMAKE_C_LINK_FLAGS "-Wl,-rpath=\$ORIGIN/dl_lib")
    set(CMAKE_CXX_LINK_FLAGS "-Wl,-rpath=\$ORIGIN/dl_lib")
else()
    # The libraries carry no RPATH. MaixCDK's build left the absolute path of its build
    # directory in libkvm.so's (CMake's build-tree rpath); the loader finds libkvm_mmf.so
    # beside libkvm.so (the server's own $ORIGIN/dl_lib).
    set(CMAKE_SKIP_RPATH ON)
endif()

# --- definitions: MaixCDK's compile.cmake (release build, platform maixcam)
add_definitions(-DRELEASE=1 -DDEBUG=0)
add_definitions(-DPLATFORM_MAIXCAM=1)

# --- where things are
set(MAIXLITE_DIR "${CMAKE_CURRENT_SOURCE_DIR}")
get_filename_component(SG2002_DIR "${MAIXLITE_DIR}/.." ABSOLUTE)
set(KVM_SYSTEM_MAIN "${SG2002_DIR}/kvm_system/main")
set(KVM_INCLUDE "${SG2002_DIR}/additional/kvm/include")
set(KVM_I2C "${SG2002_DIR}/additional/peripheral/port/maixcam/maix_i2c.cpp")
set(COMP "${MAIXLITE_DIR}/components")

if(MAIXLITE_PROJECT STREQUAL "kvm_system")
    set(CFG_DIR "${MAIXLITE_DIR}/config")
else()
    # the same two stand-in headers, PROJECT_ID kvm_vision_test
    set(CFG_DIR "${MAIXLITE_DIR}/config-kvm_vision_test")
endif()
set(INC_BASIC "${COMP}/basic/include")
set(INC_INI "${COMP}/3rd_party/ini/inifile2/src")
set(INC_YAML_SRC "${COMP}/3rd_party/yaml/yaml-cpp/src")
set(INC_YAML_INC "${COMP}/3rd_party/yaml/yaml-cpp/include")
set(INC_NTP "${COMP}/3rd_party/ntp_client/ntp_client")
set(INC_PERIPH "${COMP}/peripheral/include")

# kvm_system's own include list (its main/CMakeLists.txt ADD_INCLUDE, in order)
set(INC_MAIN
    "${KVM_SYSTEM_MAIN}/include"
    "${KVM_SYSTEM_MAIN}/lib/libqr"
    "${KVM_SYSTEM_MAIN}/lib/system_ctrl"
    "${KVM_SYSTEM_MAIN}/lib/system_state"
    "${KVM_SYSTEM_MAIN}/lib/system_init"
    "${KVM_SYSTEM_MAIN}/lib/oled_ctrl"
    "${KVM_SYSTEM_MAIN}/lib/oled_ui")

# Order matters (the link line is held to the MaixCDK one): main, peripheral, basic, ini,
# yaml, ntp_client.
add_subdirectory(components/basic basic)
add_subdirectory(components/peripheral peripheral)
add_subdirectory(components/3rd_party/ini ini)
add_subdirectory(components/3rd_party/ntp_client ntp_client)
add_subdirectory(components/3rd_party/yaml yaml)

if(MAIXLITE_PROJECT STREQUAL "kvm_system")
    add_subdirectory(main main)

    # MaixCDK's add_executable takes an empty dummy exe_src.c; the program is the libraries.
    set(EXE_SRC "${CMAKE_BINARY_DIR}/exe_src.c")
    file(WRITE "${EXE_SRC}" "")
    add_executable(kvm_system "${EXE_SRC}")
    set_target_properties(kvm_system PROPERTIES LINKER_LANGUAGE CXX)
    target_include_directories(kvm_system PRIVATE
        ${INC_MAIN} "${CFG_DIR}" "${INC_BASIC}" "${INC_INI}" "${INC_YAML_SRC}" "${INC_YAML_INC}"
        "${INC_NTP}" "${INC_PERIPH}")
    target_link_libraries(kvm_system PRIVATE main peripheral basic pthread stdc++fs ini yaml ntp_client)
else()
    # --- the libraries: kvm_mmf (shared), vision (static), kvm (shared) ---------------------
    # The pinned middleware and what scripts/build-mmf.sh staged (libs/kvm_mmf/CMakeLists.txt).
    foreach(_v NANOKVM_MIDDLEWARE_SRC NANOKVM_MMF_LIB NANOKVM_MMF_UAPI)
        if(NOT DEFINED ENV{${_v}} OR "$ENV{${_v}}" STREQUAL "")
            message(FATAL_ERROR "${_v} is not set: libkvm_mmf must build against the pinned middleware (scripts/forge-build.sh)")
        endif()
    endforeach()
    set(MW_SRC "$ENV{NANOKVM_MIDDLEWARE_SRC}")
    set(MMF_LIB "$ENV{NANOKVM_MMF_LIB}")
    set(MMF_UAPI $ENV{NANOKVM_MMF_UAPI})
    foreach(_p "${MW_SRC}/v2/include" "${MW_SRC}/v2/modules/isp/include/cv181x" "${MW_SRC}/v2/sample/common"
               "${MMF_LIB}/libsys.so" "${MMF_LIB}/3rd/libini.so")
        if(NOT EXISTS "${_p}")
            message(FATAL_ERROR "${_p} does not exist")
        endif()
    endforeach()

    set(VISION_DIR "${SG2002_DIR}/additional/vision")
    set(KVM_DIR "${SG2002_DIR}/additional/kvm")
    set(KVM_MMF_DIR "${SG2002_DIR}/additional/kvm_mmf")
    # the dependencies' include directories, in the order their PUBLIC includes accumulated
    set(INC_BASIC_DEPS "${INC_BASIC}" "${INC_INI}" "${INC_YAML_SRC}" "${INC_YAML_INC}" "${INC_NTP}")
    # kvm_mmf's middleware include list, in order
    set(INC_MW
        "${MW_SRC}/v2/component/panel/cv181x"
        "${MW_SRC}/v2/include"
        "${MW_SRC}/v2/modules/isp/include/cv181x"
        "${MW_SRC}/v2/sample/common"
        ${MMF_UAPI}
        "${MW_SRC}/v2/3rdparty/inih"
        "${MW_SRC}/v2/modules/ive/include")

    # What MaixCDK's ADD_REQUIREMENTS gave each component (PUBLIC), which fixes the link order.
    target_link_libraries(basic PUBLIC pthread stdc++fs ini yaml ntp_client)
    target_link_libraries(peripheral PUBLIC basic)

    add_subdirectory(libs/vision vision)
    add_subdirectory(libs/kvm_mmf kvm_mmf)
    add_subdirectory(libs/kvm kvm)
endif()
