#!/bin/sh
# Sets the hostname (nanokvm-<id>, or /boot/hostname). NOT an S??* script: rcS
# does not run it. S30eth runs it in a background subshell (no `timeout`: the
# image has none; this script has no wait or loop) AFTER networking was
# launched, so it can never block or alter DHCP.
# --- hostname ---------------------------------------------------------------
# /boot/hostname (non-empty) wins and is applied unchanged; the web UI writes it
# too, so a name set there persists and beats the automatic one. With none, the
# name is nanokvm-<id>, <id> being the last 4 characters of the board serial
# (SoC unique id, then device-tree serial-number, then the /proc/cpuinfo Serial)
# and failing that the last 4 hex digits of eth0's MAC. An id that is empty, all
# zeros or all f does not count. It runs after udhcpc was launched and races avahi,
# so mDNS picking the name up is best-effort. Nothing is written to /boot: idempotent.
HN_FILE=/boot/hostname
HN_ETC=/etc/hostname
AVAHI_CONF=/etc/avahi/avahi-daemon.conf

# hn_id4 STRING prints the last 4 of [0-9a-z] in STRING, lowercased, and fails
# when there are fewer than 4 or they are all 0 or all f
hn_id4() {
    _s=$(printf '%s' "$1" | tr 'A-Z' 'a-z' | tr -cd '0-9a-z')
    [ "${#_s}" -ge 4 ] || return 1
    _s=${_s#"${_s%????}"}
    case "$_s" in
        0000|ffff) return 1 ;;
    esac
    printf '%s' "$_s"
}

hn_serial() {
    _raw=""
    if [ -r /sys/class/cvi-base/base_uid ]; then
        # "<label> <hi>_<lo>": the id is the second word
        _raw=$(awk 'NR == 1 { print ($2 != "" ? $2 : $1) }' /sys/class/cvi-base/base_uid)
        hn_id4 "$_raw" && return 0
    fi
    for _f in /proc/device-tree/serial-number /sys/firmware/devicetree/base/serial-number; do
        if [ -r "$_f" ]; then
            _raw=$(tr -d '\000' < "$_f")
            hn_id4 "$_raw" && return 0
        fi
    done
    if [ -r /proc/cpuinfo ]; then
        _raw=$(sed -n 's/^Serial[[:space:]]*:[[:space:]]*//p' /proc/cpuinfo)
        hn_id4 "$_raw" && return 0
    fi
    return 1
}

hn_mac() {
    [ -r /sys/class/net/eth0/address ] || return 1
    _raw=$(cat /sys/class/net/eth0/address)
    case "$_raw" in
        00:00:00:00:00:00|ff:ff:ff:ff:ff:ff) return 1 ;;
    esac
    hn_id4 "$(printf '%s' "$_raw" | tr -d ':')"
}

set_hostname() {
    _want=""
    if [ -s "$HN_FILE" ]; then
        _want=$(tr -d '\r\n' < "$HN_FILE")
    fi
    if [ -z "$_want" ]; then
        _id=$(hn_serial) || _id=$(hn_mac) || _id=""
        if [ -z "$_id" ]; then
            echo "S30eth: no board serial and no eth0 MAC: keeping the hostname $(hostname)" >&2
            return 0
        fi
        _want="nanokvm-$_id"
    fi
    _old=$(tr -d '\r\n' < "$HN_ETC" 2>/dev/null)
    if [ "$_old" != "$_want" ]; then
        printf '%s\n' "$_want" > "$HN_ETC.tmp" && mv "$HN_ETC.tmp" "$HN_ETC"
        # keep /etc/hosts resolving the old name, as the web UI does
        if [ -n "$_old" ] && [ -w /etc/hosts ] && grep -qF "$_old" /etc/hosts; then
            sed "s/$_old/$_want/g" /etc/hosts > /etc/hosts.tmp && cat /etc/hosts.tmp > /etc/hosts
            rm -f /etc/hosts.tmp
        fi
    fi
    hostname "$_want"
    # avahi advertises the kernel hostname unless its config pins one
    if [ -f "$AVAHI_CONF" ] && grep -q '^host-name=' "$AVAHI_CONF"; then
        sed "s/^host-name=.*/host-name=$_want/" "$AVAHI_CONF" > "$AVAHI_CONF.tmp" && cat "$AVAHI_CONF.tmp" > "$AVAHI_CONF"
        rm -f "$AVAHI_CONF.tmp"
    fi
}

set_hostname
