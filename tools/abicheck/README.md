# abicheck

A link does not prove ABI equality. This checks the userspace the NanoKVM
`kvm_mmf` path uses (the sophgo-middleware libraries: libsys, libvpu, libvenc,
libvdec) against the kernel uapi headers the 7.3 osdrv port is built from.

Plain C11 + POSIX sh + awk + readelf. Not wired into forge yet (the middleware
fetch for forge is not built); run it by hand.

## Run

    make check OSDRV_V2_TGZ=osdrv.tar.gz OSDRV_V1_TGZ=osdrv2.tar.gz \
               MW_4E231EF=<clone of scpcom/sophgo-middleware @4e231ef> \
               MW_CD8BB74=<clone of scpcom/sophgo-middleware @cd8bb74>

or `./abicheck.sh --help`-style flags directly (see the top of `abicheck.sh`;
`--osdrv-v2 DIR` / `--osdrv-v1 DIR` take already-unpacked `interdrv/v2` and
`interdrv` trees instead, and are reported as UNVERIFIED). Results land in
`./out` (`probe-*.txt`, `layout-*.txt`, the diffs, `srccompat-*`, `enums-*`,
`boundary-*`). Exit 0 = every pin given is compatible, 1 = mismatch, 2 = bad input.

CC defaults to `cc`. There is no riscv64 cross compiler in the rook container or
on the box checked (beefy.local, nothing on PATH), so everything runs on the
host (aarch64, LP64). That is sound for these headers: the structs are LP64 with
natural alignment on aarch64 and riscv64 alike, `probe.c` asserts LP64, and
layout is read from compile output. It is not a riscv64-musl compile; run it
with `CC=riscv64-linux-musl-gcc` where that exists (the layout stage only
compiles, the probe stage also runs the binary: use qemu-user or compare only
`layout`).

## Source of record (what the 7.3 kernel compiles)

`fleet/containers/kernel-nanokvm-sg2002` (Makefile, `scripts/vendor-build.sh`,
`.build.json`) fetches two tarballs, applies `patches/*.patch` (v2) and
`patches-v1/*.patch` (v1), and builds the drivers built-in:

| tarball | pin | sha256 | drivers built from it |
|---|---|---|---|
| `osdrv.tar.gz` | scpcom/sophgo-osdrv `cbd9c582` (`interdrv/v2`) | `fb40b511...3d14` | base sys rtos_cmdqu fast_image cif snsr_i2c vi vpss dwa rgn ive vo |
| `osdrv2.tar.gz` | sophgo/osdrv `1b3a4964` (`interdrv`, "v1") | `9913c091...43f3` | vcodec cvi_vc_drv jpeg |

Chip: `CHIP_CODE=mars CHIP_ARCH=CV181X`; the v2 tree reaches its chip dir as
`include/chip/mars`, a symlink to `cv181x` (checked by the script).
The uapi headers (`include/common/uapi/linux`, `include/chip/*/uapi/linux`) are
**not touched by any patch** (checked: no patch hunk path contains `include/` or
`uapi` for a header outside driver-private dirs), so the pristine tarball
headers are exactly what the kernel build compiles. `abicheck.sh` unpacks the
tarballs itself and refuses any other sha256. So there are TWO kernel header
sets, and a subsystem is judged against the one its driver is built from:
**venc/vdec against v1, everything else against v2.**

## The middleware pins carry no uapi headers

Neither pin has a `uapi/` directory (kvm_mmf's CMake passes `v2/uapi`, which does
not exist at `4e231ef`; the clones are depth 1). libsys/libvenc/... `#include
<linux/cvi_*.h>` from the kernel headers they were built against, and those are
not in the pin. So the *userspace side of each pin* cannot be compared header to
header. What the tool does instead:

1. **srccompat**: compile the pin's ioctl-issuing sources (`-fsyntax-only`)
   against each kernel header set. Every macro, struct and field the library
   names must exist. Whichever set a pin compiles clean against is the
   generation it was written for.
2. **enum values**: a name present in both sets can carry a different value
   (the `VI_SDK_*` / `RGN_SDK_*` sub-command numbers are shifted between v1 and
   v2 and compile fine). Every enumerator the pin's ioctl sources name is
   compared, pin generation vs the kernel set for that subsystem.
3. **vc_drv boundary**: for a v2-generation pin, every type/enumerator the
   venc/vdec ioctls carry that differs between the v2 and v1 headers.
4. **mirror**: `sys_ion_data2` (declared by hand in the middleware's `cvi_sys.h`,
   passed to `SYS_ION_FREE`) vs the kernel's `sys_ion_data`, `_Static_assert`ed.

Plus the pin-independent kernel-vs-kernel comparison: `probe.c` (ioctl request
value, dir/type/nr/size; `sizeof`/`offsetof` of the hot structs) and
`layout.sh` (every struct, union, typedef and enumerator, from DWARF, so no
hand-kept field list) for v2 vs v1.

Not covered: kernel-internal (kapi) structs shared between the v1 vc_drv and the
v2 base/vb/sys drivers (they link, but their layouts are not compared; needs a
kernel-side compile); the prebuilt `.so` blobs (no header ties them to a layout);
ISP/3A and sensor ioctls (libisp is closed source at both pins; the lt6911
sensor code is in kvm_mmf's own tree).

## The KVM path's user/kernel surface

Device nodes (middleware opens, driver registers): `/dev/cvi-base`, `cvi-sys`
(ION alloc/free, cache flush/invalidate, bind), `cvi-vi`, `cvi-vpss`, `cvi-vo`,
`cvi-rgn`, `cvi-dwa` (via libvpu), `cvi-ive`, `cvi-tpu0`, `cvi-mipi-rx`,
`cvi_vc_enc` / `cvi_vc_dec` (`/dev/%s%d`, venc/vdec), `/dev/i2c-N`, `/dev/mem`.
Two of these have no driver in the 7.3 build: `/dev/ion` (libsys opens it for
`CVI_SYS_IonGetFd`; the port replaced ION with dma-heap and registers no
`/dev/ion`) and `cvi-tpu0` (tpu is not in `VENDOR_DRIVERS`; `CVITPU_SUBMIT_PIO`
is issued by `CVI_SYS_TDMACopy` only).

Requests: `ioctls.def` is the full list (147 names) generated from the `ioctl()`
callsites of both pins: sys (`SYS_ION_ALLOC/FREE`, `SYS_CACHE_FLUSH/INVLD`,
`SYS_{SET,GET}_BINDCFG`, `SYS_IOC_{G,S}_CTRL`), base (`IOCTL_VB_CMD`,
`IOCTL_READ_CHIP_*`), per-subsystem control multiplexers (`VI_IOC_*_CTRL`,
`VO_IOC_*_CTRL`, `RGN_IOC_*_CTRL`: the sub-command is an enum id in the payload,
hence the enum-value stage), vpss (`CVI_VPSS_*`, 42), dwa (`CVI_DWA_*`, 7) and
the vc driver (`CVI_VC_VENC_*` 60, `CVI_VC_VDEC_*` 17: `_IO()` with no size, so
the request number does not encode the struct size and only the layout stage
protects those).

Note `kvm_mmf.cpp` itself does not use `sys_ion_data` or any `*_new` struct (there
is no `sys_ion_data_new` anywhere in the pins, kernel or compat); it goes through
`CVI_SYS_*`/`CVI_VB_*`/`CVI_VPSS_*`/`CVI_VENC_*`. The ION structs are libsys's.

## Results (host aarch64, gcc 14; both pins as named above)

Kernel header sets, v2 vs v1, KVM-path ioctls: identical except the three
`CVI_VC_VENC_{ENABLE,SET,GET}_SVC*` requests (v1 only), `CVI_DWA_*` and
`CVI_VPSS_{SET,GET}_CHN_LDC` (argument size differs). Full layout diff: 610 lines.

**4e231ef** (v2-generation): sys/vpu sources compile clean against v2, venc/vdec
against v1; 213 enumerators identical; `sys_ion_data2` mirror matches. One
mismatch, on the vc_drv boundary: `USER_FRAME_INFO_S` 184 (v2) vs 192 (v1) and
`USER_RC_INFO_S` 32 vs 40 (layout shifted by the new `bRoiBinValid`/`roideltaqp`),
carried by `CVI_VC_VENC_SEND_FRAMEEX`. `kvm_mmf.cpp` does not call
`CVI_VENC_SendFrameEx`. `CVI_ERR_VENC_BUTT` also differs (error-code range only).

**cd8bb74** (v1-generation): venc/vdec compile clean against v1 (same set as
cvi_vc_drv). Against the v2 drivers it mismatches: `VB_IOCTL_GET_VB_INIT` and
`VI_SINGEL_FRAME_ENABLE` do not exist in v2; `RGN_SDK_SET_CHN_PALETTE` is 11 vs
12; and 16 `VI_SDK_*` sub-commands are off by one (e.g. `VI_SDK_GET_CHN_FRAME`
22 vs 21, `VI_SDK_ATTACH_VB_POOL` 34 vs 33) - the same numbers mean different
vi operations, so every vi sub-command with id >= 21 reaches the wrong operation
(and `VO_OUTPUT_*` values differ).

Verdict: `4e231ef` is the ABI-compatible pin for the 7.3 driver set; `cd8bb74` is
not (vi/vb/rgn sub-command ids), unless the vi/vb/rgn drivers are rebuilt from the v1 headers.
