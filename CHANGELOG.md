## 2.6.53 (2026-10-28)

### Fixes

* `kvm_system` segfaulted at start on a Cube (no OLED) and the supervisor restart-looped it (`unhandled signal 11 ... in libstdc++.so.6.0.28`). The OLED I2C buses were globals whose constructor throws when `/dev/i2c-N` is missing; the throw built a `maix::err::Exception`, whose `to_str` read the vendored `err_str` vector before its static initialiser had run. `kvm_system` no longer constructs any maix object at static-init time: the OLED buses open lazily, a failure is logged (`OLED disabled: cannot open /dev/i2c-1: ...`) and turns the OLED into a no-op, and `main` logs every uncaught exception and `std::terminate` (with `what()`) to stderr; each start-up step (new-image/new-app init, production patch, OLED probe, HDMI client) and each thread body (sys, oled, key, hdmi snapshots) logs its failure and is skipped or ends instead of taking the app down.
* The maix_err fix moved out of the vendored file: `components/basic/src/maix_err.cpp` is back to the upstream bytes (`UPSTREAM.sha256` verifies again); nothing in our code calls `maix::err::to_str` before `main`, so the vendored table is never read early.
* `S15kvmhwd` loads or removes `i2c-gpio.ko` / `i2c-algo-bit.ko` only when the file exists and logs one line when it is absent or the kernel refuses (a kernel without module support printed "kernel does not support requested operation").
* `scripts/forge-sdimage.sh` takes master's kernel pins (`kernel-nanokvm-sg2002-full` 1.6, tag `k1.6`, XTheadVector enabled) and builds on release 2.6.53.
* Not run on hardware

## 2.6.52 (2026-10-28)

### Changes

* The SD image is built with kernel-nanokvm-sg2002-full 1.4 (kernel 1.2 before): the kernel gains `CONFIG_MMC_SDHCI_PCI`, `CONFIG_GPIO_SIM` and `CONFIG_IRQ_SIM`. The userspace is unchanged from 2.6.51.

## 2.6.51 (2026-10-28)

### Fixes

* The hostname step never ran: `S30eth` called `timeout 15 sh hostname.sh`, but the image has no `timeout` (busybox lacks the applet, "timeout: not found"), so the `nanokvm-<id>` name was never set. `hostname.sh` is now run plainly in the background; it has no wait or loop. DHCP is unchanged.

## 2.6.50 (2026-10-27)

### Fixes

* 2.6.48 lost DHCP on hardware (the link light was up, no lease was ever requested; 2.6.44 got one on the same board). The cause was not reproduced: busybox 1.36.1 lists `udhcpc -x`, so the prime suspect, the foreground `set_hostname` that 2.6.48 ran before `udhcpc`, is unproven. The 2.6.49 retry logic is dropped. The network path in `/etc/init.d/S30eth` (`start()`: `ip addr flush`, the `/boot/eth.nodhcp` branch and `udhcpc -i eth0 -t 10 -T 1 -A 5 -b -p /run/udhcpc.eth0.pid &`) is restored byte for byte to what 2.6.44 ran, so `udhcpc` no longer sends `-x hostname:`.
* The hostname is set off the critical path: the functions moved to `/kvmapp/system/init.d/hostname.sh` (not an `S??` name, so rcS never runs it by itself), and the last thing `start()` does, after networking was launched, is run it in a background subshell under `timeout 15`; a failure or timeout only logs. It cannot block the boot or touch DHCP. avahi (S50) reads the kernel hostname when it starts, so the name being set before then is best-effort: mDNS may advertise the old name until avahi restarts. The name is no longer sent in the DHCP request.

## 2.6.49 (2026-10-27)

### Fixes

* Regression in 2.6.48: the SD image booted and the link light blinked but the board never sent a DHCP request (2.6.44 got a lease on the same board). The only networking change in 2.6.48 was in `/etc/init.d/S30eth`: `set_hostname` ran first in `start()` and both `udhcpc` calls gained `-x "hostname:$HN"`. The image's busybox (v1.36.1) does list `-x OPT:VAL` for udhcpc, so the option itself is not shown to be the cause; the root cause is not reproduced off-board. The script no longer lets the hostname work stand between the board and a lease: a failing `set_hostname` only logs, and if `udhcpc -x hostname:` exits non-zero (it did not start) it is run again without `-x`, in both the DHCP and the `/boot/eth.nodhcp` fallback branch. The DHCP branch runs in a background subshell so a slow lease never delays the boot. avahi still advertises the hostname over mDNS.

## 2.6.48 (2026-10-27)

### Changes

* Automatic hostname (Nik's ruling: "auto set the hostname to nanokvm-<id> if the board has a serial use the last 4 otherwise use the mac"). `/etc/init.d/S30eth` (the one place; it runs before udhcpc and avahi) sets the hostname at every boot: a non-empty `/boot/hostname` wins unchanged; otherwise `nanokvm-<id>`, `<id>` = the last 4 characters (lowercase `[0-9a-z]`) of the SoC unique id `/sys/class/cvi-base/base_uid` (the id S03usbdev and `/device_key` already use), then the device-tree `serial-number`, then the `/proc/cpuinfo` Serial; failing those, the last 4 hex digits of eth0's MAC. An id that is empty, all zeros or all f is skipped. With neither it keeps the existing hostname and logs why.
* It writes `/etc/hostname` and runs `hostname`, rewrites a `host-name=` pinned in `/etc/avahi/avahi-daemon.conf` if there is one (avahi otherwise advertises the kernel name, so mDNS shows `nanokvm-<id>.local`), and `udhcpc` now sends the name (`-x hostname:`).
* A hostname set in the web UI is written to `/boot/hostname` by the server, so it persists and wins over the automatic one. The automatic name is never written to `/boot`, so it follows the board and a later `/boot/hostname` takes over. Hand edits of `/etc/hostname` alone are replaced at the next boot.
* `scripts/FLASHING.md` and `scripts/sd-image.sh`: `/boot/hostname` is optional.
* Not run on hardware

## 2.6.47 (2026-10-27)

### Changes

* First connection sets the password (Nik's ruling: "no password then.... 1st connection sets the password"). With no `/boot/root.pass` at first boot the server no longer generates a seed password (it was shown on the OLED and kept in `/etc/kvm/initial.pass`; a Cube has no OLED, so the box was unreachable). Root is locked in `/etc/shadow` (`!`, no ssh password login) and the account state `/etc/kvm/pwd` records `passwordUnset`. `/boot/root.pass` is unchanged: present, it wins, is consumed and forces no change.
* `POST /api/auth/setup {"password": "..."}` (unauthenticated) sets the password: accepted only while it is unset, validated by `ValidatePassword`, written with `SetRootPassword`, bumps the token version and returns a logged-in session exactly like login (cookie `nano-kvm-token`). Under the store lock, so of concurrent setups exactly one wins. Once set it answers code `-7` ("password already set"), never a second setup. Anyone on the LAN who connects first claims an unset device.
* `POST /api/auth/login` while unset answers code `-6` ("password not set: call POST /api/auth/setup") and does not count as a failed attempt. `GET /api/auth/state` (unauthenticated) answers `{"password_set": bool}`. All codes travel in the body of an HTTP 200, as everywhere in this API.
* Web login page: when the password is unset it shows "Set root password" (password and confirm) and logs in on success.
* The BOOT-button reset (`kvm_system`) no longer generates a password: it locks root in `/etc/shadow` and removes `/etc/kvm/pwd`, which returns the device to the same unset state. The OLED no longer shows a password (an old `/etc/kvm/initial.pass` still is, until removed).
* A device whose password is already set is unaffected; a state with the old must-change-from-seed flag keeps working.
* `scripts/FLASHING.md` and `scripts/sd-image.sh`: `root.pass` is optional.
* Not run on hardware

## 2.6.45 (2026-10-26)

### Changes

* `scripts/forge-sdimage.sh` builds the Cube SD image on release 2.6.44 (tarball sha256 `bec56565afa7...34a68f`) with kernel `-full` 1.2 (tag `k1.2`); the image carries `othbootargs` for `cvi_vc_drv` in `uEnv.txt`. No runtime change.

## 2.6.44 (2026-10-26)

### Changes

* Fix forward of 2.6.43 (red on forge at the first step, nothing shipped): the edit that removed the MaixCDK steps from `scripts/forge-build.sh` left a stray separator line that bash ran as a command. Nothing else changes.
* Not run on hardware

## 2.6.43 (2026-10-26)

### Changes

* Every MaixCDK leftover is removed; the three binaries are unchanged. `scripts/forge-build.sh` no longer copies `/opt/maixcdk`, activates its venv or rewrites `maixcam.yaml`, and `.build.json` no longer requires `maixcdk`. Deleted: the MaixCDK dev script `support/sg2002/build`, the carrier project `support/sg2002/kvm_vision_test/`, the MaixCDK `CMakeLists.txt` and `Kconfig` of `support/sg2002/additional/{kvm,kvm_mmf,vision}` (the plain CMake under `support/sg2002/maixlite/libs/` builds them), and the unreferenced `support/sg2002/additional/sophgo-middleware` sample copy (`additional/kvm_mmf/pin/sample_common_sensor.c` is the one built). `docker/Dockerfile` drops the MaixCDK/venv stage; the Makefile loses the `support` and `vision` targets (`all` is `release-build`); the dev container, the builder workflow, `.gitignore`, `.dockerignore` and the READMEs lose their MaixCDK steps.
* `scripts/forge-build.sh` fails closed: the build exits non-zero if `$HOME/MaixCDK` exists (before or after the build), if the environment references MaixCDK, or if the build log contains a `Downloading` line or a MaixCDK path.
* Not run on hardware

## 2.6.42 (2026-10-26)

### Changes

* `libkvm_mmf.so` and `libkvm.so` are built without MaixCDK, and nothing in the release path builds with MaixCDK any more. `support/sg2002/maixlite` gains the `kvm_vision_test` project (`-DMAIXLITE_PROJECT=kvm_vision_test`; its own build tree, `libs/{vision,kvm_mmf,kvm}/CMakeLists.txt`, a `config-kvm_vision_test/` stand-in for the generated headers with `PROJECT_ID kvm_vision_test`): the same sources (`support/sg2002/additional/{kvm_mmf,kvm,vision}`, `NANOKVM_MIDDLEWARE_SRC` / `NANOKVM_MMF_LIB` / `NANOKVM_MMF_UAPI` as before), defines, include order, flags and link lines as the MaixCDK carrier project. `scripts/build-maixlite.sh` takes the project as a third argument, holds the flags to `tools/binabi/golden/kvm_vision_test.flags` (generated from the 2.6.41 evidence; the carrier executable and its `main` are not in it), strips both libraries as MaixCDK's `gen_binary.cmake` did and stages them in `kvmapp/server/dl_lib`. `scripts/build-in-container.sh` no longer syncs components into MaixCDK, activates its venv or runs `./build kvm_vision`.
* `libkvm.so` no longer carries an RPATH. MaixCDK's CMake left the absolute path of its build directory in it (`/work/home/NanoKVM/support/sg2002/kvm_vision_test/build/kvm_mmf`); the maixlite project sets `CMAKE_SKIP_RPATH`, and the two `libkvm.so` rpath entries in `tools/binabi/allow.txt` are gone. `flags.sh` deletes that one option from the MaixCDK `link.txt` it reads, as its only accepted difference. `kvm_system` keeps `-Wl,-rpath=$ORIGIN/dl_lib`.
* `scripts/forge-build.sh`: the MaixCDK download checks (they read a MaixCDK build log that no longer exists) are replaced by a check that the log shows both maixlite stagings and no MaixCDK line; the compiler check and the evidence flags now read the two maixlite build trees. The MaixCDK copy and venv it still makes are unused (removing them is S7).
* Not run on hardware

## 2.6.41 (2026-10-26)

### Changes

* Fix forward of 2.6.40 (red on forge, nothing shipped): the maixlite `kvm_system` compiled and linked, and every `flags.make` matched the MaixCDK golden; only two archive member names in `link.txt` differed (`libmain.a`, `libperipheral.a`), because CMake spells an object whose source is outside the target's directory with the source's absolute path. `tools/binabi/flags.sh` now maps those two prefixes (`kvm_system/main/`, `additional/peripheral/`) back to the MaixCDK spelling.
* Not run on hardware

## 2.6.40 (2026-10-26)

### Changes

* `kvm_system` is built without MaixCDK. `support/sg2002/maixlite/` is a trimmed vendor copy of the MaixCDK sources it compiled (pinned commit `30f4b8b7e3f6`: components `basic`, `peripheral`, `ini`, `ntp_client`, `yaml`; 153 unmodified files plus the Apache-2.0 `LICENSE`, each sha256 in `UPSTREAM.sha256` and checked before every build) with a plain CMake build (`CMakeLists.txt`, `toolchain.cmake`) of the same sources, defines, include order, flags and link line, and static `global_config.h` / `global_build_info_version.h` in place of the generated ones. `scripts/build-maixlite.sh` builds it, strips it as MaixCDK's `gen_binary.cmake` did and stages `kvmapp/kvm_system/kvm_system`; `scripts/build-in-container.sh` calls it instead of `./build kvm_system`. `libkvm.so` and `libkvm_mmf.so` still go through MaixCDK (S6).
* Flags-drift check: `tools/binabi/flags.sh` compares every generated `flags.make` and `link.txt` of the maixlite build with `tools/binabi/golden/kvm_system.flags` (the MaixCDK ones from the 2.6.39 evidence, paths normalised) and fails the build on any difference. The golden ABI check (`tools/binabi/check.sh`) now holds the maixlite `kvm_system`. The evidence bundle's `flags/kvm_system` is the maixlite build's (`flags.make`, `link.txt`, `CMakeCache.txt`, `compile_commands.json`, normalised).
* Not run on hardware

## 2.6.39 (2026-10-26)

### Changes

* Golden ABI manifest check (`tools/binabi/check.sh`, run by `scripts/forge-build.sh` right after `binprint`): `kvm_system`, `libkvm.so` and `libkvm_mmf.so` are compared field by field (NEEDED, SONAME, RPATH, sorted exports and imports, RISC-V attributes, `.comment`, `.text`/`.rodata`/`.data`/`.bss` sizes) with the golden files taken from the 2.6.38 evidence. A difference not in `tools/binabi/allow.txt` fails the build, and so does an allow entry that no longer differs. The one entry is `libkvm.so`'s build-dir RPATH leak, to be removed in S6. No shipped binary changes.
* Not run on hardware

## 2.6.38 (2026-10-26)

### Changes

* Fix forward of 2.6.37 (red on forge, nothing shipped): every MaixCDK step passed and the download check passed (no download line, no `dl/pkgs`), but the compiler check looked for `CMAKE_C_COMPILER` in `CMakeCache.txt`, where MaixCDK does not cache it; it now reads the compiler CMake settled on from `CMakeFiles/*/CMake{C,CXX}Compiler.cmake`.

## 2.6.37 (2026-10-26)

### Changes

* The MaixCDK build downloads nothing. It used to fetch the 266 MiB `host-tools-2025.7.28.tar.xz` (byte-identical, file by file, to the image's `/opt/sg2002/host-tools`, per the 2.6.36 evidence bundle) and, only because the `vision` component required `websocket`, `asio-1.28.0` and `WebSocketpp_0.8.2` from SourceForge. `scripts/forge-build.sh` now rewrites `platforms/maixcam.yaml` in its private MaixCDK copy to an empty `url` and an absolute `bin_path` of `/opt/sg2002/host-tools/...` (MaixCDK downloads only when the toolchain has a url, and joins `bin_path` onto `dl/extracted`, so an absolute path is used as is); both lines must match exactly once or the build stops.
* `support/sg2002/additional/vision` no longer requires `websocket` (nothing under `support/sg2002` includes websocketpp or asio), which also drops `-DASIO_STANDALONE=1`.
* The build fails if MaixCDK's output has any `-- Downloading`, `-- Download n%` or `-- Extracting` line, if `dl/pkgs` holds a file, or if `CMAKE_C_COMPILER` / `CMAKE_CXX_COMPILER` in the `kvm_system` and `kvm_vision_test` CMakeCaches do not resolve under `/opt/sg2002/host-tools`; it prints what it checked. The evidence bundle's toolchain part is now `compilers.txt` and `gcc-v.txt`; `scripts/toolchain-compare.sh` is removed, its comparison is done.
* Not run on hardware

## 2.6.36 (2026-10-26)

### Changes

* Build evidence, no change to any shipped byte: forge's `build` now also publishes `<version>/evidence/` next to the package (not in the OTA tarball). For `kvm_system`, `libkvm.so`, `libkvm_mmf.so` and `NanoKVM-Server` it holds `scripts/binprint.sh` fingerprints (dynamic section, sorted exports and imports, dynamic symbol types and visibilities, riscv attributes, `.comment`, section sizes, sha256, build id and embedded dates); for the two MaixCDK projects the compile and link flags CMake generated (`flags.make`, `link.txt`, `CMakeCache.txt`, `compile_commands.json` where CMake honours `CMAKE_EXPORT_COMPILE_COMMANDS`), with this build's paths normalised; and `scripts/toolchain-compare.sh`'s byte comparison of the MaixCDK-downloaded toolchain with the image's Sophgo one (`[evidence] toolchains: IDENTICAL` or `DIFFER`, informational). Together they let a build without MaixCDK be proven ABI-equivalent. An incomplete bundle fails the build.
* Not run on hardware

## 2.6.35 (2026-10-26)

### Changes

* Build fix for 2.6.34 (red on forge, nothing shipped): every source compiled and `libkvm.so` linked, but the `kvm_vision_test` link failed on `__atomic_exchange_1` (libkvm) and `__atomic_compare_exchange_1` (libsys): `-latomic` had only ever been on the link line through the OpenCV libraries. The `kvm` component now requires `atomic` itself.
* Not run on hardware

## 2.6.34 (2026-10-26)

### Changes

* OpenCV is gone from the image. `libkvm.so` NEEDed `libopencv_{core,imgproc,imgcodecs}.so.409` only because MaixCDK's whole `maix::image` class (drawing, load/save, fonts) was compiled in; the capture path only ever used `cam->read()` (MMF YVU420SP frames) and the hardware JPEG / H.264 encoders. The `vision` component is now a NanoKVM cut of MaixCDK's: `Image` keeps its constructors, `data()` / `data_size()` / `format()` / `width()` / `height()` and `to_jpeg()` reduced to the hardware YVU420SP branch (any other format returns `nullptr` and logs); the camera accepts only `FMT_YVU420SP` (no format conversion, no colour bar). `maix_image_ops.cpp`, `maix_image_obj.hpp`, `maix_image_color.hpp` and `maix_image_util.hpp` are deleted, and `vision` no longer requires `opencv`, `opencv_freetype` or `omv`, so MaixCDK's FreeType and HarfBuzz are no longer embedded either.
* Removed: `scripts/build-deps.sh` (its only job was the OpenCV build), `support/sg2002/additional/opencv` and its copy in `support/sg2002/build`, `NANOKVM_OPENCV_DIR`, the OpenCV staging into `kvmapp/server/dl_lib` and `server/dl_lib`, and the libkvm-uses-our-OpenCV checks. The package no longer ships the three `libopencv_*.so.409`. The `opencv-4.9.0.tar.gz` on the artifacts share is left alone, unused.
* Build check (fail closed) in `build-in-container.sh` and again on the extracted package in `forge-build.sh`: `libkvm.so` has no `libopencv*` NEEDED entry and imports no `cv::` symbol, and no `libopencv*` file is staged or in the package; the log prints the `libkvm.so` NEEDED list and size.
* Not run on hardware

## 2.6.33 (2026-10-26)

### Changes

* Build fix for 2.6.32 (red on forge, nothing shipped): OpenCV writes `CMakeDownloadLog.txt` on every configure, so failing on its existence always tripped. Per `cmake/OpenCVDownload.cmake` (4.9.0) the check now prints the log and fails only on a network-download record (`#cmake_download` / `#try`). G-API and ADE stay off and the download cache must hold no non-empty file.
* Not run on hardware

## 2.6.32 (2026-10-26)

### Changes

* Build fix for 2.6.31 (red on forge, nothing shipped): the only entry in OpenCV's download cache directory after a download-free configure is the `.gitignore` OpenCV writes there itself; the check now ignores exactly that file.
* Not run on hardware

## 2.6.31 (2026-10-26)

### Changes

* Build fix for 2.6.30 (red on forge, nothing shipped): OpenCV configured without any download (G-API is unavailable, no ADE), but the "download cache is empty" check treated OpenCV's own placeholder entry in the cache directory as a download. It now fails only on a non-empty regular file there and prints what the directory holds.
* Not run on hardware

## 2.6.30 (2026-10-26)

### Changes

* OpenCV's configure no longer downloads anything: `build-deps.sh` turns G-API and ADE off (`-DBUILD_opencv_gapi=OFF -DWITH_ADE=OFF`; 2.6.29 fetched ADE v0.1.2d from github.com at configure time, for a module libkvm does not use) and points `OPENCV_DOWNLOAD_PATH` at an empty directory. After configuring it fails closed unless that directory is still empty, there is no `CMakeDownloadLog.txt`, and the configure output has no download line.
* Not run on hardware

## 2.6.29 (2026-10-26)

### Changes

* Build fix for 2.6.28 (red on forge, nothing shipped; libkvm.so itself linked against our OpenCV): the build-tree check flagged `libopencv_freetype.a`, MaixCDK's own contrib freetype component compiled against our headers, as a foreign OpenCV. It is now the one named exception.
* Not run on hardware

## 2.6.28 (2026-10-26)

### Changes

* Build fix for 2.6.27 (red on forge, nothing shipped): MaixCDK links a library path as `-L<dir> -l<name>`, so `ld` looked for `libopencv_core.so` and found only `.so.409`. `build-deps.sh` now also writes `<outdir>/link/libopencv_{core,imgproc,imgcodecs}.so` symlinks to the `.so.409` files (link time only, not shipped; the SONAME `ld` records is still `.so.409`), the opencv component links those, and the build-tree check looks for `<outdir>/link`.
* Not run on hardware

## 2.6.27 (2026-10-26)

### Changes

* libkvm.so now compiles and links against OUR OpenCV 4.9.0 build, not MaixCDK's prebuilt. `support/sg2002/additional/opencv` replaces MaixCDK's opencv component (copied over it by `support/sg2002/build`): headers from `build-deps.sh` (`include/opencv4`) and its three `.so.409` files, `NANOKVM_OPENCV_DIR` required, no fallback (configure error). The OpenCV build now runs before the libkvm build.
* Build check: `build-in-container.sh` requires every OpenCV include/link path in the kvm_vision build tree to be inside `build/deps` (none into a MaixCDK prebuilt), `libkvm.so` NEEDED OpenCV to be a subset of our three sonames including core, every `cv::` symbol it imports to be defined by those libraries, and the staged `.so` files to be byte-identical to the build; it prints the evidence and sha256s. Fails closed.
* `build-deps.sh` no longer downloads from github.com: the OpenCV source is `opencv-4.9.0.tar.gz` on the artifacts share (`src/`, `$NANOKVM_SRC_MIRROR`), sha256 `ddf76f9dffd322c7c3cb1f721d0887f62d747b82059342213138dc190f28bc6c`, missing or different aborts.
* Not run on hardware

## 2.6.26 (2026-10-26)

### Changes

* Build check: `forge-build.sh` runs `tools/abicheck` on the pinned middleware (4e231ef) against the two osdrv uapi header trees `build-mmf.sh` already extracted (the riscv64 musl cross gcc for layouts, host `cc` for the executed probe). Any mismatch not listed in `tools/abicheck/allow.txt` (each entry has its reason: `USER_FRAME_INFO_S` / `USER_RC_INFO_S` under `CVI_VC_VENC_SEND_FRAMEEX`, `CVI_ERR_VENC_BUTT`) fails the build, and so does a listed entry that no longer differs or a missing tool.
* abicheck fix: an empty names file shifted awk's file index and hid every vc_drv boundary difference.
* Not run on hardware

## 2.6.25 (2026-10-26)

### Changes

* Build check: after packaging, `forge-build.sh` extracts the archive and requires the 12 MMF libraries inside to be byte-identical to what `build-mmf.sh` staged (fail closed), and `build-in-container.sh` prints their sha256 and size right after staging. 2.6.24 built green, but its blobs were the pin's files without debug sections, not the pin's bytes.
* Not run on hardware

## 2.6.24 (2026-10-26)

### Changes

* Build fix for 2.6.23 (red on forge, nothing shipped): the closure check found libsys's one undefined symbol, `__atomic_compare_exchange_1`, which the MaixCDK libsys leaves undefined too; `libkvm.so` NEEDs `libatomic.so.1`, which defines it. libatomic is now a system library of the check.
* Not run on hardware

## 2.6.23 (2026-10-26)

### Changes

* Build fix for 2.6.22 (red on forge, nothing shipped): the generated `cvi_json-c/cvi_debug.h` (json-c's debug.h) shadowed the middleware's `cvi_debug.h` in `libcvi_bin`'s include path; the generated dirs now come after the middleware's (`-idirafter`).
* Not run on hardware

## 2.6.22 (2026-10-26)

### Changes

* `libkvm_mmf` links only libraries forge builds from the pinned scpcom/sophgo-middleware `4e231ef5967f`, plus that pin's closed ISP/3A blobs. Nothing comes from MaixCDK's `v2/lib` any more (the 12 tracked blobs are gone from `server/dl_lib`).
  * `scripts/build-mmf.sh` builds `libsys libcvi_bin libvpu libvenc libvdec libini` with the pin's own `Makefile.param` / `modules/*/Makefile` and the riscv64 musl cross compiler (nothing is run). `libisp libcvi_bin_isp libisp_algo libae libaf libawb` are copied from `v2/modules/isp/lib/musl_riscv64` of the same pin; their sha256 and the build provenance (the pin's source path in each built library) are printed in the log.
  * Inputs on the artifacts share, sha256-checked and fail closed: `osdrv-cbd9c582.tar.gz` / `osdrv2-1b3a4964.tar.gz` (the kernel uapi headers the 7.3 kernel is built from; the pin ships none) and `json-c-0.15-20200726.tar.gz`. `scripts/gen-cvi-headers.sh` generates the `cvi_json-c` / `cvi_miniz` declarations `libcvi_bin` includes (Sophgo's OSS tarballs have no source); the symbols resolve at runtime from `libcvi_bin_isp`.
  * `kvm_mmf`'s CMake, with `NANOKVM_MIDDLEWARE_SRC` set, takes headers, `sample/common` and the link set from the pin and the build output: `v2/include`, `v2/modules/isp/include/cv181x`, the osdrv uapi dirs, `NANOKVM_MMF_LIB`. `sample_common_sensor.c` is the pin's file plus the two NanoKVM changes (`kvm_mmf/pin/sample_common_sensor.c`).
  * `scripts/check-mmf-closure.sh`, run for the 12 libraries and for `libkvm_mmf` after the link: every NEEDED entry must be in the shipped set or a system library, and every undefined global symbol must be defined in the set or the system libraries; `build-in-container.sh` also refuses any library in `kvmapp/server/dl_lib` that is not the pinned build byte for byte. `package.sh` `BUILT_LIBS` now lists all 17 libraries.
* Release builds need `scripts/forge-build.sh` (`make release` in docker lacks the share inputs and fails closed).
* Link and load only: nothing runs on hardware; the runtime proof is the kvm1 smoke test.

## 2.6.21 (2026-10-26)

### Changes

* The forge build fetches the middleware source of record, scpcom/sophgo-middleware `4e231ef5967f` (licheervnano, tree under `v2/`), from the artifacts share instead of GitHub: `scripts/fetch-middleware.sh` reads `<share>/src/sophgo-middleware-4e231ef5967f.tar.gz` (git archive of that commit, sha256 `a431a22dac1c...350b`), checks the pinned sha256 and extracts it to `$HOME/mw-src/sophgo-middleware-4e231ef`. A missing file, an unset mirror or any other hash aborts the build before the SDK compile; there is no fallback.
* `kvm_mmf`'s CMake takes `NANOKVM_MIDDLEWARE_SRC` to point `middleware_src_path` at that tree. Nothing exports it yet, so headers, sources and the link are still MaixCDK's middleware: 4e231ef has no `v2/include/isp/cv181x` and no `v2/uapi`, so the include set cannot be switched on its own. Linking our own built libs is the next step.
* Not run on hardware

## 2.6.20 (2026-10-26)

### Changes

* The ION debugfs readers are gone: `/sys/kernel/debug/ion/cvi_carveout_heap_dump/summary` does not exist on the 7.3 kernel (its ION compat, `cvi_ion.c` in kernel-nanokvm-sg2002, is dma-buf on the sys CMA pool and creates no debugfs node), so all three readers were dead. `kvm_mmf` lost the ION leak scan (`_free_leak_memory_of_ion`; the kernel frees a dead process's dma-bufs on fd close; the VB scan of `/proc/cvitek/vb` stays), `kvm` lost the "ION usage >= 95% -> reboot" watchdog check (`chack_ion`, which returned 0 and never fired), and `kvm_system` lost the empty `ion_free_space` stub.
* Not run on hardware

## 2.6.19 (2026-10-26)

### Changes

* `scripts/forge-sdimage.sh` builds the Cube SD image on release 2.6.18 (tarball sha256 `ebd67e58a576...b0258f`) with kernel `-full` 1.2 (tag `k1.2`); the image carries `othbootargs` for `cvi_vc_drv` in `uEnv.txt`. No runtime change.

## 2.6.18 (2026-10-26)

### Changes

* Image diet: 21 libraries nothing loads are no longer shipped (20 tracked blobs removed from `server/dl_lib`, plus the built `libjson-c`). The tracked set is now exactly the `libkvm.so` NEEDED closure: `libae libaf libawb libcvi_bin libcvi_bin_isp libini libisp libisp_algo libsys libvdec libvenc libvpu`). Removed: the audio stack (`libcvi_audio`, `libcvi_VoiceEngine`, `libcvi_vqe`, `libdnvqe`, `libaac*2`, `libtinyalsa`), `libcli` (network CLI with a root login prompt), `libcvi_ispd2` (ISP tuning daemon), `libcvi_ssp`, `libosdc`, `libraw_dump`, `libmipi_tx`, `libmisc`, `libcvi_RES1`, `libcvi_ive`, `libopencv_highgui`, and `libjson-c.so.5` (only `libcvi_ispd2` NEEDed it, so its build is gone from `scripts/build-deps.sh`).
* `libcvi_ive`: the IVE colour-space code in `kvm_mmf.cpp` is dead (`cvi_rgb2nv21` has no caller, `cvi_ive_init`/`deinit` are only `UNUSED()`-ed); `libkvm_mmf.so` has no undefined `CVI_IVE_*` symbol.
* `package.sh` prunes the dropped names from the staged tree (`DROPPED_LIBS`). `server/dl_lib/README.md` lists the closure and the removals.
* Not run on hardware (kvm1 offline)

## 2.6.17 (2026-10-26)

### Changes

* No insmod/rmmod of `soph_*` any more: the Sophgo media drivers are built into the kernel (kernel-nanokvm-sg2002 `-full`). `kvm_mmf` lost `reinit_soph_vb` (rmmod and 11 insmods) and `_is_module_in_use`; with VB pools left over from an earlier init it now logs and carries on (a built-in driver cannot be reloaded). `kvm_system` lost `rmmod soph_saradc` and the `soph_mipi_rx.ko` md5-check-and-reboot step, which on a static kernel (no such file) would have rebooted on every app update.
* `kvmapp/system/init.d/S00kmod` loads nothing. Its one parameter, `soph_vc_driver.ko MaxVencChnNum=9 MaxVdecChnNum=9`, is now `cvi_vc_drv.MaxVencChnNum=9 cvi_vc_drv.MaxVdecChnNum=9` (`module_param` in `cvi_vc_drv.c`, built-in object `cvi_vc_drv`; driver defaults are 16 and 64). `scripts/sd-image.sh --bootargs` writes them as `othbootargs=` to `uEnv.txt` on the boot partition (forge-sdimage.sh passes them); S00kmod sets them from `/sys/module/cvi_vc_drv/parameters/` (mode 0644) when the command line did not. `--no-stock-ko` now installs this S00kmod instead of its own stub.
* Still loaded as modules: `i2c-gpio.ko` / `i2c-algo-bit.ko` in `S15kvmhwd` (not `soph_*`; the load fails harmlessly where the files are absent).
* Not run on hardware (kvm1 offline)

## 2.6.16 (2026-10-26)

### Changes

* No library we build is tracked in git any more. `server/dl_lib` loses `libkvm.so`, `libkvm_mmf.so` (both already replaced by the forge build in every package), `libopencv_core/imgproc/imgcodecs.so.409` and `libjson-c.so.5`. `scripts/build-deps.sh` builds OpenCV 4.9.0 (core, imgproc, imgcodecs; bundled libjpeg-turbo, libpng, zlib) and json-c 0.15-20200726 from sha256-pinned source tarballs with the riscv64 musl T-Head compiler and the MaixCDK `-march` flags; a mismatch aborts the build. `build-in-container.sh` stages them in `kvmapp/server/dl_lib` and copies them to `server/dl_lib` for the server link; `package.sh` refuses a package missing any of the six, or with one that is not riscv64, instead of backfilling from git.
* `server/dl_lib/README.md` lists every remaining file and its source: all vendor Sophgo/CVITEK SDK blobs plus MaixCDK's unused OpenCV highgui. libkvm's FreeType and HarfBuzz were already built from source by MaixCDK. `libtinyalsa.so` is Sophgo's modified build, not upstream, so it stays a vendor blob.
* Not run on hardware (kvm1 offline). The OpenCV build is configured from scratch (no CPU dispatch, OpenMP and pthread parallel framework as before); libkvm still compiles against MaixCDK's 4.9.0 headers.

## 2.6.15 (2026-10-26)

### Changes

* LT6911 access is serialised across processes. `lt6911.c` takes the in-process mutex and an `flock(LOCK_EX)` on `/var/run/lt6911.lock` (a dedicated file, so it does not depend on what the i2c node does with flock) around every transaction; the flock is polled with `LOCK_NB` for at most 10 s and then fails with `-ETIMEDOUT` ("bus busy"), never a hang. New `lt6911_session_begin` / `lt6911_session_end(changed)` hold both locks across a multi-step sequence; the per-call functions run inside the session without retaking them (a thread-local flag, no recursive mutex; a nested begin is `-EBUSY`).
* Resync after an EDID write: `lt6911_session_end(dev, true)` rewrites the lock file once the locks are free; the HDMI state machine (libkvm) watches it with inotify (`lt6911_watch_open` / `lt6911_watch_changed`) and treats it as a resync request, so it re-reads the chip and publishes. If the watch cannot be opened the state machine says so and falls back to INT/resync only.
* `nanokvm_update_edid` runs reset, flash, verify and reset as one session and ends it "changed" even on failure; every `lt6911_enable()` / `lt6911_disable()` result is now checked; a read error or an over-long EDID file is an error. It keeps no page/offset cache and has no variable-length buffer (the library's `LT6911_MAX_BLOCK` buffer is fixed).
* Not run on hardware (kvm1 offline); the lock, nested-begin and change notice were exercised on a host build.

## 2.6.14 (2026-10-26)

### Changes

* kvm_mmf: `SAMPLE_COMM_VI_ParseIni` returns `CVI_SUCCESS` (0) when an ini parsed and `CVI_FAILURE` when none could be (it also fills the struct with defaults first). `_mmf_init` treated non-zero as success, logged "Parse complete" and went on; it now logs the failure and returns the error. `_try_release_sys` already returned on non-zero, but with a misleading message; fixed. Both zero `stIniCfg` first, and `_mmf_init` now checks `CVI_VI_SetDevNum`. A board with neither `/mnt/data/sensor_cfg.ini` nor the default ini now fails mmf init instead of running on defaults
* kvm_mmf: the VB leak scan collected nothing and called `CVI_SYS_Exit` + `CVI_VB_Exit` once per pool, mid-scan. It now reads every pool first, tears down once (return values checked), then releases the leaked blocks. The ION leak scan called `mmf_deinit()` and returned in the middle of the scan on a failed `ionFree`; it now collects the VI_DMA_BUF entries, closes the file, frees them all, and calls `mmf_deinit()` once if any free failed. Both scans now check `ferror`/`fclose` and return -1 on any failure (the caller only logs it)
* Not run on hardware (kvm1 offline)

## 2.6.13 (2026-10-26)

### Changes

* One LT6911 driver. `support/sg2002/additional/kvm/src/lt6911.c` + `internal/lt6911.h` (plain C, linked into libkvm and compiled into `nanokvm_update_edid`) own `/dev/i2c-4`: the Maix `i2c::I2C LT6911_i2c(4)` global and the three copies of the variant code in `kvm_vision.cpp` (enable/disable, hdmi_res, csi_res, plus the unused register dump helpers) are gone, and so are the edid tool's own four I2C helpers and page cache. (kvm_system's dead `hdmi.cpp` and its global I2C object were already removed in 2.5.17.)
* Checked I/O: every transfer is an `I2C_RDWR` ioctl whose result and count are checked; every call returns 0 or a negative errno. The old unchecked `readfrom()` result dereference is gone. `lt6911_reg_read/_write(dev, page, reg, ...)` select the page themselves on every call (the page register is never caller state) under one process-wide lock; `lt6911_probe` holds that lock across enable -> query -> disable. Page select is its own write (its own STOP), then the register access, as the old code and the EDID tool did on the wire
* `enum lt6911_variant {UNKNOWN, C, UXC, D, UXE}` is detected once by `lt6911_open` from `/etc/kvm/hdmi_version` (S15kvmhwd's marker: c / ux / d / ue) and drives a `struct lt6911_ops` table (`enable`, `disable`, `signal_present`, `csi_res`, address, INT edge). A missing, unreadable or unknown marker is an error (the state machine logs it and does not probe) where `kvm_vision` used to fall back to C. `hdmi_int_edge_seen` takes the ops table's `enum lt6911_int_edge`
* Hactive scale reconciled: the width register is scaled per register (`lt6911_res_regs.h_scale`). The C receive side (page D2) and D (page E0) count pairs of pixels (x2); the C CSI side (page C2) and UXC CSI (page 85) are real pixels. The old code multiplied C's receive side by 2 only for a log line, and D's by 2 for both reads: the sizes handed to the VI are unchanged. One deliberate change: "signal present" is `width != 0 && height != 0` for every variant (D used `||`)
* A probe whose I2C failed no longer reads as "no signal": the state is kept, the error logged, and a resync request is still answered
* Address: every row of the ops table is 0x2B, as before. S15kvmhwd detects a C by an ACK at 0x2C and UXC/D at 0x2B, while kvm_vision and the edid tool have always talked to a C at 0x2B; the tree cannot say which address a C answers at, so behaviour is kept and the question is open (`i2cdetect -y 4` on a C board)
* `nanokvm_update_edid` uses the library (variant detection, bus open, every register access); its build line now includes `lt6911.c`
* Not run on hardware (kvm1 offline). libkvm_mmf's prebuilt Sophgo `lt6911_cmos.c` is a fourth, unremoved bus user: it opens `/dev/i2c-%u` and probes/reads/writes the chip when the VI chain starts

## 2.6.12 (2026-10-26)

### Changes

* One board, detected once. `internal/kvm_board.h` (header-only, shared by libkvm, `kvm_system` and `nanokvm_update_edid`) reads `/etc/kvm/hw` once (`pthread_once`, checked I/O) into `struct kvm_board {kind, name, has_hdmi_reset, hdmi_reset_line, hdmi_reset_sysfs_fallback}` with `enum kvm_board_kind {UNKNOWN, CUBE_ALPHA, CUBE_BETA, PCIE}`. A missing, unreadable or unrecognised file is logged and the board is UNKNOWN (no reset line), not silently `alpha`. The reset line name `hdmi-rst`, its sysfs fallback pin (451) and the reset pulse (`KVM_HDMI_RESET_PULSE_US`, 100 ms) now live only there; libkvm's `hw_version`, the oled_ctrl parser and the edid tool's own `/etc/kvm/hw` parser are gone
* The Cube is not an error: `kvmv_hdmi_control` returns `KVMV_HDMI_NO_RESET_LINE` on a board without a reset line (`KVMV_HDMI_ERROR` is now what the old `-1`, which Go read as 255, meant). The server maps the three results to `common.HdmiSwitch` and logs an error only for a real failure
* Waits: `kvm_system` thread flags are read and written atomically, and the shutdown wait for the OLED thread has a 5 s deadline instead of looping forever; libkvm's watchdog thread reads `try_exit_thread` atomically

## 2.6.11 (2026-10-26)

### Fixes

* The 2.6.10 build stopped at the server link: the fresh `libkvm.so` the server now links against leaves the mmf and opencv symbols to the package's other libraries, so the server's cgo flags carry `-Wl,--allow-shlib-undefined` and `build-in-container.sh` copies only `libkvm.so`. No other change

## 2.6.10 (2026-10-26)

### Changes

* HDMI state is pushed, not polled or filed. libkvm publishes one immutable snapshot `{state, width, height, seq}` per state machine transition (`kvm_hdmi_snapshot`, `kvm_hdmi.h`) and `kvmv_hdmi_wait(last_seq, &out, timeout_ms)` blocks on a condvar until a newer one exists (a timeout only means nothing changed; `kvmv_deinit` releases every waiter). `kvmv_hdmi_signal_active` is gone
* The VI init no longer reads `/kvmapp/kvm/width` and `/kvmapp/kvm/height` (written with `system("echo")` + `sync`, truncate-first race, read into a 35-byte buffer): `/kvmapp/kvm/width`, `/kvmapp/kvm/height` and `/kvmapp/kvm/state` do not exist any more. libkvm hands a `struct kvm_resolution` to the sensor code (`SAMPLE_COMM_SNS_SetCustomSize`) before it restarts the VI chain
* Server: new `service/hdmi` package owns the state: `Hub` (typed `Snapshot`, fan-out with one-slot latest-wins subscriptions, a late subscriber gets the current snapshot), fed by a goroutine blocked in `kvmv_hdmi_wait`. It feeds the existing websocket channel (event `hdmi-state`, the current snapshot on connect), `GET /api/vm/hdmi` (adds `snapshot`; `signal` is `enabled && state == streaming`), `GET /api/vm/input-resolution`, and a unix socket `/var/run/kvmhdmi.sock` (one JSON line per transition) for kvm_system
* The 1 s capture warm-up sleep is gone: after capture is switched on, a capture reader waits for libkvm's next Streaming or NoSignal snapshot (bounded by the request context); the answer to a resync is always published, even when it changes nothing
* kvm_system reads the resolution from that socket in a blocking thread; its state-file writer and width/height readers are removed
* Web: `hdmi-state` websocket events feed `hdmiSnapshotAtom`; the HDMI settings entry shows the live resolution and no longer flips its switch after a 1 s timer
* `build-in-container.sh` links the server against the freshly built `libkvm.so` (the tracked copy lacks the new export)
* Not run on hardware (kvm1 offline)

## 2.6.9 (2026-10-26)

### Changes

* scripts/forge-sdimage.sh: the SD image carries release 2.6.8 (userspace GPIO by DT line name, kvmgpio), read straight from `/artifacts/2.6.8/` where the release build published it. No device code changed

## 2.6.8 (2026-10-26)

### Fixes

* tools/kvmgpio: include `<sys/time.h>` (musl needs it for `struct timeval`; the 2.6.7 build stopped there, nothing was published). No other change

## 2.6.7 (2026-10-26)

### Changes

* Every userspace GPIO user finds its line by the device-tree line NAME over the GPIO chardev (v2 uABI), not by a vendor sysfs number: `hdmi-rst`, `oled-rst`, `wifi-en`, `pwr-key`, `rst-key`, `pwr-led` (Alpha `hdd-led`). The vendor sysfs numbers (451, 502, 503, ...) are used only when NO line carries the name (the vendor kernel); a line that exists but cannot be driven, a name carried twice or an unreadable chip is a loud error, never a guessed number
* `kvm_gpio.h` (libkvm include/internal): the by-name lookup, line requests and get/set, shared by `hdmi_int.h` (now built on it), `kvm_vision.cpp`, `nanokvm_update_edid` and the new owner
* `tools/kvmgpio`: static helper that OWNS the held outputs (a line request lives as long as its fd). `S15kvmhwd` starts `kvmgpio daemon` (idempotent, returns once it listens, hold nothing and exit 1 on a missing line), drives `oled-rst`, `wifi-en` and `hdmi-rst` through it, and skips the devmem pinmux of the GPIO pads on the chardev path (the mainline DT muxes them). libkvm's `hdmi-rst` assert/release and `nanokvm_update_edid`'s reset go through the owner's unix socket `/var/run/kvmgpio.sock`; the EDID tool no longer writes an EDID after a failed reset
* Server: new `gpio` package (ioctls via golang.org/x/sys/unix, already vendored). The power and reset key presses and the power/HDD LED reads request the named line per operation and release it; the vendor sysfs file is the fallback only for `ErrNoLine`. An unparsable LED value is an error now (it was read as "off")
* `nanokvm_update_edid` and `kvmgpio` are built by `scripts/build-in-container.sh` (the tracked prebuilt edid binary predated the change); `kvmgpio` ships under `system/tool/`

## 2.6.6 (2026-10-26)

### Fixes

* scripts/sd-image.sh: the boot-partition comparison also ignored the free-space line but not the `9 files N bytes` total, which moves with the new BOOT.SD. No device code changed

## 2.6.5 (2026-10-26)

### Fixes

* scripts/sd-image.sh: the check that the new DTB carries its `/memreserve/` compared dtc's text with a fixed spelling; dtc prints `/memreserve/<tab>0x0000000080000000 0x0000000000080000;`. The numbers are compared now. No device code changed

## 2.6.4 (2026-10-26)

### Fixes

* scripts/sd-image.sh: `--kernel-fit` put the DTB's `/memreserve/` line at the wrong place (a GNU sed range that starts at line 1 runs to the end of the file); it is inserted after `/dts-v1/;` with awk. Found by the first forge run. No device code changed

## 2.6.3 (2026-10-26)

### Changes

* scripts/forge-sdimage.sh: the forge build of the Cube's first-boot SD image (`forge_build repo=nanokvm scope=["scripts"]`; scripts/forge-build.sh dispatches on `FORGE_SCOPE=scripts`). Downloads the pinned kernel pair, base image and Debian .debs (mkimage, dtc, mtools, e2fsprogs; forge-c lacks them), checks every sha256, runs `sd-image.sh --kernel-fit --no-stock-ko`, reads the result back with scripts/sdimage-verify.sh (MBR, FAT listing, FIT listing, kernel decompressed from the FIT compared with the input Image, DTB, S00kmod, e2fsck) and publishes `/artifacts/sdimg/nanokvm-cube-<ver>-<kernel>/`. No device code changed

## 2.6.2 (2026-10-26)

### Changes

* scripts/sd-image.sh: `--kernel-fit <Image> <dtb>` builds the first-boot image for OUR mainline kernel: the stock FSBL, OpenSBI and U-Boot (FIP.BIN) and the rest of the 16 MB FAT16 boot partition stay, and `BOOT.SD` (the FIT stock U-Boot loads to 0x81800000 and boots with `bootm ${uImage_addr}#config-sg2002_licheervnano_sd`) is replaced by a FIT that mkimage builds from the Image and DTB. The node names, `#address-cells = <2>`, the riscv/linux kernel image with a crc32 hash and the sha256-hashed flat_dt come from the stock FIT (read with dumpimage); there is no ramdisk, the DTB boots `root=/dev/mmcblk0p2` itself. The kernel is lzma-compressed (stock U-Boot 2021.10 does gz, bz2, lzma, lz4, zstd; `--fit-comp gzip` is the fallback) because the 38 MB Image does not fit the partition, and is loaded at `--fit-load` (default 0x83000000), above the place U-Boot reads the FIT to, so decompression cannot overwrite its own input or the DTB. The DTB gets `/memreserve/ 0x80000000 0x80000` (`--fw-reserve`): the stock bootargs hold OpenSBI back with `riscv.fwsz=0x80000`, which only the vendor kernel reads. After the edit the script reads BOOT.SD back from the FAT, compares it, extracts kernel and DTB from the FIT again and checks the kernel decompresses to the input Image, and compares FIP.BIN and the directory with the stock partition
* scripts/sd-image.sh: `--no-stock-ko` replaces `S00kmod` (insmod of the stock `soph_*.ko`) by a no-op, in `/etc/init.d` and in the `/kvmapp/system/init.d` copy kvm_system refreshes it from, because the kernel carries its drivers; other init scripts that still insmod/rmmod are listed in the log
* scripts/sd-image.sh: the partition table is read from the MBR with od (no sfdisk), and the input may be an image this script built before (`--stock-sha256` of that image): its FSBL, U-Boot and boot partition are the stock ones
* Not boot-tested: the SG2002's real FSBL and the C906 do not run under QEMU. Structure only (FIT listing, FAT read-back, rootfs read-back)

## 2.6.1 (2026-10-26)

### Fixes

* nanokvm-supervise: a supervised program that exited (the web server after a SIGTERM or a crash) was never restarted, and the supervisor sat at 100% CPU. busybox 1.36.1 ash mishandles SIGCHLD that arrives while it is inside its `wait` builtin or its in-process `sleep` applet: `nanosleep` returns EINTR and ash then loops on `wait4(-1, WNOHANG)` = ECHILD forever without sleeping again (strace on nanokvm-emu). The supervisor now polls with `kill -0` and an external `/bin/sleep 1` (a forked child, which ash waits for with a plain blocking `wait4`), and calls `wait` only once the job has finished. Measured on nanokvm-emu with the real server: a SIGTERM'd server was running again under the same supervisor at the first check 7 s later (supervisor asleep, CPU flat), `S95nanokvm stop` returned in 3 s with nothing left running, and `start` brought it back. Not hardware-tested: kvm1 was unreachable

## 2.6.0 (2026-10-26)

### Changes

* libkvm: the HDMI state machine takes the LT6911 INT line as a GPIO character device v2 line-event fd (both edges) instead of a 100 ms timerfd draining Sipeed's `/proc/lt_int` counter, so it sleeps in `poll()` until the line moves. Same decision as before: variant 0 probes on a rising edge, 1 and 3 on a falling edge. The code is `include/internal/hdmi_int.h`, shared with the emulator harness `tools/hdmiint`
* libkvm: the INT line is found by its device-tree line NAME, `hdmi-int` (a `gpio-line-names` entry on the board DT), or by an explicit `<gpiochip name> <offset>` in `/etc/kvm/hdmi_int_line`. No global GPIO number is hardcoded; vendor GPIO 353 is the RTC bank (base 352) offset 1 in the vendor numbering. Needs the Cube DT to name the line
* libkvm: fail closed. If the line cannot be found or requested, the state machine logs the reason and `FATAL` to stderr and ends without probing the chip or reporting a cable; the state stays "not connected", never a guess
* libkvm: removed `publish_vi_state` and its 10 s read of `/proc/cvitek/vi_dbg` (about a second blocking, absent on a 7.3 kernel). The shared VI state is no longer published; kvm_system falls back to its own direct read and reports the state unknown (-1) when that is absent too
* scripts/forge-build.sh: builds tools/hdmiint (riscv64 musl, static) and publishes it as `emu/hdmiint` (emulator only)
* Not hardware-tested: emulator (gpio-sim) only

## 2.5.20 (2026-10-26)

### Changes

* scripts/emu-image.sh: `--authorized-key <pubfile>` authorises a public key for root in the emulator image (`/root/.ssh` 0700, `authorized_keys` 0600, root-owned, read back), so the image build is reproducible instead of hand debugfs edits; scripts/README.md documents it
* No device code changed

## 2.5.19 (2026-10-26)

### Changes

* tools/gpioedge: a dependency-free Go program for the emulator that makes a gpio-sim chip through configfs, requests line 0 for both edges (GPIO_V2_GET_LINE_IOCTL), toggles the simulated pull and checks that every edge arrives as a line event on an fd that `ppoll` wakes on. Proves the event source a GPIO-chardev INT path would use in place of `/proc/lt_int`
* scripts/forge-build.sh: builds gpioedge (riscv64, no cgo) and publishes it as `emu/gpioedge` beside `emu/NanoKVM-Server` (emulator only, not in the OTA tarball)
* No device code changed

## 2.5.18 (2026-10-26)

### Changes

* libkvm: HDMI detection is one state machine thread (NO_SIGNAL / LOCKING / STREAMING / RES_CHANGE) that blocks in `poll()` on a wake eventfd and a 100 ms INT-drain timerfd instead of a 10 ms loop doing about 200 filesystem operations per second. It alone tears down VENC, then VPSS and VI/CSI, and re-inits them, under `vi_mutex` (the lock a frame read holds), so a restart can no longer free the VI channel mid-read
* libkvm: cable state has one source, the LT6911 read on an INT edge or a resync; the 10 s `/proc/cvitek/vi_dbg` read now only publishes the shared VI state and no longer decides the cable state
* libkvm: removed the `/etc/kvm/hdmi_mode` file and modes 0/1/2, `vi_detect_state`, `auto_change_mode`, the trial-resolution loop and manual-resolution mode. A transient unknown resolution is no longer persisted to `/etc`; a resolution outside the VI limits is reported as -7 and a listed-unsupported one as -6
* libkvm: `kvmv_hdmi_control(0)` waits for an LT6911 read in flight on a mutex instead of spinning on a flag with `sleep`; `kvmv_hdmi_control(1)` asks the state machine to re-read the chip
* libkvm: `kvmv_deinit` stops and joins the state machine before destroying `vi_mutex`
* Not hardware-tested: kvm1 was unreachable

## 2.5.17 (2026-10-26)

### Changes

* libkvm: `debug()` forwards its arguments (`vprintf`); it printed the raw format string and dropped every value
* libkvm: the watchdog and VI-detection thread functions return `NULL` on exit (they fell off the end of a `void*` function)
* libkvm: the post-idle frame discard (`fresh_frame_discard_count`) runs in its own bounded loop and no longer spends the read's try budget (`KVMV_MAX_TRY_NUM`), so the first read after enabling HDMI does not give up with "image not exist" before the stale frames are consumed
* libkvm: the ion carveout check reads `/sys/kernel/debug/ion/.../summary` directly instead of a `popen` of a `cat | grep | awk` pipeline
* Dead code removed: the unreferenced `kvm_system` `lib/hdmi` component, and libkvm's never-called `lt6911_write_edid`, `lt6911_read_edid`, `lt6911_read_fw`, `lt6911_get_hdmi_clk`, `lt6911_get_hdmi_errer`, `sync_vi_res` and the EDID blob

## 2.5.16 (2026-10-26)

### Changes

* One account: `root`. The web login authenticates root against its `/etc/shadow` hash (SHA-512 `$6$`, SHA-256 `$5$` and MD5 `$1$` crypt, pure Go); the separate `admin` account, the multi-user account store, roles and Settings > Account > Users are gone, and so is `/boot/initial.pass`. Changing the password in the web UI sets root's shadow entry, the only password. `/etc/kvm/pwd` now holds only the session generation and the forced-change flag
* Migration from 2.5.14/2.5.15: root's shadow entry is kept as it is (those releases copied the web password into it), the old account file is replaced and other accounts are dropped. If root's hash is not `$6$`/`$5$`/`$1$` the migration is refused and logged, the old file is left, and nothing is changed, so an upgraded device never loses a login it had
* First boot: `/boot/root.pass` sets root's password and is consumed (no change forced). Without it a random password is set, shown on the OLED and kept in `/etc/kvm/initial.pass` (the OLED channel only), and the first web login must change it
* Passwords are bounded at 256 bytes (was bcrypt's 72): crypt hashes thousands of rounds, so the length stays bounded on the unauthenticated login route
* Web: the login form has no username field (the account is root); dead user-management strings removed from every language
* Web: legible Light theme for the Tailscale icon, the Num/Caps pill and the toolbar (27502a7)

## 2.5.15 (2026-10-26)

* bump-only release to exercise OTA on hardware

## 2.5.14 (2026-10-26)

### Features

* First boot: an optional `/boot/root.pass` on the FAT boot partition sets root's password (read like `/boot/initial.pass`: one trailing newline trimmed, non-empty, never logged). If present when the owner is first created, root gets it instead of the admin seed, and the file is removed from `/boot` (and synced) once applied; if it cannot be removed the device logs a SECURITY line. If the shadow file cannot hold it (it contains `:`, a newline or NUL), it is left in place and root gets the admin seed. Absent, root shares the admin seed as before

### Changes

* No password policy: the 8-character minimum and the 12-character boot-seed minimum are gone, in the web UI as well as the server (owner and user create, change, reset, the boot seed and the root password). Any non-empty password is accepted. The two safety bounds stay: an empty password is refused, and one over bcrypt's 72 bytes is refused rather than silently truncated. The forced password change at first login is unchanged

## 2.5.13 (2026-10-26)

### Features

* First boot: an operator-supplied seed. If `/boot/initial.pass` (the FAT boot partition, writable from a Mac) exists when the owner account is first created and holds at least 12 characters (one trailing newline is trimmed), it is the admin seed instead of a random one. It is also set as root's, the first web login still forces a password change, and the file is removed from `/boot` (and synced) as soon as it is consumed. A seed that is too short or unreadable is ignored with a log line (never the value) and the random seed is used; if the file cannot be removed the device carries on and logs it loudly. With no file the random seed and OLED flow is unchanged

## 2.5.12 (2026-10-25)

### Features

* Web: Settings → Appearance chooses Dark (the default, as before), Light or System; System follows the browser's colour scheme and changes with it live. The choice is kept in the browser (`nano-kvm-theme`)
* Settings → Network has a Firewall selector: Open (every port reachable) or Drop (only the KVM's own service ports). It writes `firewall:` in /etc/kvm/server.yaml and applies it with `nanokvm-firewall`; switching to Drop asks first, and if applying fails the previous mode is kept and an error is shown. API: `GET`/`POST /api/vm/firewall` (admin), `{"mode":"open"|"drop"}`

### Bug Fixes

* Web: every language carries the strings added since 2.5.0 (Ethernet panel, users and roles, HID-only, the UI-tour fixes, appearance, firewall) instead of falling back to English
* Web: the Czech and Swedish language codes are the ISO 639-1 `cs` and `sv` (were `cz` and `se`); a stored choice of the old code is migrated once. Danish and Italian are listed by their own names (Dansk, Italiano)

## 2.5.11 (2026-10-25)

### Bug Fixes

* Web: the top menubar fits a phone-width screen (it scrolls sideways when wider than the viewport) so the drag grip and the collapse button stay reachable
* Web: the "WebRTC connection failed" notification closes after 8 seconds, updates one notification on every retry instead of stacking, and sits at the bottom right so it no longer covers the menubar, Settings close button or popover buttons
* Web: the screen shows a muted "Connecting…" or connection-failure message with a hint to switch the video mode, instead of plain black, while there is no H.264 WebRTC stream
* Web: non-antd text (Change Password heading, keyboard lock status) uses the sans-serif font instead of the browser's serif default, and the keyboard lock labels are 10px instead of 8px
* Web: the Delete shortcut key reads "Del" instead of the backspace symbol, and the Scale submenu shows "%" instead of a stray icon
* Web: the Power long-click and Wheel-speed sliders have a visible track and handle on the dark theme, and the Fast/Slow labels have spacing
* Web: the HID-Only dialog shows a translated error and disables the action while the current mode is unknown; the Image Downloader error text has padding; the OLED setting says "Not detected" when there is no OLED
* Web: About no longer shows a dangling "kvm-" / "kvm-.local" hostname when the suffix is empty, and shows "—" for an empty image version
* Web: the virtual keyboard dialog has a hidden title and description (no more Radix accessibility warnings)
* Web: the Cursor submenu marks the selected item with the same blue check as the other submenus
* Web: the empty Images list and Scripts popover say where to put images and how to add a script
* Web: the Ethernet panel validates the address and gateway as you type with a message under the field, keeps Save disabled while the input is invalid or unchanged, aligns the Address and Gateway labels, and wraps the summary line below the DHCP/Static control on narrow screens
* Web: the Users role select is wide enough to show "Administrator"

### Security

* The inbound firewall is open by default: INPUT policy ACCEPT and none of our rules, and a device that already had the default-DROP allowlist is opened on the next `S95nanokvm firewall` or boot. `firewall: drop` in /etc/kvm/server.yaml turns the allowlist back on (then run `/etc/init.d/S95nanokvm firewall`); an unknown value is logged and treated as open

## 2.5.10 (2026-10-25)

### Build

* The qemu emulator no longer starts `kvm_system`, which crashes there: `emu-image.sh` writes `/etc/kvm/emulator` and `S95nanokvm` does not start `kvm_system` when that file exists. Since the supervised `S95nanokvm` (2.5.2), the emulator's line strip could not remove the start from it. A device never has the marker, so nothing changes on hardware

## 2.5.9 (2026-10-25)

### Bug Fixes

* An update is installed by whole-directory renames: the new tree is staged as `/kvmapp.new` and synced, then swapped in. Power lost at any point leaves either the old or the new version, and at boot `S95nanokvm` completes or rolls back an interrupted swap. Previously files were moved one by one, so a power cut could leave `/kvmapp` half old, half new

### Build

* `emu-image.sh --refresh` also installs `/kvmapp/system/init.d/S95nanokvm` as `/etc/init.d/S95nanokvm`, as `kvm_system` does on a device

## 2.5.8 (2026-10-25)

### Bug Fixes

* An update (online or offline) flushes its files to disk before it reports success, so power lost seconds after an update no longer leaves `/kvmapp` with empty files
* An empty `/kvmapp/version` counts as the default version (1.0.0) rather than an unparseable one, so a box left that way can still take an online update; an online update refused as not newer is logged as a warning

## 2.5.7 (2026-10-25)

### Features

* Settings → Network: a static address is entered as one CIDR field (e.g. 192.168.1.20/24) instead of separate address and netmask fields, and the static form opens prefilled with the interface's current address and gateway
* A static address may be a /31 point-to-point link (RFC 3021): both addresses of the pair are usable, and with no gateway given the other address is used
* A static address may be a /32: a gateway outside it is reached through a host route; with no gateway there is no default route

## 2.5.6 (2026-10-25)

### Build

* forge also publishes an emulator build of the server (`-tags nokvm`) and `emu-image.sh` under `<version>/emu/`. `emu-image.sh --refresh` re-applies the emulator edits to an image that has taken an OTA, so the qemu emulator keeps serving after an update. Nothing shipped to devices changes.

## 2.5.5 (2026-10-25)

### Security

* Online update refuses a release that is not newer than the installed version, so an older, genuinely signed package cannot be served as an update. An explicit reinstall or downgrade is `POST /api/application/update?allow_downgrade=1`; an offline upload is still allowed at any version and logs a warning when it is not newer

## 2.5.4 (2026-10-25)

### Bug Fixes

* The network confirm dialog no longer repeats its title in the message

## 2.5.3 (2026-10-25)

### Features

* Settings → Network shows each network interface (Ethernet, Wi-Fi) with the same settings: DHCP or static IPv4 address, netmask and gateway, plus the SSID for Wi-Fi. An interface the device does not have is shown as not present. A change must be confirmed from the device's address within 120 seconds, or the previous settings are restored, including after a reboot.
* Wi-Fi station mode honours a static address in /boot/wlan.nodhcp (same format as /boot/eth.nodhcp), re-applied without dropping the Wi-Fi join

## 2.5.2 (2026-10-25)

### Security

* Default-DROP inbound firewall in its own chain (web, ssh, DHCP, mDNS, SSDP, usb0/AP DHCP+DNS, tailscale0 allowed; built in a scratch chain and swapped in before the policy closes; a failed load leaves the previous state)
* PicoClaw is off by default, runs as the unprivileged `picoclaw` user with a private home in /etc/kvm/picoclaw, and its internal token reaches only HID routes (screenshot, actions, MCP, session, USB recover); shell, file, web, cron and skill tools are disabled in its config and the load-image route is gone
* HDMI frame dimensions are bounded before any copy into encode buffers; bounded string copies in kvm_mmf

### Bug Fixes

* NanoKVM-Server and kvm_system run from /kvmapp under a restarting supervisor, and /dev/watchdog is fed when the board has one
* Each device presents its own USB serial number instead of 0123456789ABCDEF
* First-boot hardware detection is idempotent and reboots PCIe boards at most once, after the result is saved
* Fixed the H.264 encoder copying every NV21 row to the same offset when the stride differs from the width

## 2.5.1 (2026-09-09)

### Features

* Added device-level multi-user support with `admin` and `user` roles and a user management page under Settings, separating administrative areas (terminal, scripts, storage, network, updates, MCP, PicoClaw and Tailscale) from the video, keyboard, mouse, paste, wake-on-LAN and power controls every account keeps
* Moved sessions to HttpOnly cookies with a per-user token version, so changing a password or disabling an account immediately ends that user's open web and WebSocket sessions
* Added support for a custom update server, so the application can be updated from your own mirror, with credentials in the URL, manifest validation and a settings page for configuring it
* Added SHA-256 verification of offline update packages before they are installed
* Added configurable mouse input regions for absolute mode, with automatic black-border detection, manual selection of the controllable area on screen, and original-resolution presets, so the pointer stays aligned when the target machine outputs a letterboxed or non-16:9 signal
* Added touch gestures to relative mouse mode, including tap, drag, long press, double-tap drag and two-finger scroll, so relative mode is now usable from a phone or tablet
* Added a `signal` field to `/api/vm/hdmi` reporting whether an HDMI source is actually present, not just whether capture is enabled
* Added a default root shell profile showing the user, host and working directory in the prompt over SSH and the Web terminal

### Bug Fixes

* Fixed mouse coordinates being offset in Direct H.264 mode when the target machine pads the picture with black bars
* Fixed touch input in absolute mode, and stopped taps on the black letterbox area from being sent to the target machine
* Fixed the video area not fitting the window, by scaling H.264 and MJPEG playback to contain and letting the page handle touch instead of the browser
* Fixed WebRTC H.264 failures leaving a silent black screen; signaling, negotiation, ICE and playback failures now report what went wrong and offer a retry
* Fixed a slow WebRTC viewer stalling the shared H.264 source for everyone else, by dropping its buffered frames until the next key frame
* Fixed application updates running from a temporary directory that could be too small or shared with another update; online and offline updates are now staged in isolated persistent workspaces with archive, manifest and free-space checks, and the last rollback backup is kept when space runs short
* Fixed the update lock being released before the server finished restarting, which allowed a second update to start and be killed mid-install
* Fixed Web terminal sessions not loading the root profile, by starting them as login shells in `/root`

### Performance

* Routed Direct and WebRTC viewers through a single shared H.264 capture source instead of each reading the encoder separately

### Localization

* Synchronized translations of other languages according to English

### Chores

* Upgraded the web build tooling to Vite 8 and switched to Vite's native tsconfig path resolution
* Added a dev container for full-stack development
* Prefixed GitHub Release titles with `nanokvm`
* Added release asset verification with per-device package size limits to the packaging workflow

## 2.5.0 (2026-08-04)

### Features

* Added an MCP service that lets trusted MCP clients capture screenshots and control the keyboard and mouse with an API key, including a settings page for enabling it and regenerating the key
* Added coordinated device control so MCP, PicoClaw and manual input hand ownership over to each other instead of writing HID input at the same time
* Added remote keyboard lock indicators that show the target machine's Num Lock, Caps Lock and Scroll Lock state
* Added an optional SHA-256 checksum and cancellation to remote image downloads

### Bug Fixes

* Fixed the USB network gadget getting a random host-side MAC on every bind, which made the attached PC register a new network adapter after each reboot; both the device and host MAC are now derived from the chip UID (thanks to [@BeaconCat](https://github.com/BeaconCat))
* Fixed OLED sleep durations above 255 seconds being truncated, so the 5 min, 10 min, 30 min and 1 hour options now behave as selected
* Fixed the mounted image API returning an error in HID-Only mode
* Fixed image downloads and application updates interfering with each other when running at the same time
* Fixed Direct H.264 playback falling behind when the decoder queue backed up, by resynchronizing at the next key frame and preferring hardware decoding
* Added a warning when Direct and WebRTC H.264 stream modes are used at the same time
* Made the video input state shared between processes and read it without shell pipelines, so HDMI status stays consistent between `NanoKVM-Server` and `kvm_system`

### Performance

* Reduced latency in 60 Hz mode with decode-driven flow control, GOP-aware bounded queues, and dedicated writer goroutines with write deadlines so a slow client no longer stalls the others
* Integrated zero-copy H.264 capture into the video pipeline instead of relying on a preload hook
* Stopped HDMI capture when no user is logged in or all viewers are idle, reducing power consumption

### UI Improvements

* Tidied the image download dialog layout and truncated long file names in the upload box

### Localization

* Synchronized translations of other languages according to English

### Chores

* Split release automation into separate package, tag and release workflows, and produced identifiable build artifacts with checksums for pull requests
* Allowed overriding the builder image, installed `patchelf` in the build environment, and fixed the exit codes of `support/sg2002/build`
* Fixed slow container builds caused by uid/gid mismatch and reused existing container users and groups

## 2.4.3 (2026-06-09)

### Features

* Added LT6911D support

### Bug Fixes

* Improved USB network adapter compatibility on Windows

## 2.4.2 (2026-05-20)

### Features

* Added HDMI capture status detection with localized warning and error overlays on the desktop screen

### Bug Fixes

* Fixed left-button touch cancellation and context menu cleanup for mouse input
* Improved the WebRTC loading indicator
* Added Wi-Fi password field validation
* Hardened Wake-on-LAN MAC handling by normalizing stored addresses, avoiding shell execution, handling missing history gracefully, and improving named MAC display
* Stabilized PicoClaw session switching by waiting for gateway release, refreshing runtime status before reconnecting, rendering structured thought messages, and preventing history row overflow

### Performance

* Improved H.264 streaming client handling by reducing frame queue latency, using client snapshots for fanout, applying backend ICE server configuration, and cleaning up disconnected WebRTC clients more aggressively

### UI Improvements

* Refreshed desktop menu ordering and community links, including replacing the Discussion link with Discord

### Chores

* Removed the legacy H.264 stream package
* Updated PostCSS, refreshed the MSW worker, and added pnpm workspace build approvals

## 2.4.1 (2026-05-08)

### Features

* Added DNS management to Network settings, including DHCP/manual modes, effective DNS display, network details, IPv4/IPv6 server validation, and persistent udhcpc hook support
* Added a configurable server `host` option for binding NanoKVM to a specific listen address (thanks to [@allmazz](https://github.com/allmazz))
* Added French keyboard layout support and language options (thanks to [@ilyesAj](https://github.com/ilyesAj))

### Bug Fixes

* Hardened HID recovery and cleanup by adding non-blocking HID writes, bounded reopen retries, stale event draining, and release reports after write failures
* Fixed the `kvm_vision` project name and build output message in the SG2002 build script (thanks to [@Voranto](https://github.com/Voranto))

### UI Improvements

* Moved TLS and Wi-Fi settings into the Network settings section
* Optimized the Settings scrollbar style

### Localization

* Updated Korean translations (thanks to [@kmw0410](https://github.com/kmw0410))
* Synchronized translations of other languages according to English

### Security

* Upgraded vulnerable web dependencies

## 2.4.0 (2026-04-10)

### Features

* **Introduced [PicoClaw](https://github.com/sipeed/picoclaw) support** — an AI-powered remote desktop assistant for NanoKVM. PicoClaw seamlessly integrates a lightweight AI agent with NanoKVM's underlying hardware capabilities. Key highlights include:
  * **Zero-Agent Architecture:** Operates entirely through HDMI video capture (vision) and USB HID emulation (keyboard/mouse). No SSH access, network connection to the host, or OS-level software installation is required.
  * **Natural Language Control:** Features a built-in chat interface allowing users to issue complex instructions in plain text.
  * **Autonomous GUI Operation:** The AI agent can autonomously observe the remote host's screen, understand UI elements, reason about the task, and execute operations mimicking human behavior.
  * **Installation Note:** PicoClaw is not built into the NanoKVM device or firmware. Install it separately when needed.

### Bug Fixes

* Added a default subnet mask for static IP configurations
* Fixed an issue where the IP address would occasionally not display

## 2.3.6 (2026-03-12)

### Features

* Implemented AP password authentication for the WiFi configuration page
* Added Korean and Japanese virtual keyboard layouts (thanks to [@klim4-bot](https://github.com/klim4-bot))
* Enabled support for 640x480 resolution (thanks to [@Voranto](https://github.com/Voranto))
* Added encryption parameters for OLED distribution network
* Added custom logo function and logo generation tools
* Added update mechanism for tailscale startup script

### Localization

* Updated Japanese translation (thanks to [@tkmsst](https://github.com/tkmsst))

## 2.3.5 (2026-02-28)

### Features

* Implemented login brute-force protection with lockout mechanism

### Bug Fixes

* Improved Tailscale error handling in UI components and refined backend state mapping

### Chores

* Updated server and web dependencies

## 2.3.4 (2026-01-26)

### Features

* Introduced `Leader Key` functionality to bypass local browser shortcut interception

### Bug Fixes

* Fixed an issue where the Right Shift key was not being recognized on Windows system
* Fixed a bug where the menu bar would fail to close correctly

### Localization

* Updated Traditional Chinese translations (thanks to [@j796160836](https://github.com/j796160836))
* Updated Korean translations (thanks to [@kmw0410](https://github.com/kmw0410))

## 2.3.3 (2026-01-16)

### Features

* Added support for setting the display mode of the menu bar

### Bug Fixes

* Fixed a keyboard issue where IME composition events were not handled correctly
* Resolved an issue where the `AltGr` key was not recognized on the Windows system
* Fixed a bug where `Command` key combinations were not fully released on the macOS system
* Fixed input issues with the `IntlBackslash` key on the German virtual keyboard

### Localization

* Updated Korean translations (thanks to [@kmw0410](https://github.com/kmw0410))

### Chores

* Bump react-router-dom from 6.27.0 to 6.30.3
* Updated eslint

## 2.3.2 (2026-01-08)

### Features

* Added support for custom keyboard shortcuts
* Added support for mouse Forward and Back buttons (reboot required)
* Added support for touchscreen mouse operation
* The menu bar now supports auto-hide and drag

### Bug Fixes

* Fixed an issue where the previous IP address was not released after configuring a static IP

### Performance

* Refactored the keyboard to support simultaneous keystrokes and a wider range of international layouts (reboot required)
* Refactored the mouse to improve response latency
* Refactored the WebSocket module to transmit keyboard and mouse data using the standard HID format

## 2.3.1 (2025-12-26)

### Features

* Added support for offline application updates (thanks to [@Alexander-Ger-Reich](https://github.com/Alexander-Ger-Reich))
* Implemented support for uploading ISO images (thanks to [@Alexander-Ger-Reich](https://github.com/Alexander-Ger-Reich))
* Extended clipboard compatibility to support Russian characters (thanks to [@pekishev](https://github.com/pekishev))
* Added configuration options for video scaling
* Added support for configuring mouse scroll wheel direction

### Bug Fixes

* Resolved an issue where custom port may not be accessible
* Fixed a bug where Web Terminal and Serial Terminal may not disconnect when the page is closed

### Security

* Enhanced validation logic on the Wi-Fi configuration page. Requests are now rejected when the device is not in AP mode to prevent unauthorized changes

### Chore

* Added a Docker image environment to support building `libkvm.so` and `NanoKVM-Server` (thanks to [@lowtech-guy](https://github.com/lowtech-guy))
* Optimized the Settings UI

### Localization

* Updated Korean translation (thanks to [@kmw0410](https://github.com/kmw0410))
* Updated Swedish translation (thanks to [@acidflash](https://github.com/acidflash))
* Updated Spanish translation (thanks to [@Deses](https://github.com/Deses))

## 2.3.0 [0e30a26](https://github.com/sipeed/NanoKVM/commit/0e30a26db42cbc08416662b49a88a5d16ad93424) (2025-11-26)

### Features

* Added an EDID editor in the terminal with a built-in 1080P EDID template
* Added German virtual keyboard and German clipboard support (thanks to [@Alexander-Ger-Reich](https://github.com/Alexander-Ger-Reich))
* Resetting the HID also resets the USB hardware
* Added support for deleting ISO images

### Refactoring & Improvements

* Video (H.264 WebRTC): Refactored the module to significantly reduce video latency
* Video (H.264 Direct): Refactored to optimize data transmission and support data parsing even when the page is in the background
* Video (MJPEG): Refactored and ensure correct data length transmission
* Added locking before resetting USB to prevent HID deadlock (thanks to [@scpcom](https://github.com/scpcom))
* Optimized HID writing logic
* Made HDMI toggle state persist across reboots (thanks to [@tpretz](https://github.com/tpretz))
* Enhanced the paste feature with browser clipboard API integration (thanks to [@patrickpilon](https://github.com/patrickpilon))
* Set the inquiry string for the virtual storage device (thanks to [@scpcom](https://github.com/scpcom))
* Hostname now updates `/etc/hosts` simultaneously and takes effect immediately (thanks to [@scpcom](https://github.com/scpcom))
* Improved compatibility for static IP configuration on Windows
* Optimized the web page title update logic
* Added a confirmation dialog when uninstalling Tailscale
* Improved UI for Clipboard, Image Mounting, and settings pages

### Bug Fixes

* Tailscale: Fixed an issue where Tailscale would turn the device into a router
* Tailscale: Fixed an issue where Tailscale would disable IPv6
* Tailscale: Added a swap memory option to prevent Tailscale Out-Of-Memory errors
* Tailscale: Fixed Tailscale accept-dns configuration (thanks to [@lazydba247](https://github.com/lazydba247))
* Fixed an issue where certain modifier keys were not recognized
* Fixed vertical mouse drift when the page is zoomed in or out

### Localization

* Update Korean translation (thanks to [@xenix4845](https://github.com/xenix4845))
* Update Traditional Chinese translation (thanks to [@protonchang](https://github.com/protonchang))
* Add Brazilian Portuguese translation (thanks to [@chiconws](https://github.com/chiconws) and [@Luccas-LF](https://github.com/Luccas-LF))
* Add Swedish translation (thanks to [@acidflash](https://github.com/acidflash))
* Add Catalan translation (thanks to [@Zagur](https://github.com/Zagur))
* Add Turkish translation (thanks to [@Keylem](https://github.com/Keylem))

### Security

* Implemented a delay after failed login attempts to mitigate brute-force attacks
* Upgraded dependencies to fix known security vulnerabilities

## 2.2.9 [c77981c](https://github.com/sipeed/NanoKVM/commit/c77981cc0ceebd8f6705b6c5d8c3cf4edf4f6717) (2025-06-13)

* fix(security): resolve parameter injection in serial port terminal

## 2.2.8 [01e28f1](https://github.com/sipeed/NanoKVM/commit/01e28f10ae8b581d484bb6077ddfe7bbe4e57919) (2025-05-22)

* feat: add AZERTY virtual keyboard Layout (thanks to [@felix068](https://github.com/felix068))
* feat: add support for enabling/disabling HDMI output (PCIe version only) (thanks to [@tpretz](https://github.com/tpretz))
* feat: add support for custom mouse wheel speed
* fix: prevent direct H.264 stream buffer overflow and replay issues
* perf: improve keyboard paste performance (thanks to [@ethanperrine](https://github.com/ethanperrine))
* Localization
  * update Korean translation (thanks to [@kmw0410](https://github.com/kmw0410))
  * update Ukrainian translation (thanks to [@arbdevml](https://github.com/arbdevml))
  * update Russian translation (thanks to [@arbdevml](https://github.com/arbdevml))

## 2.2.7 [e18ec22](https://github.com/sipeed/NanoKVM/commit/e18ec2219d22886529575d1fdaad5c320e05f5b2) (2025-05-08)

* feat: add HTTPS support
* feat: support direct H.264 streaming over HTTP
* Localization
  * update Russian translation (thanks to [polyzium](https://github.com/polyzium))
  * update Dutch translation (thanks to [LeonStraathof](https://github.com/LeonStraathof))
  * update Ukrainian translation (thanks to [click0](https://github.com/click0))
  * update German translation (thanks to [3limin4tor](https://github.com/3limin4tor))

## 2.2.6 [c83dc55](https://github.com/sipeed/NanoKVM/commit/c83dc5565c9dbed22336661a8832edbd93a06d11) (2025-04-17)

* feat: add mouse jiggler to prevent system sleep
* feat: add support for swap memory
* feat: add support for customizing the device hostname
* feat: add support for customizing the web page title
* feat: add support for assigning custom names to Wake-on-LAN MAC addresses
* feat: add confirmation prompts for power operations
* feat: add logo to the login page (thanks to [S33G](https://github.com/S33G))
* fix: fix possible privacy issues with MIC drivers. [Repair Record](https://github.com/sipeed/NanoKVM/commit/f9244b36df090a05cd59ba11ea4fd01e9b638995)
* fix: fix iptables rule that could interfere with SSH connections (thanks to [scpcom](https://github.com/scpcom))
* fix: fix the static IP gateway configuration might not apply correctly (thanks to [xitation](https://github.com/xitation))
* perf: optimized OLED display handling and sleep logic
* perf: improve H.264 streaming reliability by adding a heartbeat mechanism
* perf: set the minimum screen size to 640x480
* perf: add display of both wired and wireless IPv4 addresses in the settings page
* perf: update the Thai language translations (thanks to [ChokunPlayZ](https://github.com/ChokunPlayZ))
* chore: bump axios to 1.8.4
* chore: bump golang.org/x/net to v0.39.0
* chore: bump github.com/golang-jwt/jwt/v5 to to v5.2.2

## 2.2.5 [3286cc2](https://github.com/sipeed/NanoKVM/commit/3286cc2f85a14133d65935cb476c833dcf151459) (2025-03-26)

* fix: server crash caused by MJPEG frame detection error
* feat: add HID-Only mode
* feat: support preview updates
* perf: improve image reading performance by optimizing screen parameters

## 2.2.4 [1bf986d](https://github.com/sipeed/NanoKVM/commit/1bf986d41b34d568c1ffee5df90ce61b6b08456b) (2025-03-21)

* fix: resolve USB initialization issue
* fix: correct abnormal updates in certain models
* perf: add version restrictions for production testing

## 2.2.3 [6ef83cb](https://github.com/sipeed/NanoKVM/commit/6ef83cb22fcd77f721d32c97c85d12f2bfc3035a) (2025-03-21)

* feat: add support for setting H.264 GOP
* fix: resolve deadlock caused by HDMI resolution errors
* perf: merge H.264 SPS and PPS into I-frame
* perf: refactor MJPEG frame detection
* perf: added more configuration options to the serial port terminal (thanks to [@mekaris](https://github.com/mekaris))
* perf: improve the `update-nanokvm.py` script (thanks to [@reishoku](https://github.com/reishoku))
* perf: disable mDNS by default in new products
* perf: update log timestamp to millisecond precision
* chore: bump Go to 1.23
* chore: bump `golang.org/x/net` to v0.37.0

## 2.2.2 [58d5ab2](https://github.com/sipeed/NanoKVM/commit/58d5ab2d37244b1e1a68b925a5c23c324c489ad3) (2025-03-11)

* feat: add watchdog for NanoKVM-Server
* feat: add support for UE chip
* feat: support system reboot
* feat: support enable/disable mDNS
* fix: resolve UE chip cannot start server
* perf: refactor automatic resolution detection
* perf: add output prompt for unsupported resolutions
* perf: add lock to kvmv_read_img
* perf: configurable VENC automatic recycling feature
* perf: add maximum limit for vi
* perf: add tooltips to menu bar (thanks to [@S33G](https://github.com/S33G))
* perf: menu bar is now draggable (thanks to [@forumi0721](https://github.com/forumi0721))
* perf: image list support auto-refresh (thanks to [@forumi0721](https://github.com/forumi0721))
* perf: update translations

## 2.2.1 [b5e48a0](https://github.com/sipeed/NanoKVM/commit/b5e48a07e82df3aedd60442342ae50b95684a697) (2025-02-21)

* fix: mounted image were not being detected correctly
* perf: add support for CD-ROM mode when mounting image (thanks to [@scpcom](https://github.com/scpcom))
* perf: add a loading state during login
* perf: add changelog link in settings
* perf: update translation and cleanup the code (thanks to [@ChokunPlayZ](https://github.com/ChokunPlayZ) [@Stoufiler](https://github.com/Stoufiler) [@polyzium](https://github.com/polyzium) [@Jonher937](https://github.com/Jonher937) [@S33G](https://github.com/S33G))

## 2.2.0 [0dbf8c0](https://github.com/sipeed/NanoKVM/commit/0dbf8c007f2d0183d0f0601c3da6d3c3fccd8b31) (2025-02-17)

NanoKVM [Image v1.4.0](https://github.com/sipeed/NanoKVM/releases/tag/v1.4.0) has been released!

> Please refer to the [wiki](https://wiki.sipeed.com/hardware/en/kvm/NanoKVM/system/introduction.html) for more details about the image and application.

* fix: improve password update notification logic (thanks to [@li20034](https://github.com/li20034))
* perf: increase update wait time to 10s (from 6s)
* perf: update Korean translation (thanks to [@forumi0721](https://github.com/forumi0721))
* perf: update Traditional Chinese translation (thanks to [@protonchang](https://github.com/protonchang))
* refactor: update `libkvm.so` and `libkvm_mmf.so` libraries

## 2.1.6 [6eb4a4e](https://github.com/sipeed/NanoKVM/commit/6eb4a4ea6254f465a47f9881d13934c686649061) (2025-02-14)

* feat: support downloading image from online URL (thanks to [@Itxaka](https://github.com/Itxaka))
* feat: add keyboard shortcut `Ctrl+Alt+Del` (thanks to [@CaffeeLake](https://github.com/CaffeeLake))
* fix: fix the CSRF issue
* perf: add an option to configure custom ICE servers (thanks to [@VMFortress](https://github.com/VMFortress))
* perf: removed unnecessary modifications to DNS configuration
* perf: add an SSH enable/disable toggle in the web UI
* perf: add a Tailscale enable/disable toggle in the web UI
* perf: download Tailscale installation package from the official source
* perf: automatic enable/disable GOMEMLIMIT on tailscale start/stop
* perf: add JWT configuration
  * secretKey: customize secret key. If empty, generated a random 64-byte secret key by default
  * refreshTokenDuration: customize token expiration time
  * revokeTokensOnLogout: invalidate all JWT tokens on logout
* perf: implement secure password storage using bcrypt hashing
* perf: implement integrity checks for online updates
* refactor: refactor HDMI module and remove the dependency `libmaixcam_lib.so`
* refactor: web terminal use pty instead of SSH
* refactor: move Tailscale APIs from the `network` module to the `extensions` module

## 2.1.5 [85f6447](https://github.com/sipeed/NanoKVM/commit/85f6447a16cc2591c6459b7d3dfda4d4cb75e98c) (2025-01-14)

* feat: add HDMI reset for NanoKVM-PCIe
* fix: remove unnecessary lock acquisition during HID reset
* refactor: refactor Tailscale

## 2.1.4 [d7ca7c4](https://github.com/sipeed/NanoKVM/commit/d7ca7c453d821ad099bf79b463969419041279cb) (2025-01-10)

* feat: support configuring OLED sleep settings
* feat: support setting the `GOMEMLIMIT` environment variable
* fix: fix Wi-Fi configuration
* perf: password changes now update both the web user and the system root user
* perf: add MAC address verification for Wake-on-LAN
* refactor: a lot update to web UI
* refactor: refactor Tailscale

## 2.1.3 [26078fe](https://github.com/sipeed/NanoKVM/commit/26078fe46e43d4543d7b09901b4992e4fbe4f01f) (2024-12-27)

* feat: add API to retrieve Wi-Fi information
* fix: fix keyboard modifier keys
* fix: update keyboard and mouse HID codes
* fix: update hardware version information

## 2.1.2 [5a39562](https://github.com/sipeed/NanoKVM/commit/5a39562f2d32695933f4e7e86866136236cc9903) (2024-12-04)

* feat: add hardware version to configuration
* feat: add Wi-Fi configuration support for NanoKVM-PCIe
* perf: update web UI
* chore: add dependency libraries

## 2.1.1 [74a303b](https://github.com/sipeed/NanoKVM/commit/74a303bd5cbb58f9d8ddd81abaaf4919dbbfb71b) （2024-11-06）

* feat: support h264
