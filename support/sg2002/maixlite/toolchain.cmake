# The cross toolchain of kvm_system: the image's /opt/sg2002/host-tools gcc (the MaixCDK
# "musl_t-head" host-tools, byte-identical). Its bin directory comes from the environment
# (scripts/build-maixlite.sh sets it); nothing is searched for and nothing is downloaded.
#
# CMAKE_SYSTEM_NAME is left at the host's, as MaixCDK's compile.cmake left it: setting it to
# Linux/Generic for a cross build changes which flags CMake adds to the link line.

set(_bin "$ENV{MAIXLITE_TOOLCHAIN_BIN}")
if(NOT _bin OR NOT IS_DIRECTORY "${_bin}")
    message(FATAL_ERROR "MAIXLITE_TOOLCHAIN_BIN is not set to a directory: '${_bin}'")
endif()
set(_prefix "${_bin}/riscv64-unknown-linux-musl-")
foreach(_t gcc g++ ar ranlib strip)
    if(NOT EXISTS "${_prefix}${_t}")
        message(FATAL_ERROR "${_prefix}${_t} does not exist")
    endif()
endforeach()

set(CMAKE_C_COMPILER "${_prefix}gcc")
set(CMAKE_CXX_COMPILER "${_prefix}g++")
set(CMAKE_AR "${_prefix}ar" CACHE FILEPATH "")
set(CMAKE_RANLIB "${_prefix}ranlib" CACHE FILEPATH "")
set(CMAKE_STRIP "${_prefix}strip" CACHE FILEPATH "")

# As MaixCDK: the compiler is trusted, not probed by linking a test program.
set(CMAKE_C_COMPILER_WORKS 1)
set(CMAKE_CXX_COMPILER_WORKS 1)
