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
