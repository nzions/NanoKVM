package main

import (
	"syscall"
	"unsafe"
)

const pollIn = 0x1

type pollFd struct {
	fd      int32
	events  int16
	revents int16
}

// poll waits on fds with ppoll(2): riscv64 has no poll(2) syscall, and ppoll
// is the same wait with a timespec timeout.
func poll(fds []pollFd, timeoutMS int) (int, error) {
	ts := syscall.Timespec{Sec: int64(timeoutMS / 1000), Nsec: int64(timeoutMS%1000) * 1000000}
	for {
		r, _, e := syscall.Syscall6(syscall.SYS_PPOLL, uintptr(unsafe.Pointer(&fds[0])), uintptr(len(fds)),
			uintptr(unsafe.Pointer(&ts)), 0, 0, 0)
		if e == syscall.EINTR {
			continue
		}
		if e != 0 {
			return 0, e
		}
		return int(r), nil
	}
}
