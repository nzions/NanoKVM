# Flashing a NanoKVM Cube microSD card from macOS

These steps flash the image that `sd-image.sh` builds: the stock Sipeed 1.4.3 image with our
signed release in `/kvmapp`. The commands are for the current published build, 2.5.14.
For another build, take the file names and hashes from `SHA256SUMS`.

Published at https://hc.nikspace.net/nanokvm/sdimg/:

| file | size | sha256 |
|---|---|---|
| `nanokvm-cube-2.5.14-sd.img.xz` | 207,365,424 B | `7e21de745cf0abfd44e5b0ee84b6eb77c65150cd9b3359993bc37605f8998224` |
| `nanokvm-cube-2.5.14-sd.img` | 1,627,390,464 B | `a1a826f719313fffd85725ded490a9b16fd6b8720e18338f6c47d61d0d857a48` |

You need a microSD card reader on the Mac.

## 0. Get the card out

Power the Cube off. The microSD slot may be inside the case, so you may need to take the
cover off to reach it.

## Option A: Raspberry Pi Imager (easiest)

1. Install it: `brew install --cask raspberry-pi-imager`, or get it from https://www.raspberrypi.com/software/.
2. Download the image and check its hash:

       cd ~/Downloads && curl -fLO https://hc.nikspace.net/nanokvm/sdimg/nanokvm-cube-2.5.14-sd.img.xz && shasum -a 256 nanokvm-cube-2.5.14-sd.img.xz

   It must print `7e21de745cf0abfd44e5b0ee84b6eb77c65150cd9b3359993bc37605f8998224`.
3. Insert the card and open Imager.
4. **Device**: pick any device, or "No filtering".
5. **Operating System**: scroll to the bottom, choose **Use Custom**, and select the `.img.xz`.
6. **Storage**: pick the SD card. Check its size so you don't pick another disk.
7. Press **Next**. If it offers OS customisation, answer **No**. Write it; Imager
   verifies the card and ejects it.

## Option B: Terminal

This uses the raw `.img`, because macOS ships no `xz`.

1. Download the image and check its hash:

       cd ~/Downloads && curl -fLO https://hc.nikspace.net/nanokvm/sdimg/nanokvm-cube-2.5.14-sd.img && shasum -a 256 nanokvm-cube-2.5.14-sd.img

   It must print `a1a826f719313fffd85725ded490a9b16fd6b8720e18338f6c47d61d0d857a48`.
2. Find the card: `diskutil list external physical`. It is the disk the size of your card,
   e.g. `/dev/disk4`. **Be sure of N**: `dd` overwrites whatever disk you give it.
3. Write it, with `N` replaced by that number:

       diskutil unmountDisk /dev/diskN && sudo dd if=nanokvm-cube-2.5.14-sd.img of=/dev/rdiskN bs=4m status=progress && sync && diskutil eject /dev/diskN

## After flashing: set the hostname, the admin seed and (optionally) the root password

The card's small FAT partition (label `boot`) is readable and writable from the Mac, and
the first boot takes its admin password (and optionally root's) from it, so nothing has to
be read off the OLED. The network is DHCP on first boot: the card carries no `eth.nodhcp`.

1. Re-insert the card (the flash ejected it). It mounts as `/Volumes/boot` (macOS adds a
   number, e.g. `boot 1`, if another volume is already named `boot`; use the name you see).
2. On m5sspro (where nik's sage key is), as one line: generate the seed, store it in sage
   FIRST, share it with the rook, only then write it to the card, and eject:

       printf 'kvm1' > /Volumes/boot/hostname && S=$(openssl rand -base64 18) && printf '%s' "$S" | ~/.claude/bin/sage set --stdin secret /nik/devicecred/nanokvm/kvm1/admin && ~/.claude/bin/sage share /nik/devicecred/nanokvm/kvm1/admin rook-agent && ~/.claude/bin/sage sync && ~/.claude/bin/sage get --show /nik/devicecred/nanokvm/kvm1/admin > /Volumes/boot/initial.pass && unset S && sync && diskutil eject /Volumes/boot

   The seed here is 24 characters, but there is no password policy: any non-empty password
   (at most 72 bytes) works, even `a`. `initial.pass` holds just the seed. An empty file is
   ignored and the device falls back to a random seed shown on the OLED.
   Optional: give root its own password instead of the admin seed. Same one line, a second
   secret and file (`root.pass`); a new secret is created by `sage set --stdin secret <path>`
   (the first line above does exactly that for the admin path), and `--show` is needed or
   sage prints a redaction instead of the value:

       R=$(openssl rand -base64 18) && printf '%s' "$R" | ~/.claude/bin/sage set --stdin secret /nik/devicecred/nanokvm/kvm1/root && ~/.claude/bin/sage share /nik/devicecred/nanokvm/kvm1/root rook-agent && ~/.claude/bin/sage sync && ~/.claude/bin/sage get --show /nik/devicecred/nanokvm/kvm1/root > /Volumes/boot/root.pass && unset R && sync

   Run it BEFORE the line above, because that line ejects the card. Without `root.pass`,
   root's password is the admin seed.
3. Put the card back, cable the Cube as before, and power it on. First boot takes a minute
   or two. The server makes `admin` with the seed from `initial.pass`, sets root's password
   from `root.pass` (else the same seed), deletes both files from the card as soon as it has
   used them, and the first web login forces a password change.
4. **kvm1 in our estate: do NOT log in yourself.** Tell the rook. It logs in with the seed
   from sage, sets a new password stored in sage for rookery, captures the device cert, adds
   kvm1 to rookery's and capsd's config, and runs the screenshot and signed/tampered OTA
   checks (journey 01k5oi5052).
5. If the device has a working screen it still shows the seed on the OLED as before. If
   `initial.pass` or `root.pass` is still on the card after a minute or two of running, the
   device could not remove it: delete it yourself and report it.
6. If it never comes up on the network after a few minutes, report it. The image has not been
   boot-tested on hardware before; it cannot run under qemu.
