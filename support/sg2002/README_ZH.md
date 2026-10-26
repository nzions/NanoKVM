# NanoKVM support 说明

NanoKVM-Lite/Full/PCIe 以 SG2002 为主控芯片。support 部分的程序使用纯 CMake 和 Sophgo riscv64 musl 交叉编译器（`riscv64-unknown-linux-musl-gcc`，位于构建镜像或 forge 中）编译，不再使用 MaixCDK，构建时也不会下载任何内容。

## 目录说明

- `kvm_system/main`：`kvm_system` 守护进程，负责 NanoKVM 系统状态监控、系统更新、屏幕按键驱动和少部分的系统功能。
- `additional/kvm`、`additional/kvm_mmf`、`additional/vision`：`libkvm.so` 和 `libkvm_mmf.so` 的源码，即供 Go 调用的图像获取编码子系统。
- `additional/peripheral`：NanoKVM 自己的 `maix_i2c.cpp`。
- `maixlite/`：编译以上所有内容的 CMake 工程（MaixCDK 中所需源码的精简副本，见 `maixlite/README.md`）。

## 编译

`kvm_system`、`libkvm.so` 和 `libkvm_mmf.so` 由 `scripts/build-maixlite.sh` 编译，`scripts/build-in-container.sh` 在 `make release-build`（docker）中调用它，`scripts/forge-build.sh` 在 forge 中调用。详见 `scripts/README.md` 和 `maixlite/README.md`。
