# NanoKVM Support Instructions

NanoKVM-Lite/Full/PCIe is based on the SG2002 as the main control chip. The programs in the
support section are built with plain CMake and the Sophgo riscv64 musl cross compiler
(`riscv64-unknown-linux-musl-gcc`, in the builder image or on forge). MaixCDK is not used and
nothing is downloaded at build time.

## What is here

- `kvm_system/main`: the `kvm_system` daemon. It monitors the NanoKVM system status, handles
  system updates, the screen key drivers and a few system functions.
- `additional/kvm`, `additional/kvm_mmf`, `additional/vision`: the sources of `libkvm.so` and
  `libkvm_mmf.so`, the image acquisition and encoding subsystem the Go server calls.
- `additional/peripheral`: NanoKVM's own `maix_i2c.cpp`.
- `maixlite/`: the CMake projects that build all of the above (a trimmed vendor copy of the
  MaixCDK sources they compile; see `maixlite/README.md`).

## Building

`kvm_system`, `libkvm.so` and `libkvm_mmf.so` are built by `scripts/build-maixlite.sh`, which
`scripts/build-in-container.sh` runs as part of `make release-build` (docker) and
`scripts/forge-build.sh` (forge). See `scripts/README.md` and `maixlite/README.md`.
