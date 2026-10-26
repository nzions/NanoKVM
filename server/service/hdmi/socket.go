package hdmi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	log "github.com/sirupsen/logrus"
)

// SocketPath is where kvm_system reads the snapshots. One JSON line per
// transition, the Snapshot struct: {"state":2,"width":1920,"height":1080,"seq":7}.
const SocketPath = "/var/run/kvmhdmi.sock"

// ServeSocket listens on path until ctx is done. Every connection first gets
// the current snapshot, then one line per transition.
func (h *Hub) ServeSocket(ctx context.Context, path string) error {
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return fmt.Errorf("%s exists and is not a socket", path)
		}
		if err := os.Remove(path); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	ln, err := net.Listen("unix", path)
	if err != nil {
		return err
	}
	if err := os.Chmod(path, 0o660); err != nil {
		_ = ln.Close()
		return err
	}

	go func() {
		<-ctx.Done()
		_ = ln.Close()
		_ = os.Remove(path)
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		go h.serveConn(ctx, conn)
	}
}

const socketWriteTimeout = 5 * time.Second

func (h *Hub) serveConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	sub := h.Subscribe()
	defer sub.Close()

	// a client never writes; EOF on read is how its exit is noticed
	gone := make(chan struct{})
	go func() {
		defer close(gone)
		var buf [1]byte
		for {
			if _, err := conn.Read(buf[:]); err != nil {
				return
			}
		}
	}()

	for {
		select {
		case s := <-sub.C:
			line, err := json.Marshal(s)
			if err != nil {
				log.Errorf("hdmi socket: marshal snapshot: %v", err)
				return
			}
			_ = conn.SetWriteDeadline(time.Now().Add(socketWriteTimeout))
			if _, err := conn.Write(append(line, '\n')); err != nil {
				return
			}
		case <-gone:
			return
		case <-ctx.Done():
			return
		}
	}
}
