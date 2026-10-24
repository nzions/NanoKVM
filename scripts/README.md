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
