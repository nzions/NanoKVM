/*
 * Static stand-in for the global_config.h that MaixCDK's Kconfig step generated for
 * kvm_system (platform maixcam). Not upstream; not generated; no timestamps.
 *
 * What the vendored sources read from it: PROJECT_ID (components/basic/src/maix_app.cpp).
 * Every CONFIG_* the maixcam code paths test (CONFIG_BUILD_WITH_MAIXPY, CONFIG_OMP_ENABLE)
 * is off, which an undefined macro already means. PLATFORM_MAIXCAM, RELEASE and DEBUG come
 * from the -D flags in CMakeLists.txt, exactly as MaixCDK passed them.
 */
#ifndef MAIXLITE_GLOBAL_CONFIG_H
#define MAIXLITE_GLOBAL_CONFIG_H

#define PLATFORM "maixcam"
#define PROJECT_ID "kvm_system"

#endif
