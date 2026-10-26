// Package gpio drives board GPIO lines by their device-tree line NAME over the
// GPIO character device (uABI v2). The vendor kernel's global sysfs numbers
// (503, 505, ...) differ on the mainline kernel, so nothing here knows one: a
// line is found by the gpio-line-names entry the board DT provides (pwr-key,
// rst-key, pwr-led, ...). A name no chip carries is ErrNoLine (the caller may
// then use the vendor sysfs path, because there is no named line); a name two
// lines carry, or any failing chip, is an error and never a guess.
//
// Lines are requested for one operation and released: held outputs (hdmi-rst,
// oled-rst, wifi-en) belong to the kvmgpio owner process, which this package
// never touches.
package gpio

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// ErrNoLine: every gpiochip was read and none carries the name.
var ErrNoLine = errors.New("no gpio line with that name")

// devGlob is where the chips are; a variable so the lookup can be pointed at a
// different directory.
var devGlob = "/dev/gpiochip*"

const (
	ioctlType = 0xB4
	maxName   = 32
	maxAttrs  = 10
	maxLines  = 64

	iocWrite = 1
	iocRead  = 2

	lineFlagInput  = 1 << 2
	lineFlagOutput = 1 << 3

	attrIDOutputValues = 2
)

type chipInfo struct {
	Name  [maxName]byte
	Label [maxName]byte
	Lines uint32
}

type lineAttribute struct {
	ID      uint32
	Padding uint32
	Value   uint64
}

type lineConfigAttribute struct {
	Attr lineAttribute
	Mask uint64
}

type lineConfig struct {
	Flags    uint64
	NumAttrs uint32
	Padding  [5]uint32
	Attrs    [maxAttrs]lineConfigAttribute
}

type lineRequest struct {
	Offsets         [maxLines]uint32
	Consumer        [maxName]byte
	Config          lineConfig
	NumLines        uint32
	EventBufferSize uint32
	Padding         [5]uint32
	Fd              int32
}

type lineInfo struct {
	Name     [maxName]byte
	Consumer [maxName]byte
	Offset   uint32
	NumAttrs uint32
	Flags    uint64
	Attrs    [maxAttrs]lineAttribute
	Padding  [4]uint32
}

type lineValues struct {
	Bits uint64
	Mask uint64
}

// The kernel's struct sizes (linux/gpio.h, v2). A layout that drifts fails the
// build instead of corrupting a request.
var (
	_ [68 - unsafe.Sizeof(chipInfo{})]struct{}
	_ [unsafe.Sizeof(chipInfo{}) - 68]struct{}
	_ [592 - unsafe.Sizeof(lineRequest{})]struct{}
	_ [unsafe.Sizeof(lineRequest{}) - 592]struct{}
	_ [256 - unsafe.Sizeof(lineInfo{})]struct{}
	_ [unsafe.Sizeof(lineInfo{}) - 256]struct{}
	_ [16 - unsafe.Sizeof(lineValues{})]struct{}
	_ [unsafe.Sizeof(lineValues{}) - 16]struct{}
)

func ioc(dir, nr, size uintptr) uintptr {
	return dir<<30 | size<<16 | ioctlType<<8 | nr
}

var (
	ioctlChipInfo  = ioc(iocRead, 0x01, unsafe.Sizeof(chipInfo{}))
	ioctlLineInfo  = ioc(iocRead|iocWrite, 0x05, unsafe.Sizeof(lineInfo{}))
	ioctlGetLine   = ioc(iocRead|iocWrite, 0x07, unsafe.Sizeof(lineRequest{}))
	ioctlGetValues = ioc(iocRead|iocWrite, 0x0E, unsafe.Sizeof(lineValues{}))
	ioctlSetValues = ioc(iocRead|iocWrite, 0x0F, unsafe.Sizeof(lineValues{}))
)

func ioctl(fd int, req uintptr, arg unsafe.Pointer) error {
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), req, uintptr(arg)); errno != 0 {
		return errno
	}
	return nil
}

func cString(b []byte) string {
	if i := strings.IndexByte(string(b), 0); i >= 0 {
		return string(b[:i])
	}
	return string(b)
}

// find returns the chip path and offset of the line called name.
func find(name string) (string, uint32, error) {
	if name == "" || len(name) >= maxName {
		return "", 0, fmt.Errorf("gpio: invalid line name %q", name)
	}
	chips, err := filepath.Glob(devGlob)
	if err != nil {
		return "", 0, fmt.Errorf("gpio: %w", err)
	}
	var (
		foundPath string
		foundLine uint32
		matches   int
	)
	for _, path := range chips {
		fd, err := unix.Open(path, unix.O_RDWR|unix.O_CLOEXEC, 0)
		if err != nil {
			return "", 0, fmt.Errorf("gpio: open %s: %w", path, err)
		}
		var ci chipInfo
		if err := ioctl(fd, ioctlChipInfo, unsafe.Pointer(&ci)); err != nil {
			_ = unix.Close(fd)
			return "", 0, fmt.Errorf("gpio: chip info %s: %w", path, err)
		}
		for line := uint32(0); line < ci.Lines; line++ {
			li := lineInfo{Offset: line}
			if err := ioctl(fd, ioctlLineInfo, unsafe.Pointer(&li)); err != nil {
				_ = unix.Close(fd)
				return "", 0, fmt.Errorf("gpio: line info %s %d: %w", path, line, err)
			}
			if cString(li.Name[:]) == name {
				matches++
				foundPath, foundLine = path, line
			}
		}
		_ = unix.Close(fd)
	}
	switch {
	case matches > 1:
		return "", 0, fmt.Errorf("gpio: line %q is named %d times: refusing to pick one", name, matches)
	case matches == 0:
		return "", 0, fmt.Errorf("%w: %q (%d chip(s) read)", ErrNoLine, name, len(chips))
	}
	return foundPath, foundLine, nil
}

// request takes the named line; the returned fd is the line (close it to
// release). For an output, value is the initial level.
func request(name string, output bool, value int) (int, error) {
	path, offset, err := find(name)
	if err != nil {
		return -1, err
	}
	cfd, err := unix.Open(path, unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, fmt.Errorf("gpio: open %s: %w", path, err)
	}
	defer unix.Close(cfd)

	var req lineRequest
	req.Offsets[0] = offset
	copy(req.Consumer[:], "nanokvm-server")
	req.NumLines = 1
	if output {
		req.Config.Flags = lineFlagOutput
		req.Config.NumAttrs = 1
		req.Config.Attrs[0].Attr = lineAttribute{ID: attrIDOutputValues, Value: uint64(value & 1)}
		req.Config.Attrs[0].Mask = 1
	} else {
		req.Config.Flags = lineFlagInput
	}
	if err := ioctl(cfd, ioctlGetLine, unsafe.Pointer(&req)); err != nil {
		return -1, fmt.Errorf("gpio: request %q (%s offset %d): %w", name, path, offset, err)
	}
	return int(req.Fd), nil
}

// Exists reports whether a line carries the name: nil, ErrNoLine, or the
// error that stopped the lookup.
func Exists(name string) error {
	_, _, err := find(name)
	return err
}

// Pulse drives the named output high for d, then low, and releases it.
func Pulse(name string, d time.Duration) error {
	fd, err := request(name, true, 1)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	time.Sleep(d)
	v := lineValues{Bits: 0, Mask: 1}
	if err := ioctl(fd, ioctlSetValues, unsafe.Pointer(&v)); err != nil {
		return fmt.Errorf("gpio: release %q: %w", name, err)
	}
	return nil
}

// Read returns the level of the named line (true = high).
func Read(name string) (bool, error) {
	fd, err := request(name, false, 0)
	if err != nil {
		return false, err
	}
	defer unix.Close(fd)
	v := lineValues{Mask: 1}
	if err := ioctl(fd, ioctlGetValues, unsafe.Pointer(&v)); err != nil {
		return false, fmt.Errorf("gpio: read %q: %w", name, err)
	}
	return v.Bits&1 == 1, nil
}

// SysfsRead reads a vendor sysfs value file ("0"/"1").
func SysfsRead(path string) (bool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	s := strings.TrimSpace(string(b))
	switch s {
	case "0":
		return false, nil
	case "1":
		return true, nil
	}
	return false, fmt.Errorf("gpio: %s holds %q", path, s)
}

// SysfsPulse is the vendor path: 1, wait d, 0.
func SysfsPulse(path string, d time.Duration) error {
	if err := os.WriteFile(path, []byte("1"), 0o666); err != nil {
		return err
	}
	time.Sleep(d)
	return os.WriteFile(path, []byte("0"), 0o666)
}
