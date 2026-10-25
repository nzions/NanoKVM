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
pinned toolchains at `/opt` (Sophgo host-tools, MaixCDK and its venv, Go 1.26.0,
Node 22 with pnpm 11). The pins match `docker/Dockerfile`.

```text
forge_build repo=nanokvm ref=<sha>
```

The script runs `build-in-container.sh`, the `make web` steps and `package.sh`
in private copies of the tree and MaixCDK under `$HOME`, then publishes
`nanokvm_<version>.tar.gz`, `latest.json` and `nanokvm_<version>.tar.gz.sha256`
to `/artifacts/<version>/` (`/data/nfs/artifacts/nanokvm/<version>/` on beefy).
A published version is never overwritten with different bytes. The version is
`$NANOKVM_VERSION`, else an exact numeric git tag on the commit, else the newest
`## X.Y.Z` heading in `CHANGELOG.md`; bump the CHANGELOG to build a new one.
The same atomic publish also carries `emu/NanoKVM-Server` (a `-tags nokvm`
`CGO_ENABLED=0` build), `emu/NanoKVM-Server.sha256` and `emu/emu-image.sh` for
the qemu emulator. They are not in the OTA tarball, not signed, and never go to
the update base.
MaixCDK downloads its own toolchain and component packages while building, so
the run needs network access.

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
| `server/dl_lib/` | whatever the `kvm_vision` build emits, with any library it does not emit backfilled from the tracked `server/dl_lib/` |
| `server/web/` | `web/dist/` |
| `kvm_system/kvm_system` | `support/sg2002` `build kvm_system` |
| `system/tool/` | prebuilt binaries in `tools/nanokvm_update_edid/` |
| `kvm/` | default runtime state (resolution, fps, quality) |
| everything else | tracked `kvmapp/` |

Note that `kvm/` ships default runtime state, and `install.go` replaces
`/kvmapp` wholesale, so an update resets those values on the device. That is
long-standing behaviour, not something these scripts introduce.

## The two copies of libkvm.so

`NanoKVM-Server` is cgo-linked against the **tracked** `server/dl_lib/libkvm.so`
(`server/common/kvm_vision.go`: `-L../dl_lib -lkvm`), but the package ships the
library the `kvm_vision` build just produced. Those are two different files, and
the tracked one can lag well behind — published releases have shipped this
mismatch for a long time.

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
scripts/emu-image.sh [--sha256 <hex>] <stock.img> <NanoKVM-Server> <out.img>
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
