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

# scope `scripts` (forge_build repo=nanokvm scope=["scripts"]) is not the release: it
# builds the Cube's first-boot microSD image with our kernel (scripts/forge-sdimage.sh)
if [ "${FORGE_SCOPE:-}" = scripts ]; then
    exec bash "$SRC/scripts/forge-sdimage.sh"
fi

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

# --- pinned middleware source ----------------------------------------------------
# scpcom/sophgo-middleware @ 4e231ef5967f from the artifacts share, sha256-checked,
# fail closed (scripts/fetch-middleware.sh). Fetched first so a missing or wrong
# mirror stops the build before the long SDK compile. Landed at a fixed path.
MW_SRC="$HOME/mw-src/sophgo-middleware-4e231ef"
log "fetching the pinned middleware source (4e231ef5967f)"
NANOKVM_SRC_MIRROR="$ARTIFACTS/src" bash "$WORK_NANOKVM/scripts/fetch-middleware.sh" "$MW_SRC"
[ "$(cat "$MW_SRC/.pin")" = 4e231ef5967f8f7d77cecd65d9e2052a6f92f18c ] || die "$MW_SRC does not carry the pin"

# --- the open MMF libraries, built from that pin ---------------------------------------
# libsys libcvi_bin libvpu libvenc libvdec libini from the pin's own make files, plus the
# pin's closed ISP/3A blobs; closure-checked (NEEDED + undefined symbols). The kvm_mmf CMake
# links ONLY this directory (scripts/build-mmf.sh), nothing from MaixCDK's v2/lib.
MMF_OUT="$HOME/mmf-out"
log "building the MMF libraries from the pinned source"
NANOKVM_SRC_MIRROR="$ARTIFACTS/src" bash "$WORK_NANOKVM/scripts/build-mmf.sh" "$MW_SRC" "$MMF_OUT"
export NANOKVM_MIDDLEWARE_SRC="$MW_SRC"
export NANOKVM_MMF_LIB="$MMF_OUT/lib"
NANOKVM_MMF_UAPI="$(cat "$MMF_OUT/uapi")"
export NANOKVM_MMF_UAPI

# --- ABI check of the pinned middleware against the 7.3 kernel uapi headers -----------
# tools/abicheck, on the pin and on the two osdrv header trees build-mmf.sh already
# extracted (sha256-checked there; no second fetch). Fails closed: a missing tool is an
# error, never a skip, and so is any mismatch not in tools/abicheck/allow.txt (each entry
# carries its reason; an entry that no longer differs fails too). The riscv64 musl cross
# gcc does the layout/syntax/mirror stages (target-accurate); probe.c is EXECUTED, so it
# is built with the image's host cc (the structs are LP64 on both).
log "abicheck: pinned middleware vs the 7.3 osdrv uapi headers"
for tool in cc readelf awk comm sort; do
    command -v "$tool" >/dev/null 2>&1 || die "abicheck needs '$tool', which the build image lacks (a skipped check is a failed one)"
done
ABI_OUT="$HOME/abicheck-out"
rm -rf "$ABI_OUT"
CC=riscv64-unknown-linux-musl-gcc sh "$WORK_NANOKVM/tools/abicheck/abicheck.sh" \
    --osdrv-v2 "$MMF_OUT/osdrv-v2/interdrv/v2" --osdrv-v1 "$MMF_OUT/osdrv-v1/interdrv" \
    --mw-4e231ef "$MW_SRC" --probe-cc cc \
    --kvm-src "$WORK_NANOKVM/support/sg2002/additional/kvm_mmf/src/kvm_mmf.cpp" \
    --allow "$WORK_NANOKVM/tools/abicheck/allow.txt" --out "$ABI_OUT" \
    || die "abicheck found an ABI mismatch between the pinned middleware and the 7.3 kernel headers (see above)"


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

# The 12 MMF libraries in the package must be the exact bytes build-mmf.sh staged: nothing
# between the build and the archive may swap or rewrite one (fail closed).
log "verifying the MMF libraries inside $(basename "$TARBALL")"
VERIFY_DIR="$HOME/pkg-verify"
rm -rf "$VERIFY_DIR"
mkdir -p "$VERIFY_DIR"
tar -xzf "$TARBALL" -C "$VERIFY_DIR"
for lib in libsys.so libcvi_bin.so libvpu.so libvenc.so libvdec.so libini.so libisp.so \
           libcvi_bin_isp.so libisp_algo.so libae.so libaf.so libawb.so; do
    src="$MMF_OUT/lib/$lib"
    [ "$lib" = libini.so ] && src="$MMF_OUT/lib/3rd/libini.so"
    shipped="$VERIFY_DIR/nanokvm_$VERSION/server/dl_lib/$lib"
    printf '  %s  %s  %s\n' "$(sha256sum "$shipped" | cut -d' ' -f1)" "$(stat -c %s "$shipped")" "$lib"
    cmp -s "$src" "$shipped" || die "$lib in the package is not the pinned build ($src)"
done

# No OpenCV in the package (fail closed): no libopencv* file anywhere in the extracted
# tree, and libkvm.so as packaged has no libopencv NEEDED entry and imports no cv:: symbol.
log "verifying the package carries no OpenCV"
PKG_ROOT="$VERIFY_DIR/nanokvm_$VERSION"
pkg_ocv="$(find "$VERIFY_DIR" -name 'libopencv*')" || die "find failed over $VERIFY_DIR"
[ -z "$pkg_ocv" ] || die "OpenCV libraries in the package: $pkg_ocv"
pkg_kvm="$PKG_ROOT/server/dl_lib/libkvm.so"
[ -f "$pkg_kvm" ] || die "$pkg_kvm is missing from the package"
pkg_dyn="$(readelf -d "$pkg_kvm")" || die "readelf -d failed on the packaged libkvm.so"
pkg_syms="$(readelf --dyn-syms --wide "$pkg_kvm")" || die "readelf --dyn-syms failed on the packaged libkvm.so"
[ "$(printf '%s\n' "$pkg_syms" | awk '$7 == "UND" { n++ } END { print n + 0 }')" -gt 0 ] \
    || die "the packaged libkvm.so has no undefined dynamic symbol: the cv:: check cannot see anything"
pkg_needed="$(printf '%s\n' "$pkg_dyn" | sed -n 's/.*(NEEDED).*\[\(.*\)\].*/\1/p' | LC_ALL=C sort -u | tr '\n' ' ')"
pkg_cv="$(printf '%s\n' "$pkg_syms" | awk '$7 == "UND" && $8 ~ /^_ZN2cv|^_ZNK2cv|opencv/ { print $8 }')"
log "packaged libkvm.so: $(stat -c %s "$pkg_kvm") bytes, NEEDED: $pkg_needed"
case " $pkg_needed " in *opencv*) die "the packaged libkvm.so NEEDs OpenCV" ;; esac
[ -z "$pkg_cv" ] || die "the packaged libkvm.so imports cv:: symbols: $pkg_cv"
log "no OpenCV in the package: no libopencv file, no libopencv NEEDED entry, no cv:: import"
rm -rf "$VERIFY_DIR"

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

# hdmiint: libkvm's real INT-line code (include/internal/hdmi_int.h) in a tiny
# harness, for the emulator's gpio-sim kernel. Static, guest only.
EMU_HDMIINT="$WORK_NANOKVM/build/emu/hdmiint"
log "building the emulator INT line harness (tools/hdmiint)"
"$HOST_TOOLS_BIN/riscv64-unknown-linux-musl-gcc" -static -O2 -Wall -Wextra \
    -o "$EMU_HDMIINT" "$WORK_NANOKVM/tools/hdmiint/hdmiint.c"
[ -s "$EMU_HDMIINT" ] || die "the hdmiint build produced no $EMU_HDMIINT"
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
    install -m 0755 "$EMU_HDMIINT" "$STAGE/emu/hdmiint"
    (cd "$STAGE/emu" && sha256sum hdmiint > hdmiint.sha256)
    install -m 0755 "$WORK_NANOKVM/tools/kvmgpio/kvmgpio" "$STAGE/emu/kvmgpio"
    (cd "$STAGE/emu" && sha256sum kvmgpio > kvmgpio.sha256)
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
