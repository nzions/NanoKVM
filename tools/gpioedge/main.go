// gpioedge proves that a gpio-sim line's edges arrive as GPIO chardev v2 line
// events on a file descriptor that poll() wakes on. It is the event source the
// HDMI state machine (libkvm kvm_vision.cpp) would use in place of /proc/lt_int.
//
// It makes a gpio-sim chip through configfs, requests line 0 for both edges,
// toggles the simulated pull from a goroutine and reads the events. Exit 0 only
// when it saw the expected number of alternating rising/falling events.
package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	configfsDir = "/sys/kernel/config"
	simName     = "gpioedge"
	toggles     = 10
	toggleGap   = 100 * time.Millisecond
	waitMS      = 3000

	ioctlWrite = 1
	ioctlRead  = 2
	gpioIoctl  = 0xB4
	getLineNr  = 0x07

	flagInput       = 1 << 2
	flagEdgeRising  = 1 << 4
	flagEdgeFalling = 1 << 5

	eventRising  = 1
	eventFalling = 2
)

type lineAttr struct {
	ID      uint32
	Padding uint32
	Value   uint64
}

type lineConfigAttr struct {
	Attr lineAttr
	Mask uint64
}

type lineConfig struct {
	Flags    uint64
	NumAttrs uint32
	Padding  [5]uint32
	Attrs    [10]lineConfigAttr
}

type lineRequest struct {
	Offsets         [64]uint32
	Consumer        [32]byte
	Config          lineConfig
	NumLines        uint32
	EventBufferSize uint32
	Padding         [5]uint32
	Fd              int32
}

type lineEvent struct {
	TimestampNS uint64
	ID          uint32
	Offset      uint32
	Seqno       uint32
	LineSeqno   uint32
	Padding     [6]uint32
}

func ioctlNr(dir, typ, nr, size uintptr) uintptr {
	return dir<<30 | size<<16 | typ<<8 | nr
}

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "FAIL: "+format+"\n", a...)
}

func write(path, val string) error {
	return os.WriteFile(path, []byte(val), 0o644)
}

func run() (rc int) {
	if _, err := os.Stat(filepath.Join(configfsDir, "gpio-sim")); err != nil {
		if err := syscall.Mount("configfs", configfsDir, "configfs", 0, ""); err != nil {
			fail("configfs has no gpio-sim and mount failed: %v", err)
			return 2
		}
	}
	chip := filepath.Join(configfsDir, "gpio-sim", simName)
	bank := filepath.Join(chip, "bank0")
	if err := os.Mkdir(chip, 0o755); err != nil {
		fail("mkdir %s: %v", chip, err)
		return 2
	}
	defer func() {
		// live=0 first, or configfs refuses the rmdir
		for _, step := range []func() error{
			func() error { return write(filepath.Join(chip, "live"), "0") },
			func() error { return os.Remove(bank) },
			func() error { return os.Remove(chip) },
		} {
			if err := step(); err != nil {
				fail("teardown: %v", err)
				rc = 2
				return
			}
		}
		fmt.Println("teardown: configfs chip removed")
	}()
	if err := os.Mkdir(bank, 0o755); err != nil {
		fail("mkdir %s: %v", bank, err)
		return 2
	}
	if err := write(filepath.Join(bank, "num_lines"), "8"); err != nil {
		fail("num_lines: %v", err)
		return 2
	}
	if err := write(filepath.Join(chip, "live"), "1"); err != nil {
		fail("live=1: %v", err)
		return 2
	}
	nameB, err := os.ReadFile(filepath.Join(bank, "chip_name"))
	if err != nil {
		fail("chip_name: %v", err)
		return 2
	}
	chipName := strings.TrimSpace(string(nameB))
	pulls, _ := filepath.Glob("/sys/devices/platform/gpio-sim.*/" + chipName + "/sim_gpio0/pull")
	if len(pulls) != 1 {
		fail("want one pull file for %s, found %v", chipName, pulls)
		return 2
	}
	pull := pulls[0]
	devPath := "/dev/" + chipName
	fmt.Printf("chip %s, device %s, pull %s\n", chipName, devPath, pull)
	if err := write(pull, "pull-down"); err != nil {
		fail("initial pull-down: %v", err)
		return 2
	}

	dev, err := os.OpenFile(devPath, os.O_RDONLY, 0)
	if err != nil {
		fail("open %s: %v", devPath, err)
		return 2
	}
	defer dev.Close()

	var req lineRequest
	req.Offsets[0] = 0
	copy(req.Consumer[:], "gpioedge")
	req.NumLines = 1
	req.Config.Flags = flagInput | flagEdgeRising | flagEdgeFalling
	nr := ioctlNr(ioctlRead|ioctlWrite, gpioIoctl, getLineNr, unsafe.Sizeof(req))
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, dev.Fd(), nr, uintptr(unsafe.Pointer(&req))); e != 0 {
		fail("GPIO_V2_GET_LINE_IOCTL: %v", e)
		return 2
	}
	lfd := int(req.Fd)
	defer syscall.Close(lfd)
	fmt.Printf("line request ok: fd %d, ioctl %#x, sizeof(request) %d\n", lfd, nr, unsafe.Sizeof(req))

	togDone := make(chan error, 1)
	go func() {
		for i := 0; i < toggles; i++ {
			time.Sleep(toggleGap)
			v := "pull-up"
			if i%2 == 1 {
				v = "pull-down"
			}
			if err := write(pull, v); err != nil {
				togDone <- fmt.Errorf("toggle %d (%s): %w", i, v, err)
				return
			}
		}
		togDone <- nil
	}()

	var last uint32
	seen := 0
	var evSize = int(unsafe.Sizeof(lineEvent{}))
	buf := make([]byte, evSize*16)
	for seen < toggles {
		pfd := []pollFd{{fd: int32(lfd), events: pollIn}}
		n, err := poll(pfd, waitMS)
		if err != nil {
			fail("poll: %v", err)
			return 1
		}
		if n == 0 {
			fail("poll timed out after %d ms with %d of %d events", waitMS, seen, toggles)
			return 1
		}
		got, err := syscall.Read(lfd, buf)
		if err != nil {
			fail("read: %v", err)
			return 1
		}
		if got%evSize != 0 {
			fail("short read %d, event size %d", got, evSize)
			return 1
		}
		for off := 0; off < got; off += evSize {
			var ev lineEvent
			ev.TimestampNS = binary.LittleEndian.Uint64(buf[off:])
			ev.ID = binary.LittleEndian.Uint32(buf[off+8:])
			ev.Offset = binary.LittleEndian.Uint32(buf[off+12:])
			ev.Seqno = binary.LittleEndian.Uint32(buf[off+16:])
			ev.LineSeqno = binary.LittleEndian.Uint32(buf[off+20:])
			name := "?"
			switch ev.ID {
			case eventRising:
				name = "RISING"
			case eventFalling:
				name = "FALLING"
			}
			fmt.Printf("event %2d: %-7s line %d seqno %d line_seqno %d ts_ns %d\n",
				seen+1, name, ev.Offset, ev.Seqno, ev.LineSeqno, ev.TimestampNS)
			if ev.Offset != 0 {
				fail("event on line %d, want 0", ev.Offset)
				return 1
			}
			wantID := uint32(eventRising)
			if seen%2 == 1 {
				wantID = eventFalling
			}
			if ev.ID != wantID {
				fail("event %d: id %d, want %d (not alternating)", seen+1, ev.ID, wantID)
				return 1
			}
			if seen > 0 && ev.LineSeqno != last+1 {
				fail("event %d: line_seqno %d after %d", seen+1, ev.LineSeqno, last)
				return 1
			}
			last = ev.LineSeqno
			seen++
		}
	}
	if err := <-togDone; err != nil {
		fail("%v", err)
		return 1
	}
	fmt.Printf("PASS: %d alternating edge events seen via poll() on the line fd\n", seen)
	return 0
}

func main() {
	os.Exit(run())
}
