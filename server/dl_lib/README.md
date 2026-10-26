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
