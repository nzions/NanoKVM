/*
 * Static stand-in for MaixCDK's generated global_build_info_version.h (git tag and hash
 * of the MaixCDK checkout at build time). Nothing compiled for kvm_system reads these;
 * they are fixed so that no build embeds a date or a hash. Not upstream.
 */
#ifndef MAIXLITE_GLOBAL_BUILD_INFO_VERSION_H
#define MAIXLITE_GLOBAL_BUILD_INFO_VERSION_H

#define CONFIG_BUILD_VERSION_MAJOR 0
#define CONFIG_BUILD_VERSION_MINOR 0
#define CONFIG_BUILD_VERSION_MICRO 0
#define CONFIG_BUILD_VERSION_DEV 0
#define CONFIG_BUILD_GIT_COMMIT_ID "30f4b8b7e3f6"
#define CONFIG_BUILD_GIT_IS_DIRTY 0

#endif
