# Building the NanoKVM Cube SD image (kernel, app, image)

Three parts, three repos' worth of code. Read this before touching any of them.
Tool detail for `sd-image.sh` lives in `scripts/README.md`; this page is the
end-to-end recipe and the traps. Checked 2026-10-28.

```
fleet  containers/kernel-nanokvm-sg2002   -> Image + DTB          (A)
nanokvm  CHANGELOG.md + release build     -> nanokvm_<ver>.tar.gz (B)
nanokvm  scripts/forge-sdimage.sh         -> nanokvm-cube-<ver>-k<kver>-sd.img.xz (C)
```

## A. The kernel (repo nzions/fleet)

- `.build.json` entry `build.native.kernel-nanokvm-sg2002-full` is the one the
  SD image uses (vendor drivers built in). `kernel-nanokvm-sg2002` and
  `kernel-nanokvm-sg2002-vendor` share the SAME tree,
  `containers/kernel-nanokvm-sg2002/`. A change to that tree (a kconf line, a
  patch) must bump the version of **all three** entries: a landed version is
  immutable and forge refuses a changed build at an existing version.
  (At 2026-10-28: full 1.5, plain 1.8, vendor 1.14; read the file for today's.)
- Options: `kconf/enable` (no `CONFIG_` prefix), `kconf/disable`, and
  `kconf/required`, which the build asserts. Linux 7.3.0, clang 19, the
  `forge-kernel` toolchain image.
- **Trunk does not auto-start native kernel builds.** Drive it yourself, ~27 min:

  ```
  forge_build repo=fleet ref=<sha> scope=["all"]
  ```

  Package/name scopes run only the Go template, and `changed` is refused on master.
- Lands at
  `/data/nfs/artifacts/fleet/artifacts/kernel-nanokvm-sg2002-full/<ver>/`
  (`Image`, DTB, `config`, `SHA256SUMS`, `stamp.json`). Verify an option with
  `grep CONFIG_X <dir>/config`.

## B. The app (this repo)

The release version is the top `## x.y.z` heading of `CHANGELOG.md`. Bump it,
push, then:

```
forge_build repo=nanokvm scope=["all"]
```

-> `/data/nfs/artifacts/nanokvm/<ver>/nanokvm_<ver>.tar.gz`. Live work is on
branch `delorean/first-connection-password`; master lags it.

## C. The SD image (`scripts/forge-sdimage.sh`)

Its header documents itself. Run it with

```
forge_build repo=nanokvm ref=<sha> scope=["scripts"]
```

in forge's `c` image. It calls
`scripts/sd-image.sh --kernel-fit <Image> <dtb> --no-stock-ko`. Every input is
pinned by sha256 at the top of the script:

- **base image** `nanokvm-cube-2.5.16-sd.img.xz`, Sipeed stock 1.4.3 underneath.
  The MBR, FSBL, OpenSBI, U-Boot, boot FAT **and the whole rootfs** (buildroot,
  busybox, the vendor musl `ld-musl-riscv64v0p7_xthead.so.1`, vendor `.so`) are
  Sipeed's. We cannot rebuild them; we replace only `BOOT.SD` (our kernel FIT)
  and `/kvmapp` (our app).
- **kernel Image + DTB**, fetched from
  `https://hc.nikspace.net/nanokvm/sdimg/inputs/`. These are **staged by hand**
  from the fleet artifact (nothing copies them; discovery 01k607tbpq).
- **app tarball** from B (`USERSPACE`, `USERSPACE_SHA256`).
- pinned Debian trixie tool `.deb`s (mkimage, dtc, mtools, e2fsprogs).

Output: `/data/nfs/artifacts/nanokvm/sdimg/nanokvm-cube-<ver>-k<kver>/` with
`*.img.xz`, `.sha256` (of the raw image), `build.log`, `verify.txt`. An existing
directory is never overwritten: bump a pin or `KERNEL_TAG`.

### Re-pin recipe (what 2.6.52-k1.4 did)

1. Bump `CHANGELOG.md`, run the release build (B).
2. Stage the new kernel `Image` and DTB into `sdimg/inputs/` (root on homecraw,
   owner nik), named `kernel-nanokvm-sg2002-full-<ver>.Image` / `.…dtb`.
3. Edit `USERSPACE`, `USERSPACE_SHA256`, `KERNEL_TAG`, and the kernel URL and
   sha256 pins in `scripts/forge-sdimage.sh`; push the branch.
4. Run the `scope=["scripts"]` build; read `verify.txt`.

## D. Testing without the board: what the emulator can and cannot do

Beefy's system qemu is upstream 10.2.4 (fleet `containers/qemu-system`,
bootstrap section 1y2), which implements `th.sxstatus` (CSR 0x5c0). So the board
kernel boots under `-M virt -cpu thead-c906` with `sdhci-pci` + `sd-card`:

- pad the image to 2 GiB;
- extract the kernel from `BOOT.SD` (a FIT, lzma at byte 244);
- bootargs `console=ttyS0,115200 earlycon root=/dev/mmcblk0p2 rootwait`.

**Userland dies**: Sipeed's musl uses XTheadVector, which no upstream qemu
implements (decide step 01k60afat3). So an SD image can be boot-tested to the
kernel, never to the web UI; the emulator image (`scripts/emu-image.sh`) swaps
the loaders for plain ones and is a different artifact. Recipe dirs on beefy:
`/home/rook/nanokvm-emu/exact-20261028/` and `exact-2652/`.

## E. Serial console on the LicheeRV Nano (the board in kvm1)

Left header, Ethernet end, board top-up: pin 1 (A17) = UART0 RX, pin 2 (A16) =
UART0 TX, pin 3 = GND. 3.3 V logic, 115200. Source:
<https://wiki.sipeed.com/hardware/zh/lichee/assets/RV_Nano/intro/RV_Nano_4.jpg>.
