# maixlite

`kvm_system` (the OLED and button daemon, `support/sg2002/kvm_system`) built without MaixCDK.

This is a **trimmed vendor copy** of [Sipeed/MaixCDK](https://github.com/sipeed/MaixCDK) at
commit `30f4b8b7e3f66ded9cfa64fb081b46e30bc27248` (the `MAIXCDK_COMMIT` of `docker/Dockerfile`),
holding only what `kvm_system` compiles: the `basic`, `peripheral`, `ini`, `ntp_client` and
`yaml` (yaml-cpp) components, platform maixcam, plus the header directories on their include
paths. 153 files plus MaixCDK's `LICENSE` (Apache-2.0). yaml-cpp (MIT) and ntp_client keep
their own `LICENSE` files next to their sources. The files are unmodified upstream bytes;
`UPSTREAM.sha256` lists the sha256 of each (`sha256sum -c UPSTREAM.sha256` from this
directory), and `scripts/build-maixlite.sh` checks it before every build. Each is fetched
from `https://raw.githubusercontent.com/sipeed/MaixCDK/<commit>/<same path>`.

What is ours, not upstream:

| file | what |
| --- | --- |
| `CMakeLists.txt`, `toolchain.cmake`, `main/CMakeLists.txt`, `components/*/CMakeLists.txt`, `components/3rd_party/*/CMakeLists.txt` | plain CMake with the sources, defines, include order, flags and link line MaixCDK's CMake produced for `kvm_system` |
| `config/global_config.h`, `config/global_build_info_version.h` | static stand-ins for the two headers MaixCDK generates (`PROJECT_ID`, `PLATFORM`; fixed version numbers, no date or hash) |

`components/peripheral/port/maixcam/maix_i2c.cpp` is not vendored: upstream has none at this
commit, and the one `kvm_system` has always compiled is NanoKVM's own,
`support/sg2002/additional/peripheral/port/maixcam/maix_i2c.cpp`, which `CMakeLists.txt` uses
in place. `kvm_system`'s own sources stay in `support/sg2002/kvm_system/main` and
`support/sg2002/additional/kvm/include` supplies its headers.

The build is held to MaixCDK's: `tools/binabi/check.sh` compares the binary with the golden
manifest and `tools/binabi/flags.sh` compares every generated `flags.make` and `link.txt`
with `tools/binabi/golden/kvm_system.flags`. `libkvm.so` and `libkvm_mmf.so` still build
through MaixCDK.

## Upstream files

Every path below is relative to this directory; the hashes are in `UPSTREAM.sha256`.

- `LICENSE`
- `components/basic/include/*` (16 headers), `components/basic/src/*.cpp` (9),
  `components/basic/port/maixcam/*.cpp` (3)
- `components/peripheral/include/*` (12 headers), `components/peripheral/port/linux_common/*`
  (9), `components/peripheral/port/maixcam/*` (8; `maix_gpio_port.hpp` and seven sources)
- `components/3rd_party/ini/inifile2/src/inifile.{cpp,h}`
- `components/3rd_party/ntp_client/ntp_client/*` (`ntp_client.{c,h}`, `xtime.{c,h}`,
  `xtypes.h`, `LICENSE`)
- `components/3rd_party/yaml/yaml-cpp/LICENSE`, `include/yaml-cpp/**` (37 headers),
  `src/*` (29 sources and their private headers)
