# Flashing a NanoKVM Cube microSD card from macOS

These steps flash the image that `sd-image.sh` builds: the stock Sipeed 1.4.3 image with our
signed release in `/kvmapp`. The commands are for the current published build, 2.5.16.
For another build, take the file names and hashes from `SHA256SUMS`.

Published at https://hc.nikspace.net/nanokvm/sdimg/:

| file | size | sha256 |
|---|---|---|
| `nanokvm-cube-2.5.16-sd.img.xz` | 207,350,956 B | `0d85839b3a92b51f4c28fbaab4c0b148da23eac896b1f641184ff5477c11ba0c` |
| `nanokvm-cube-2.5.16-sd.img` | 1,627,390,464 B | `e0035fa435a6916ac58da3c908b4c6e93543d4c41c359ebfb83e76390f4dfe0b` |

You need a microSD card reader on the Mac.

## 0. Get the card out

Power the Cube off. The microSD slot may be inside the case, so you may need to take the
cover off to reach it.

## Option A: Raspberry Pi Imager (easiest)

1. Install it: `brew install --cask raspberry-pi-imager`, or get it from https://www.raspberrypi.com/software/.
2. Download the image and check its hash:

       cd ~/Downloads && curl -fLO https://hc.nikspace.net/nanokvm/sdimg/nanokvm-cube-2.5.16-sd.img.xz && shasum -a 256 nanokvm-cube-2.5.16-sd.img.xz

   It must print `0d85839b3a92b51f4c28fbaab4c0b148da23eac896b1f641184ff5477c11ba0c`.
3. Insert the card and open Imager.
4. **Device**: pick any device, or "No filtering".
5. **Operating System**: scroll to the bottom, choose **Use Custom**, and select the `.img.xz`.
6. **Storage**: pick the SD card. Check its size so you don't pick another disk.
7. Press **Next**. If it offers OS customisation, answer **No**. Write it; Imager
   verifies the card and ejects it.

## Option B: Terminal

This uses the raw `.img`, because macOS ships no `xz`.

1. Download the image and check its hash:

       cd ~/Downloads && curl -fLO https://hc.nikspace.net/nanokvm/sdimg/nanokvm-cube-2.5.16-sd.img && shasum -a 256 nanokvm-cube-2.5.16-sd.img

   It must print `e0035fa435a6916ac58da3c908b4c6e93543d4c41c359ebfb83e76390f4dfe0b`.
2. Find the card: `diskutil list external physical`. It is the disk the size of your card,
   e.g. `/dev/disk4`. **Be sure of N**: `dd` overwrites whatever disk you give it.
3. Write it, with `N` replaced by that number:

       diskutil unmountDisk /dev/diskN && sudo dd if=nanokvm-cube-2.5.16-sd.img of=/dev/rdiskN bs=4m status=progress && sync && diskutil eject /dev/diskN

## After flashing: the hostname and the root password

The hostname is OPTIONAL: a non-empty `hostname` file on the `boot` partition wins. Without
one the box names itself `nanokvm-<id>` (mDNS `nanokvm-<id>.local`, and the name DHCP
sends), `<id>` being the last 4 characters of the board's serial number (the SoC unique id),
or, if it has none, the last 4 hex digits of eth0's MAC. A name set later in the web UI
is saved to that same `hostname` file, so it persists and wins.

There is one account, `root`: the web login and the Linux login share its password. The
card's small FAT partition (label `boot`) is readable and writable from the Mac. A
`root.pass` on it is OPTIONAL: with one, the first boot takes root's password from it; with
none, the box boots with NO password (root is locked, no ssh password login) and the first
web connection sets it. The OLED is not used. **Warning: with no `root.pass`, anyone on the
LAN who connects first claims the box.** The network is DHCP on first boot: the card carries
no `eth.nodhcp`.

1. Re-insert the card (the flash ejected it). It mounts as `/Volumes/boot` (macOS adds a
   number, e.g. `boot 1`, if another volume is already named `boot`; use the name you see).
2. (Optional: skip it for the no-password first boot; write `hostname` only if you want a name of your own.) On m5sspro (where nik's sage key is), as one line: generate the password, store it in
   sage FIRST, share it with the rook, only then write it to the card, and eject. `--show`
   is needed or sage prints a redaction instead of the value:

       printf 'kvm1' > /Volumes/boot/hostname && R=$(openssl rand -base64 18) && printf '%s' "$R" | ~/.claude/bin/sage set --stdin secret /nik/devicecred/nanokvm/kvm1/root && ~/.claude/bin/sage share /nik/devicecred/nanokvm/kvm1/root rook-agent && ~/.claude/bin/sage sync && ~/.claude/bin/sage get --show /nik/devicecred/nanokvm/kvm1/root > /Volumes/boot/root.pass && unset R && sync && diskutil eject /Volumes/boot

   The password here is 24 characters, but there is no policy: any non-empty password of at
   most 256 bytes without `:`, a newline or NUL works, even `a`. An empty `root.pass` is
   ignored and the device boots with no password (first web connection sets it).
3. Put the card back, cable the Cube as before, and power it on. First boot takes a minute
   or two. The server sets root's password from `root.pass`, deletes the file from the card
   as soon as it has used it, and does not force a change (you chose it). Without
   `root.pass` root has no password: open the web UI at the device's address, the login
   page shows "Set root password", and the first connection sets it (once; a second setup is
   refused). The button reset (BOOT held 10 s) returns the device to this no-password state.
4. **kvm1 in our estate: do NOT log in yourself.** Tell the rook. It logs in as `root` with
   the password from sage, sets a new password stored in sage for rookery, captures the
   device cert, adds kvm1 to rookery's and capsd's config, and runs the screenshot and
   signed/tampered OTA checks (journey 01k5oi5052).
5. If `root.pass` is still on the card after a minute or two of running, the device could
   not remove it: delete it yourself and report it.
6. If it never comes up on the network after a few minutes, report it. The image has not been
   boot-tested on hardware before; it cannot run under qemu.
