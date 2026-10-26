# server/dl_lib

Everything here is a vendor SDK blob with no source in this tree. Every library
we can build is built by forge and is NOT tracked; `scripts/build-in-container.sh`
stages them into this directory (untracked) for the server link, and
`scripts/package.sh` ships them from the build, refusing a package that lacks one.

## Built from source (not in git)

| Library | Source |
|---|---|
| `libkvm.so` | `support/sg2002/additional/kvm` + `vision` (MaixCDK build, `./build kvm_vision`); embeds FreeType 2.13.2 and HarfBuzz 8.2.1 built by MaixCDK from its pinned sources |
| `libkvm_mmf.so` | `support/sg2002/additional/kvm_mmf` (same build) |
| `libopencv_core.so.409`, `libopencv_imgproc.so.409`, `libopencv_imgcodecs.so.409` | OpenCV 4.9.0 source tarball, sha256-pinned, `scripts/build-deps.sh` (libjpeg-turbo, libpng, zlib are OpenCV's bundled copies) |
| `libjson-c.so.5` | json-c 0.15-20200726 source tarball, sha256-pinned, `scripts/build-deps.sh` |

## Tracked vendor SDK libraries (no source path here)

| Library | What it is |
|---|---|
| `libae.so libaf.so libawb.so libisp.so libisp_algo.so libcvi_bin.so libcvi_bin_isp.so libcvi_ispd2.so libisp_algo` | Sophgo/CVITEK ISP and 3A (closed) |
| `libsys.so libvdec.so libvenc.so libvpu.so libmipi_tx.so libmisc.so libosdc.so libini.so libcli.so libraw_dump.so` | Sophgo/CVITEK middleware (system, video codec, MIPI, OSD) |
| `libcvi_audio.so libcvi_VoiceEngine.so libcvi_vqe.so libcvi_ive.so libcvi_ssp.so libcvi_RES1.so libdnvqe.so libaac*.so libtinyalsa.so` | Sophgo/CVITEK audio, IVE and voice engine; `libtinyalsa.so` is Sophgo's modified tinyalsa (auxiliary-PIC stub, no SONAME), not upstream |
| `libopencv_highgui.so.409` | MaixCDK's prebuilt OpenCV highgui; nothing in the package links it |

Dropping the never-loaded entries is journey step 01k5rem2v3; the ISP/3A objects
are 01k5reln6l.
