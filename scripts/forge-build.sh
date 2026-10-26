#!/bin/bash
#
# Build the NanoKVM riscv64 release on forge, without docker.
#
# Entry point of the `build` profile in .build.json. It runs inside forge's
# `c` build image (nzions/fleet containers/forge-c, forge-c >= 1.26) as an
# unprivileged user, with the source at /work/tree (read-only for our purposes),
# HOME=/work/home and the artifacts share at /artifacts (rw).
#
# It performs the same steps `make release` runs in the docker builder image:
#   1. scripts/build-in-container.sh   kvm_system, libkvm.so, NanoKVM-Server
#   2. web: pnpm install --frozen-lockfile && pnpm build
#   3. scripts/package.sh <version>    nanokvm_<ver>.tar.gz + latest.json
# and then publishes the result to /artifacts/<version>/. Everything is written
# under $HOME (a private copy of the tree and of MaixCDK) and /artifacts.
#
# The toolchains are baked into the image at fixed paths and are NOT on its
# default PATH (they are opt-in); this script puts them on PATH for this run:
#   /opt/go-nanokvm         Go 1.26.0 (the pin of docker/Dockerfile)
#   /opt/sg2002/host-tools  Sophgo riscv64-linux-musl gcc (cgo for the server)
#   /opt/node               Node 22 + pnpm 11 (web/)
#   /opt/maixcdk            MaixCDK at a pinned commit, with its python venv
#
# VERSION (first match wins):
#   1. $NANOKVM_VERSION                       explicit override
#   2. an exact numeric git tag on HEAD       how upstream releases are cut
#                                             (create-tag.yml tags MAJOR.MINOR.PATCH)
#   3. the newest "## X.Y.Z" heading of CHANGELOG.md
# The repo has no VERSION file; the version is baked into the package
# (/kvmapp/version, latest.json) and package.sh accepts MAJOR.MINOR.PATCH only.
#
# Signing is NOT done here: the device refuses a package without a razor-os
# updatesig envelope. The script ends by printing the sha256 and the name to
# sign; see scripts/README.md.

set -eu
set -o pipefail

SRC="$(cd "$(dirname "$0")/.." && pwd)"
ARTIFACTS="${ARTIFACTS_DIR:-/artifacts}"

MAIXCDK_SRC=/opt/maixcdk
GO_ROOT=/opt/go-nanokvm
HOST_TOOLS_BIN=/opt/sg2002/host-tools/gcc/riscv64-linux-musl-x86_64/bin
NODE_ROOT=/opt/node

die() {
    echo "[forge-build] ERROR: $*" >&2
    exit 1
}

log() {
    echo "[forge-build] $*"
}

# --- environment ---------------------------------------------------------------
[ -n "${HOME:-}" ] || die "HOME is unset"
case "$HOME" in
    /|"") die "HOME is '$HOME'; refusing to build into the filesystem root" ;;
esac
[ "$(id -u)" -ne 0 ] || die "refusing to run as root"

for path in "$GO_ROOT/bin/go" "$HOST_TOOLS_BIN/riscv64-unknown-linux-musl-gcc" \
            "$NODE_ROOT/bin/node" "$NODE_ROOT/bin/pnpm" "$MAIXCDK_SRC/bin/activate"; do
    [ -e "$path" ] || die "$path is missing: this is not forge-c >= 1.26 (the image carrying the NanoKVM toolchains)"
done
for tool in patchelf python3 openssl cmake git tar gzip; do
    command -v "$tool" >/dev/null 2>&1 || die "'$tool' is missing from the build image"
done

# Opt-in toolchains, in front of the image defaults. GOTOOLCHAIN=local: never let
# go fetch a toolchain; the pinned one is the build.
export PATH="$GO_ROOT/bin:$HOST_TOOLS_BIN:$NODE_ROOT/bin:$PATH"
export GOTOOLCHAIN=local
export CI=true

# --- version -------------------------------------------------------------------
changelog_version() {
    awk '/^## [0-9]+\.[0-9]+\.[0-9]+ / { print $2; exit }' "$SRC/CHANGELOG.md"
}

VERSION="${NANOKVM_VERSION:-}"
VERSION_SOURCE="NANOKVM_VERSION"
if [ -z "$VERSION" ] && [ -e "$SRC/.git" ]; then
    VERSION="$(git -C "$SRC" tag --points-at HEAD 2>/dev/null | grep -E '^[0-9]+\.[0-9]+\.[0-9]+$' | sort -V | tail -n 1 || true)"
    VERSION_SOURCE="git tag at HEAD"
fi
if [ -z "$VERSION" ]; then
    VERSION="$(changelog_version)"
    VERSION_SOURCE="CHANGELOG.md"
fi
printf '%s' "$VERSION" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+$' \
    || die "no MAJOR.MINOR.PATCH version found (got '$VERSION' from $VERSION_SOURCE)"
log "version $VERSION (from $VERSION_SOURCE); commit ${FORGE_COMMIT:-unknown}"

# Reproducible archive timestamps. package.sh takes the last commit time from git;
# a tree copied without .git would fall back to epoch 0, so use the release date
# the CHANGELOG records for this version instead.
if [ -z "${SOURCE_DATE_EPOCH:-}" ] && ! git -C "$SRC" log -1 --format=%ct >/dev/null 2>&1; then
    changelog_date="$(awk -v v="$VERSION" '$1 == "##" && $2 == v { gsub(/[()]/, "", $3); print $3; exit }' "$SRC/CHANGELOG.md")"
    if [ -n "$changelog_date" ] && SOURCE_DATE_EPOCH="$(date -u -d "$changelog_date" +%s 2>/dev/null)"; then
        export SOURCE_DATE_EPOCH
    else
        unset SOURCE_DATE_EPOCH
    fi
fi

# --- private working copies ----------------------------------------------------
# support/sg2002/build resolves both trees through ~ (~/MaixCDK, ~/NanoKVM) and
# writes into both (components are overlaid on MaixCDK; dist/ and kvmapp/ in the
# source), so each gets a private copy under HOME. The MaixCDK venv stays at
# /opt/maixcdk: it holds absolute paths, and activating it in place is what
# docker/Dockerfile's `. bin/activate` does.
WORK_NANOKVM="$HOME/NanoKVM"
WORK_MAIXCDK="$HOME/MaixCDK"
for d in "$WORK_NANOKVM" "$WORK_MAIXCDK"; do
    case "$d" in
        "$HOME"/*) rm -rf "$d" ;;
        *) die "refusing to clear $d outside HOME" ;;
    esac
done
log "copying the source tree and MaixCDK under $HOME"
cp -a "$SRC" "$WORK_NANOKVM"
cp -a "$MAIXCDK_SRC" "$WORK_MAIXCDK"
chmod -R u+rwX "$WORK_NANOKVM" "$WORK_MAIXCDK"

# --- riscv64 artifacts (what `make release-build` runs in the builder image) ---
# build-in-container.sh needs HOME == $BUILD_HOME and the activated venv.
log "building riscv64 artifacts (kvm_system, libkvm.so, NanoKVM-Server)"
export BUILD_HOME="$HOME"
set +u
# shellcheck disable=SC1091
. "$MAIXCDK_SRC/bin/activate"
set -u
bash "$WORK_NANOKVM/scripts/build-in-container.sh"
# Leave the venv: the web build must not see the SDK's python on PATH first.
set +u
deactivate
set -u

# --- web (what `make web` runs) ------------------------------------------------
log "building the frontend"
(
    cd "$WORK_NANOKVM/web"
    pnpm install --frozen-lockfile
    pnpm build
)

# --- package (what `make package` runs) ----------------------------------------
log "packaging nanokvm_$VERSION"
(
    cd "$WORK_NANOKVM"
    bash scripts/package.sh "$VERSION"
)

OUT="$WORK_NANOKVM/build/release"
TARBALL="$OUT/nanokvm_$VERSION.tar.gz"
for f in "$TARBALL" "$OUT/latest.json" "$TARBALL.sha256"; do
    [ -s "$f" ] || die "package.sh did not produce $f"
done
SHA256="$(tr -d ' \n' < "$TARBALL.sha256")"
printf '%s' "$SHA256" | grep -qE '^[0-9a-f]{64}$' || die "bad sha256 in $TARBALL.sha256: '$SHA256'"
[ "$SHA256" = "$(sha256sum "$TARBALL" | cut -d' ' -f1)" ] || die "sha256 file does not match the tarball"

# --- emulator build of the server (qemu, no cgo, no hardware) -------------------
# Same pinned Go as above. The release server (server/build.sh) is a plain
# `go build`; -trimpath is added so the binary carries no build-host paths.
EMU_SERVER="$WORK_NANOKVM/build/emu/NanoKVM-Server"
log "building the emulator server (-tags nokvm)"
mkdir -p "$(dirname "$EMU_SERVER")"
(
    cd "$WORK_NANOKVM/server"
    CGO_ENABLED=0 GOOS=linux GOARCH=riscv64 go build -trimpath -tags nokvm -o "$EMU_SERVER" .
)
[ -s "$EMU_SERVER" ] || die "the emulator server build produced no $EMU_SERVER"

# gpioedge: the GPIO chardev v2 edge proof for the emulator's gpio-sim kernel
# (tools/gpioedge, no dependencies). Run in the guest; never installed on a device.
EMU_GPIOEDGE="$WORK_NANOKVM/build/emu/gpioedge"
log "building the emulator gpio edge proof (tools/gpioedge)"
(
    cd "$WORK_NANOKVM/tools/gpioedge"
    CGO_ENABLED=0 GOOS=linux GOARCH=riscv64 go build -trimpath -o "$EMU_GPIOEDGE" .
)
[ -s "$EMU_GPIOEDGE" ] || die "the gpioedge build produced no $EMU_GPIOEDGE"
[ -s "$WORK_NANOKVM/scripts/emu-image.sh" ] || die "scripts/emu-image.sh is missing from the tree"

# --- publish to the artifacts share --------------------------------------------
[ -d "$ARTIFACTS" ] && [ -w "$ARTIFACTS" ] || die "$ARTIFACTS is not a writable directory (run this under the build profile)"
DEST="$ARTIFACTS/$VERSION"
if [ -e "$DEST" ]; then
    # A version is published once. The same bytes again is a harmless re-run;
    # anything else would silently replace what devices may already have fetched.
    existing="$(sha256sum "$DEST/nanokvm_$VERSION.tar.gz" 2>/dev/null | cut -d' ' -f1 || true)"
    if [ "$existing" = "$SHA256" ]; then
        log "$DEST already holds this exact package (sha256 $SHA256); leaving it"
    else
        die "$DEST already exists with different content (existing sha256 '${existing:-none}', this build $SHA256); bump the version"
    fi
else
    STAGE="$ARTIFACTS/.nanokvm-$VERSION.$$"
    rm -rf "$STAGE"
    mkdir "$STAGE"
    cp "$TARBALL" "$OUT/latest.json" "$TARBALL.sha256" "$STAGE/"
    # emu/ is for the qemu emulator only: NOT in the OTA tarball, NOT signed,
    # and must never be copied to the update base.
    mkdir "$STAGE/emu"
    install -m 0755 "$EMU_SERVER" "$STAGE/emu/NanoKVM-Server"
    (cd "$STAGE/emu" && sha256sum NanoKVM-Server > NanoKVM-Server.sha256)
    install -m 0755 "$EMU_GPIOEDGE" "$STAGE/emu/gpioedge"
    (cd "$STAGE/emu" && sha256sum gpioedge > gpioedge.sha256)
    install -m 0755 "$WORK_NANOKVM/scripts/emu-image.sh" "$STAGE/emu/emu-image.sh"
    mv "$STAGE" "$DEST"
    log "published $DEST"
fi

echo
cat "$OUT/latest.json"
echo
echo "version:   $VERSION"
echo "artifacts: $DEST/{nanokvm_$VERSION.tar.gz,latest.json,nanokvm_$VERSION.tar.gz.sha256}"
echo "emulator:  $DEST/emu/{NanoKVM-Server,NanoKVM-Server.sha256,emu-image.sh} (not signed, not for the update base)"
echo "sha256:    $SHA256"
echo "sign name: nanokvm-$VERSION-riscv64"
echo "next:      keymaster_sign_digest kind=app name=nanokvm-$VERSION-riscv64 sha256=$SHA256"
echo "           -> write the envelope to $DEST/nanokvm_$VERSION.tar.gz.sig"
