# maixlite: kvm_system without MaixCDK.
#
# The same sources, defines, include order, flags and link line MaixCDK's CMake produced
# for support/sg2002/kvm_system (commit 30f4b8b7e3f6, platform maixcam, MinSizeRel), as
# plain CMake. tools/binabi holds the result to the MaixCDK-built binary and
# tools/binabi/flags.sh holds the generated flag files to the MaixCDK ones.
#
#   MAIXLITE_TOOLCHAIN_BIN=<toolchain bin> cmake -G "Unix Makefiles" \
#       -DCMAKE_TOOLCHAIN_FILE=toolchain.cmake -DCMAKE_BUILD_TYPE=MinSizeRel <this dir>
#
# or scripts/build-maixlite.sh, which also strips and stages the binary.

cmake_minimum_required(VERSION 3.17)

if(NOT CMAKE_BUILD_TYPE STREQUAL "MinSizeRel")
    message(FATAL_ERROR "CMAKE_BUILD_TYPE must be MinSizeRel (MaixCDK's release build): '${CMAKE_BUILD_TYPE}'")
endif()

project(kvm_system LANGUAGES C CXX)

set(CMAKE_EXPORT_COMPILE_COMMANDS ON)

# --- flags: MaixCDK's platforms/maixcam.yaml c_flags/cxx_flags, then its compile_flags.cmake
set(_arch "-mcpu=c906fdv -march=rv64imafdcv0p7xthead -mcmodel=medany -mabi=lp64d")
set(CMAKE_C_FLAGS "${_arch} -Wall -fPIC")
set(CMAKE_CXX_FLAGS "${_arch} -Wall -fPIC -std=c++17")
# $ORIGIN is literal: the loader expands it.
set(CMAKE_C_LINK_FLAGS "-Wl,-rpath=\$ORIGIN/dl_lib")
set(CMAKE_CXX_LINK_FLAGS "-Wl,-rpath=\$ORIGIN/dl_lib")

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

set(CFG_DIR "${MAIXLITE_DIR}/config")
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
