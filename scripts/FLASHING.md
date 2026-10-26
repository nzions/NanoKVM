# Flashing a NanoKVM Cube microSD card from macOS

These steps flash the image that `sd-image.sh` builds: the stock Sipeed 1.4.3 image with our
signed release in `/kvmapp`. The commands are for the current published build, 2.5.13.
For another build, take the file names and hashes from `SHA256SUMS`.

Published at https://hc.nikspace.net/nanokvm/sdimg/:

| file | size | sha256 |
|---|---|---|
| `nanokvm-cube-2.5.13-sd.img.xz` | 195,671,592 B | `8ae6ee85b05a6f8563f2651824b1cfa1811285a0ee6f309b53b156066287742a` |
| `nanokvm-cube-2.5.13-sd.img` | 1,627,390,464 B | `626660700ddc48575436dccf78ab4c8891da233dfb8d65380778b78f5db2d96d` |

You need a microSD card reader on the Mac.

## 0. Get the card out

Power the Cube off. The microSD slot may be inside the case, so you may need to take the
cover off to reach it.

## Option A: Raspberry Pi Imager (easiest)

1. Install it: `brew install --cask raspberry-pi-imager`, or get it from https://www.raspberrypi.com/software/.
2. Download the image and check its hash:

       cd ~/Downloads && curl -fLO https://hc.nikspace.net/nanokvm/sdimg/nanokvm-cube-2.5.13-sd.img.xz && shasum -a 256 nanokvm-cube-2.5.13-sd.img.xz

   It must print `8ae6ee85b05a6f8563f2651824b1cfa1811285a0ee6f309b53b156066287742a`.
3. Insert the card and open Imager.
4. **Device**: pick any device, or "No filtering".
5. **Operating System**: scroll to the bottom, choose **Use Custom**, and select the `.img.xz`.
6. **Storage**: pick the SD card. Check its size so you don't pick another disk.
7. Press **Next**. If it offers OS customisation, answer **No**. Write it; Imager
   verifies the card and ejects it.

## Option B: Terminal

This uses the raw `.img`, because macOS ships no `xz`.

1. Download the image and check its hash:

       cd ~/Downloads && curl -fLO https://hc.nikspace.net/nanokvm/sdimg/nanokvm-cube-2.5.13-sd.img && shasum -a 256 nanokvm-cube-2.5.13-sd.img

   It must print `626660700ddc48575436dccf78ab4c8891da233dfb8d65380778b78f5db2d96d`.
2. Find the card: `diskutil list external physical`. It is the disk the size of your card,
   e.g. `/dev/disk4`. **Be sure of N**: `dd` overwrites whatever disk you give it.
3. Write it, with `N` replaced by that number:

       diskutil unmountDisk /dev/diskN && sudo dd if=nanokvm-cube-2.5.13-sd.img of=/dev/rdiskN bs=4m status=progress && sync && diskutil eject /dev/diskN

## After flashing: set the hostname and the admin seed

The card's small FAT partition (label `boot`) is readable and writable from the Mac, and
the first boot takes its admin password from it, so nothing has to be read off the OLED.

1. Re-insert the card (the flash ejected it). It mounts as `/Volumes/boot` (macOS adds a
   number, e.g. `boot 1`, if another volume is already named `boot`; use the name you see).
2. On m5sspro (where nik's sage key is), as one line: generate the seed, store it in sage
   FIRST, share it with the rook, only then write it to the card, and eject:

       printf 'kvm1' > /Volumes/boot/hostname && S=$(openssl rand -base64 18) && printf '%s' "$S" | ~/.claude/bin/sage set --stdin secret /nik/devicecred/nanokvm/kvm1/admin && ~/.claude/bin/sage share /nik/devicecred/nanokvm/kvm1/admin rook-agent && ~/.claude/bin/sage sync && ~/.claude/bin/sage get --show /nik/devicecred/nanokvm/kvm1/admin > /Volumes/boot/initial.pass && unset S && sync && diskutil eject /Volumes/boot

   The seed is 24 characters (at least 12 are required). `initial.pass` holds just the
   seed. If the file is shorter than 12 characters the device ignores it and falls back to
   a random seed shown on the OLED.
3. Put the card back, cable the Cube as before, and power it on. First boot takes a minute
   or two. The server makes `admin` with the seed from `initial.pass` (also root's password),
   deletes the file from the card as soon as it has used it, and the first web login forces
   a password change.
4. **kvm1 in our estate: do NOT log in yourself.** Tell the rook. It logs in with the seed
   from sage, sets a new password stored in sage for rookery, captures the device cert, adds
   kvm1 to rookery's and capsd's config, and runs the screenshot and signed/tampered OTA
   checks (journey 01k5oi5052).
5. If the device has a working screen it still shows the seed on the OLED as before. If
   `initial.pass` is still on the card after a minute or two of running, the device could
   not remove it: delete it yourself and report it.
6. If it never comes up on the network after a few minutes, report it. The image has not been
   boot-tested on hardware before; it cannot run under qemu.
