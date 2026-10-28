# Release packaging

These scripts assemble the application update package that NanoKVM devices pull
over the air: `nanokvm_<version>.tar.gz` plus its `latest.json` manifest.

| Script | Purpose |
|---|---|
| `build-in-container.sh` | Builds every riscv64 artifact (`kvm_system`, `libkvm.so`, `NanoKVM-Server`). Runs inside the `nanokvm-builder` image only. |
| `forge-build.sh` | forge's build entry: the same steps as `make release`, without docker; publishes to `/artifacts/<version>/`. See "Building on forge". |
| `package.sh` | Stages the package tree, creates the tarball, and writes `latest.json`. |
| `compare-release.sh` | Diffs a freshly built package against the currently published one. Informational. |
| `verify-release-assets.sh` | Checks the three GitHub Release assets before publishing or promotion. |
| `verify-release-tag.sh` | Requires an annotated numeric tag whose commit is on `main`. |

## What the updater expects

The format is not arbitrary — it is fixed by the on-device updater in
`server/service/application/`:

- **One safe root directory.** The updater scans before extracting and accepts
  only directories and regular files under `nanokvm_<version>/`; links,
  special files, duplicate paths, and path traversal are rejected.
- **`name` is the file name.** `version.go` builds the download URL as
  `<base>/<name>`, where `<base>` is `https://hc.nikspace.net/nanokvm` (or
  `.../preview` when `/etc/kvm/preview_updates` exists). The base must be
  https.
- **Every package needs a signature.** Publish `<name>.sig` next to the
  tarball: a razor-os updatesig envelope (kind `app`, name
  `nanokvm-<version>-riscv64`, over the tarball's sha256) chaining to the
  device's pinned firmware CA. `package.sh` prints the sha256 and the signing
  name. The device refuses an update without a valid `.sig`.
- **`sha512` is base64, not hex.** `update.go` compares against
  `base64(raw sha512 digest)`. A hex digest will fail verification on device.
- **`/kvmapp/version`** is what the device reports as its installed version, so
  it must match the `version` field.

New releases use manifest v2. `size` remains the exact compressed byte count
for older clients, while v2 clients use `size_bytes` and
`unpacked_size_bytes` for storage preflight checks. Historical v1 `size`
values have inconsistent units, so v2 clients only require v1 `size` to be
non-zero and verify the downloaded SHA-512 instead. Publish v2 metadata before
releasing clients that consume it; clients retain v1 compatibility for custom
update servers.

## Building a release

The frontend needs Node and pnpm on the host; everything riscv64 needs the
builder image (see `docker/Dockerfile`).

```bash
make release VERSION=2.4.4
```

That is equivalent to:

```bash
make release-build         # kvm_system + libkvm.so + NanoKVM-Server, in Docker
make web                   # web/dist
make package VERSION=2.4.4 # build/release/{nanokvm_2.4.4.tar.gz,latest.json}
```

In CI this runs as the **NanoKVM Package** workflow. Pull requests and manual
runs only create Actions artifacts. For a public release, run **NanoKVM Create
Tag**, then run **NanoKVM Release** and choose prerelease, stable, or promotion.
Publishing attaches three assets:

Publishing is one-shot: an existing Release or draft is an error, and only a
published prerelease can be promoted.

- `nanokvm_<version>.tar.gz`
- `latest.json`
- `sha256.txt`

The Actions artifact also includes `BUILD_INFO.txt` with the source commit,
workflow run, immutable builder image digest, and both checksum encodings; that
provenance file is not attached to the GitHub release. Uploading the tarball and
`latest.json` to the CDN is what actually offers the update to devices in the
field, so that step stays manual. `sha256.txt` is only for users verifying a
manual download and does not change the on-device `latest.json` contract.

## Building on forge

forge (the estate build box) builds the same release without docker: the
`build` profile in `.build.json` runs `scripts/forge-build.sh` in forge's `c`
image (nzions/fleet `containers/forge-c`, 1.26 or newer), which carries the
pinned toolchains at `/opt` (Sophgo host-tools, Go 1.26.0,
Node 22 with pnpm 11). The pins match `docker/Dockerfile`.

```text
forge_build repo=nanokvm ref=<sha>
```

The script runs `build-in-container.sh`, the `make web` steps and `package.sh`
in a private copy of the tree under `$HOME`, then publishes
`nanokvm_<version>.tar.gz`, `latest.json` and `nanokvm_<version>.tar.gz.sha256`
to `/artifacts/<version>/` (`/data/nfs/artifacts/nanokvm/<version>/` on beefy).
A published version is never overwritten with different bytes. The version is
`$NANOKVM_VERSION`, else an exact numeric git tag on the commit, else the newest
`## X.Y.Z` heading in `CHANGELOG.md`; bump the CHANGELOG to build a new one.
The same atomic publish also carries `emu/NanoKVM-Server` (a `-tags nokvm`
`CGO_ENABLED=0` build), `emu/NanoKVM-Server.sha256` and `emu/emu-image.sh` for
the qemu emulator. They are not in the OTA tarball, not signed, and never go to
the update base.
No MaixCDK is used and nothing is downloaded. The build fails closed if `$HOME/MaixCDK`
exists (before or after the build), if the environment references MaixCDK, or if the build
log contains a `Downloading` line or a MaixCDK path; it also fails if a compiler does not
resolve into `/opt/sg2002/host-tools`.

The output is **unsigned**. The script prints the sha256 and the name to sign;
a rook then calls

```text
keymaster_sign_digest kind=app name=nanokvm-<version>-riscv64 sha256=<sha256>
```

and writes the returned envelope next to the tarball as
`nanokvm_<version>.tar.gz.sig`. Publish tarball, `.sig` and `latest.json`
together; the device refuses an update without a valid `.sig`.

## Where each file in the package comes from

| Package path | Source |
|---|---|
| `version` | the requested version |
| `server/NanoKVM-Server` | `server/build.sh` (BoringCrypto, RPATH `$ORIGIN/dl_lib`) |
| `server/dl_lib/` | libkvm.so, libkvm_mmf.so (the `kvm_vision` build), no OpenCV (`libkvm.so` is checked for none); the remaining vendor SDK libraries are backfilled from the tracked `server/dl_lib/` (see its README.md) |
| `server/web/` | `web/dist/` |
| `kvm_system/kvm_system` | `support/sg2002` `build kvm_system` |
| `system/tool/` | prebuilt binaries in `tools/nanokvm_update_edid/` |
| `kvm/` | default runtime state (resolution, fps, quality) |
| everything else | tracked `kvmapp/` |

Note that `kvm/` ships default runtime state, and `install.go` replaces
`/kvmapp` wholesale, so an update resets those values on the device. That is
long-standing behaviour, not something these scripts introduce.

## The two copies of libkvm.so

`NanoKVM-Server` is cgo-linked against `server/dl_lib/libkvm.so`
(`server/common/kvm_vision.go`: `-L../dl_lib -lkvm`). That file is no longer
tracked: `build-in-container.sh` copies the freshly built libkvm.so (and the other
built libraries) there before linking, so the link-time and shipped libraries are
the same build.

Because a symbol the binary imports could in principle be absent from the
shipped library, and that would only surface as a crash on a real device,
`build-in-container.sh` checks the shipped `libkvm.so` still exports every
symbol `NanoKVM-Server` actually imports from it, and fails the build otherwise.

## emu-image.sh: a QEMU-bootable emulator image

`emu-image.sh` turns Sipeed's stock NanoKVM SD image into an image that boots
under `qemu-system-riscv64 -M virt -cpu thead-c906` and serves the web UI, so
the server can be exercised without hardware. It needs `debugfs`, `e2fsck`
(e2fsprogs) and `sfdisk` (util-linux); no root and no loop mount.

```sh
# server: CGO_ENABLED=0 GOOS=linux GOARCH=riscv64 go build -tags nokvm   (from server/)
scripts/emu-image.sh [--sha256 <hex>] [--authorized-key <pubfile>] <stock.img> <NanoKVM-Server> <out.img>
```

`<out.img>` must not exist; the stock image is only read. The script finds
partition 2 (the ext4 rootfs) in the partition table, cuts it out, runs
`e2fsck -fy` (a booted image carries `needs_recovery`, and a journal replay
would silently revert the edits), edits it with `debugfs -w`, checks it again,
splices it back into a copy and prints the output's sha256. Every edit is read
back and the script fails if one did not land:

1. The `/lib/ld-musl-riscv64*xthead*` loaders become symlinks to
   `/lib/ld-musl-riscv64.so.1`. Sipeed's xthead loaders use RVV 0.7 vector
   instructions that QEMU's thead-c906 lacks.
2. The server is written to `/kvmapp/server/NanoKVM-Server` (0755).
3. `/etc/init.d/S95nanokvm` loses its `kvm_system` start line (the hardware
   daemon dies without `/dev/cvi-*`); the original is kept as
   `/etc/S95nanokvm.orig`, outside `init.d` because rcS runs every `S??*`
   there. The diff is printed.
4. `/etc/inittab` becomes a busybox inittab (original kept as `inittab.orig`),
   because sysvinit's `/sbin/init` stalls under QEMU after its first sysinit
   job. It carries the stock inittab's mount/mkdir sysinit lines ahead of rcS:
   without `/proc` the server's `os.Executable` fails and it panics.
5. `/usr/bin/init` becomes a symlink to `/bin/busybox` (the kernel is booted
   with `init=/usr/bin/init`).
6. `/etc/kvm/ssh_stop` is removed so sshd starts.
7. `/etc/kvm/emulator` marks the image as an emulator (our supervised
   `S95nanokvm` skips `kvm_system` when it exists).
8. With `--authorized-key <pubfile>`, the OpenSSH public key(s) in `<pubfile>`
   are authorised for root: `/root/.ssh` (0700) and `/root/.ssh/authorized_keys`
   (0600), both `root:root`, read back like every other edit. The script
   refuses a file that is not an OpenSSH public key or that holds a private key.
   Without the flag the image carries no credential. Use a dedicated,
   emulator-only key (the fleet's is `/rook-agent/nanokvm/emu-access-key` in
   sage; the `.pub` half is derived with `ssh-keygen -y`), never a device key.

Boot it:

```sh
qemu-system-riscv64 -M virt -cpu thead-c906 -smp 1 -m 512 -bios default \
  -kernel <kernel-nanokvm-emu Image> \
  -append 'root=/dev/vda2 rw console=ttyS0 init=/usr/bin/init' \
  -drive file=<out.img>,format=raw,if=virtio \
  -netdev user,id=n0,hostfwd=tcp::18443-:443,hostfwd=tcp::18080-:80 \
  -device virtio-net-device,netdev=n0 -nographic
```

The web UI is then on `https://localhost:18443` and `http://localhost:18080`.

#### After an OTA: `--refresh`

An online update replaces `/kvmapp` wholesale, which brings back the hardware
(cgo) server and an `S95nanokvm` that starts `kvm_system`; both segfault under
qemu. Two `S95nanokvm` copies run: `/etc/init.d/S95nanokvm` at boot (rcS), and
`/kvmapp/system/init.d/S95nanokvm` for the `restart` the update issues.
`--refresh` fixes both, in place:

```sh
scripts/emu-image.sh --refresh <out.img> <NanoKVM-Server>
```

1. Let the emulator take the OTA.
2. Stop the qemu VM (the image is edited in place; never run this on a live VM).
3. Run `--refresh` with the `emu/NanoKVM-Server` from the version's artifact
   dir (or your own `-tags nokvm` build). It reinstalls the server (0755),
   strips the `kvm_system` start line from both `S95nanokvm` copies (printing
   the diffs; a copy that has none counts as already refreshed), and uses the
   same dd cut-out, `debugfs -w`, `e2fsck` and `dd conv=notrunc` splice as the
   build. It refuses an image without `/etc/S95nanokvm.orig`, so only an image
   this script already built is accepted.
4. Boot the VM again with the qemu command above.

### The kernel

The kernel is built by forge, not patched: fleet's native files build
`kernel-nanokvm-emu` (`containers/kernel-nanokvm-emu`) compiles a pinned
kernel.org stable tarball (6.12.x) for `ARCH=riscv` with clang/lld, offline. It
is `defconfig` with `CONFIG_ERRATA_THEAD=n` (the T-Head errata probe
`csrr sxstatus`, which QEMU traps on, is never built; `ARCH_THEAD`,
`ARCH_SUNXI` and the other SoCs that select it are off), modules off, virtio
(pci and mmio) blk/net/console, ext4, 8250 serial and devtmpfs built in, and
netfilter (conntrack, the xtables matches the firewall uses, legacy iptables
and nf_tables with `NFT_COMPAT`) built in, so `nanokvm-firewall` finds its
tables. The build fails if a required setting did not take. It lands at
`<share>/fleet/artifacts/kernel-nanokvm-emu/<version>/{Image,config}`; boot it
with `-kernel Image` and
`-append 'root=/dev/vda2 rw console=ttyS0 init=/usr/bin/init'`. There is no
initrd and no modules. The old 4-byte patch of Ubuntu's `linux-riscv` `vmlinuz`
(`emu-image.sh --patch-kernel`) is gone.

## sd-image.sh: a flashable microSD image for real hardware

End-to-end recipe (kernel, app, re-pinning, traps): [docs/building-the-sd-image.md](../docs/building-the-sd-image.md).

`sd-image.sh` is the real-device counterpart of `emu-image.sh`. It takes
Sipeed's stock image and one of our signed releases and writes an image that
boots what an OTA of that release leaves on a stock device. The kernel, boot
partition, musl loaders and sysvinit stay stock (none of `emu-image.sh`'s
emulator edits apply). No root, no loop mount: dd cut-out of partition 2,
`debugfs -w`, `e2fsck`, dd splice back, then a read-back of the whole `/kvmapp`
tree against the release.

```bash
scripts/sd-image.sh [--stock-sha256 <hex>] [--sha256 <hex>] \
    <20260610_NanoKVM_Rev1_4_3.img> <nanokvm_2.5.16.tar.gz> <out.img>
```

- Both inputs are verified by sha256 (stock: the 1.4.3 image is the default;
  tarball: `--sha256` or the `<tarball>.sha256` next to it) and the tarball by
  the updater's rules (one `nanokvm_<ver>/` root, only directories and regular
  files, riscv64 server and `kvm_system`, `version` matches).
- The stock `/kvmapp` is removed and the release tree is installed in its
  place (files 0755, root:root). `kvm_new_app` stays in the tree and
  `kvm_new_img` is carried over from the stock image, so `kvm_system` runs its
  normal first-boot steps. The release's `S00kmod S01fs S03usbdev S15kvmhwd
  S30eth S50sshd S95nanokvm` (and `S30wifi`) are copied to `/etc/init.d`, the
  copies `new_app_init` makes, so the first boot already runs our supervised
  `S95nanokvm`, firewall step and boot recovery.
- No credential is written: `/etc/kvm/pwd`, `initial.pass`, `root.pass` and
  `server.yaml` do not exist in the image. There is one account, `root`: the web
  login and the Linux login share root's password. An operator drops it (any
  non-empty password, at most 256 bytes) in `root.pass` on the FAT boot
  partition (`/boot`, label `boot`, writable from a Mac; see FLASHING.md): the
  server sets root's password from it and removes the file once applied. Without
  it the server sets a random password, shows it on the OLED, stores it in
  `/etc/kvm/initial.pass` (0600, only the OLED channel), and the first web login
  must change it. The image's `/boot` is the
  stock one, untouched. `/etc/kvm/ssh_stop` stays as the stock
  image ships it (sshd off until enabled in the UI).
- The result cannot be boot-tested under QEMU: the stock kernel and xthead
  loaders need the SG2002's C906 vector unit and cvi video hardware.
- Flash the (xz-decompressed) image to the card, for example
  `dd if=nanokvm-cube-2.5.16-sd.img of=/dev/rdiskN bs=4m` on macOS.

### sd-image.sh --kernel-fit: our own kernel on the Cube

```bash
scripts/sd-image.sh --kernel-fit <Image> <sg2002-nanokvm-cube.dtb> --no-stock-ko \
    [--fit-load 0x83000000] [--fit-comp lzma|gzip] [--fw-reserve 0x80000] \
    <stock-or-earlier-sd.img> <nanokvm_VERSION.tar.gz> <out.img>
```

Keeps the stock FSBL, OpenSBI and U-Boot (`FIP.BIN` on the FAT16 boot partition) and
replaces only `BOOT.SD` with a FIT of OUR Image and DTB (fleet
`containers/kernel-nanokvm-sg2002`, the `-full` build with the vendor drivers built in).
Stock U-Boot's `sdboot` loads `BOOT.SD` to 0x81800000 and runs
`bootm ${uImage_addr}#config-sg2002_licheervnano_sd`, so that configuration name is kept.
Needs mkimage, dumpimage, dtc, mtools (mdir mcopy mdel) and xz besides the tools above. The
forge build is `forge_build repo=nanokvm ref=<sha> scope=["scripts"]` (see below).

* The FIT has no ramdisk (our DTB's bootargs name `root=/dev/mmcblk0p2`; U-Boot's own bootargs
  win anyway and carry the same root).
* The Image is ~38 MB and the boot partition is 16 MB, so the kernel is lzma-compressed in the
  FIT (gzip, 14 MB, is the fallback and also fits). The script refuses a FIT that does not fit.
* `--fit-load` is above `0x81800000 + FIT size` on purpose: decompressing onto the stock
  0x80200000 would overwrite the FIT (and the DTB, which bootm leaves in place) being read.
* `--fw-reserve`: the stock bootargs carry `riscv.fwsz=0x80000`, the vendor kernel's way of
  keeping off OpenSBI. The mainline DTB has no node for it, so it gets `/memreserve/ 0x80000000 0x80000`.
* `--no-stock-ko`: `S00kmod` (insmod of `soph_*.ko`) is replaced by the repo's `kvmapp/system/init.d/S00kmod`, which loads nothing.
* `--bootargs <args>`: writes `othbootargs=<args>` to `uEnv.txt` on the boot partition (other lines of an existing uEnv.txt are kept). The former insmod parameters of the built-in drivers go here as `<driver>.<param>=<v>`: `cvi_vc_drv.MaxVencChnNum=9 cvi_vc_drv.MaxVdecChnNum=9` (forge-sdimage.sh passes them). `S00kmod` sets the same two values from `/sys/module/cvi_vc_drv/parameters/` if the command line did not carry them.
* The stock boot partition is untouched except `BOOT.SD`; `uEnv.txt` on it (loaded by stock
  U-Boot's `loadenvcmd`) can override `othbootargs`, for example `loglevel=7`, without a rebuild.
* The result is checked structurally only (FIT listing, FAT read-back, kernel extracted from the
  FIT and compared). It cannot be booted under QEMU.

On forge: the `build` profile with scope `scripts` runs `scripts/forge-sdimage.sh` instead of
the release build. Its pins (release version, kernel Image/DTB, base image, Debian .debs for
mkimage/dtc/mtools/e2fsprogs) are at the top of that script, every download is sha256-checked,
and the result lands in `/artifacts/sdimg/nanokvm-cube-<userspace>-<kernel tag>/` (image `.xz`,
`.sha256` of the raw image, `build.log`, `verify.txt` from `scripts/sdimage-verify.sh`). A
published directory is never overwritten. Nothing is flashed.

## ABI manifest (`tools/binabi`)

`scripts/forge-build.sh` runs `tools/binabi/check.sh` on the `scripts/binprint.sh` output right
after fingerprinting. It compares `kvm_system`, `libkvm.so` and `libkvm_mmf.so` with the golden
files in `tools/binabi/golden/` (taken from the 2.6.38 evidence): NEEDED, SONAME, RPATH,
exports, imports, RISC-V attributes, `.comment` and the `.text`/`.rodata`/`.data`/`.bss` sizes
(exact). Any difference not in `tools/binabi/allow.txt` fails the build, and so does an allow
entry that no longer differs; each entry carries its reason. The only entry today is
`libkvm.so`'s build-dir RPATH leak (removed in S6). Run it by hand on any evidence directory:
`tools/binabi/check.sh <dir>`.
