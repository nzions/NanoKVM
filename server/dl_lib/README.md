# server/dl_lib

Everything tracked here is a vendor SDK blob with no source in this tree. Every library
we can build is built by forge and is NOT tracked; `scripts/build-in-container.sh`
stages them into this directory (untracked) for the server link, and
`scripts/package.sh` ships them from the build, refusing a package that lacks one.

## Built from source (not in git)

| Library | Source |
|---|---|
| `libkvm.so` | `support/sg2002/additional/kvm` + `vision` (MaixCDK build, `./build kvm_vision`); embeds FreeType 2.13.2 and HarfBuzz 8.2.1 built by MaixCDK from its pinned sources |
| `libkvm_mmf.so` | `support/sg2002/additional/kvm_mmf` (same build) |
| `libopencv_core.so.409`, `libopencv_imgproc.so.409`, `libopencv_imgcodecs.so.409` | OpenCV 4.9.0 source tarball, sha256-pinned, `scripts/build-deps.sh` (libjpeg-turbo, libpng, zlib are OpenCV's bundled copies) |

## Tracked vendor SDK libraries (no source path here)

Exactly the dynamic-link closure of `libkvm.so` (`readelf -d` NEEDED, transitively):
`libkvm.so` -> `libkvm_mmf.so` -> the libraries below.

| Library | What it is |
|---|---|
| `libae.so libaf.so libawb.so libisp.so libisp_algo.so libcvi_bin.so libcvi_bin_isp.so` | Sophgo/CVITEK ISP and 3A (closed); step 01k5reln6l |
| `libsys.so libvdec.so libvenc.so libvpu.so libini.so` | Sophgo/CVITEK middleware (system, video codec, ini parser) |

## Removed (2.6.18, step 01k5rem2v3)

Measured on the 2.6.17 package: no ELF in it NEEDs these and none of the remaining
libraries or the server / `kvm_system` carries a `dlopen` for them (`libopencv_core`
has a generic dlopen for its own plugins only). They are neither tracked nor shipped:

* audio: `libcvi_audio libcvi_VoiceEngine libcvi_vqe libdnvqe libaacdec2 libaacenc2 libaaccomm2 libaacsbrdec2 libaacsbrenc2 libtinyalsa`
* `libcli` (network CLI with a root login prompt), `libcvi_ispd2` (ISP tuning daemon, binds a socket; its only dependency was json-c)
* `libcvi_ssp libosdc libraw_dump libmipi_tx libmisc libcvi_RES1 libopencv_highgui.so.409`
* `libjson-c.so.5`: only `libcvi_ispd2` NEEDed it, so its forge build is gone from `scripts/build-deps.sh`
* `libcvi_ive.so`: `kvm_mmf.cpp` has an IVE colour-space path (`cvi_rgb2nv21`, `cvi_ive_init`), but nothing calls it
  (`cvi_ive_init`/`deinit` are only `UNUSED()`-ed, `cvi_rgb2nv21` has no caller), so `libkvm_mmf.so` has no undefined
  `CVI_IVE_*` symbol and does not NEED the library.

`scripts/package.sh` also prunes these names from the staged tree (`DROPPED_LIBS`), in case
MaixCDK's dist stages any of them.

## Source availability (per middleware pin)

Inventory of the 12 libraries `libkvm_mmf.so` links (`support/sg2002/additional/kvm_mmf/CMakeLists.txt`),
read from a checkout of each candidate pin of `scpcom/sophgo-middleware` (step 01k5spsan1).
Both pins were reachable by their exact commit: `4e231ef5967f` (branch `licheervnano`, tree rooted
at `v2/`) and `cd8bb74c94e1` (branch `maix_mmf-cvisdk`, tree rooted at the repo top).
Classes: FULL = Makefile plus the `.c` that produce it; BLOB = only prebuilt `.a`/`.so` in git, no source.
No library is PARTIAL (no tracked `.o`; the only `.o`/`.a` in either tree are the ISP blobs).
Paths are relative to the pin's root (`v2/` for `4e231ef`). Build system everywhere: plain GNU make
(`Makefile.param`, `modules/Makefile` order `sys bin vpu ... venc vdec ... isp`), no CMake.

| Library | `4e231ef` (licheervnano) | `cd8bb74` (maix_mmf-cvisdk) | Notes |
|---|---|---|---|
| `libsys` | FULL `modules/sys/{Makefile,src/*.c}` (10 files, minus `cvi_rpc.c`) | FULL, same layout | needs only libc |
| `libcvi_bin` | FULL `modules/bin/src/*.c` (7 files) | FULL, same layout | pulls `libcvi_json-c` + `libcvi_miniz`, built from OSS tarballs by the Makefile (tarball fetch, `cd8bb74` into `modules/bin/prebuilt/$(SDK_VER)`) |
| `libvdec` | FULL `modules/vdec/src/cvi_vdec.c` (272 lines) | FULL, identical file | thin `ioctl` shim to the kernel vcodec driver; no codec in userspace |
| `libvenc` | FULL `modules/venc/src/cvi_venc.c` (695 lines) | FULL (718 lines) | thin `ioctl` shim (`CVI_VC_*`, `linux/cvi_vc_drv_ioctl.h`); links `libsys` (+ `libcli` only if `CLI_DEBUG_SUPPORT=1`) |
| `libvpu` | FULL `modules/vpu/src/*.c` (14 files: gdc, region, vi, vo, vpss, ioctls, `dump_register.c`) | FULL, but split: `modules/vpu/Makefile` compiles `../{gdc,rgn,vi,vo,vpss}/src/*.c` | undefined `CVI_ISP_*` (e.g. `CVI_ISP_GetVDTimeOut`, `dump_register.c:221`; also `cvi_vi.c`, `vi_ioctl.c`) resolve into closed `libisp` |
| `libini` | FULL `3rdparty/inih/{ini.c,Makefile}` -> `libini.so` | FULL, same (Makefile lacks the `install` step) | upstream inih |
| `libisp` | BLOB `modules/isp/lib/{32bit,musl_riscv64}/libisp.{a,so}`; `modules/isp/Makefile` only `cp`s them | FULL source `modules/isp/cv181x/isp/{Makefile,src/*.c}` (50 `.c`; `cv180x/isp` is a symlink to it); builds `libisp.{a,so}` | `cd8bb74` is the only pin with ISP register/control source; it still links the closed 3A blobs below |
| `libcvi_bin_isp` | BLOB `modules/isp/lib/*/libcvi_bin_isp.{a,so}` | FULL source `modules/isp/cv181x/isp_bin/{Makefile,src/*.c}` (4 `.c`) | |
| `libisp_algo` | BLOB (`modules/isp/lib/*/`) | BLOB `modules/isp/cv181x/{32bit,64bit,glibc_riscv64,musl_riscv64}/libisp_algo.{a,so}`; `cv181x/isp_algo/` holds headers only | closed; the `modules/isp/Makefile` copies the blob for `$(SDK_VER)` |
| `libae` | BLOB | BLOB (same dirs as `libisp_algo`; `modules/isp/algo/ae/` headers only) | closed 3A (step 01k5reln6l) |
| `libaf` | BLOB | BLOB | closed 3A |
| `libawb` | BLOB | BLOB | closed 3A |

Summary: `4e231ef` is source for 6 of 12 (sys, cvi_bin, vdec, venc, vpu, ini) and blob for 6 (all ISP/3A).
`cd8bb74` is source for 8 of 12 (adds `libisp` and `libcvi_bin_isp`) and blob for 4 (`libisp_algo`, `libae`,
`libaf`, `libawb`). Only the 3A trio plus `libisp_algo` is closed in the `cd8bb74` tree, and every open
`libvpu`/`libisp` symbol that needs them resolves at link time against those four blobs.
Other ISP-side libraries in both trees (`libcvi_ispd2`, `libraw_dump`, `libraw_replay`, `libjson-c`)
are blobs in `4e231ef` and source under `modules/isp/common` / `isp-daemon2` in `cd8bb74`; none is linked here.

### libvi and libvpss

Neither is a separate library at either pin and neither is in the link list: VI, VO, VPSS, GDC and region
(`cvi_vi.c`, `cvi_vo.c`, `cvi_vpss.c`, `cvi_gdc.c`, `cvi_region.c`) are all compiled into `libvpu.so`
(`4e231ef`: `modules/vpu/src/`; `cd8bb74`: `modules/{vi,vpss,vo,gdc,rgn}/src` via `modules/vpu/Makefile`).
Their per-directory Makefiles exist at `cd8bb74` but only feed `libvpu`.

### Not inspected

MaixCDK at `MAIXCDK_COMMIT=30f4b8b7` (`docker/Dockerfile:67`) is not on this box and was not cloned; the
table covers the two middleware pins only, so which blobs MaixCDK's `v2/lib` carries is unverified here.
Source availability is not buildability: nothing was compiled.
