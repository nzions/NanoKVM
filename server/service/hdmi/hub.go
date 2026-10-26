// Package hdmi owns the HDMI input state of the device: one immutable
// Snapshot per transition, published by libkvm's state machine, fanned out to
// every consumer (websocket clients, the kvm_system socket, capture waiters).
package hdmi

import (
	"context"
	"sync"
)

// State is the libkvm HDMI state machine state. The values are the wire
// values and equal enum kvm_hdmi_state in kvm_hdmi.h.
type State uint8

const (
	NoSignal State = iota
	Locking
	Streaming
	ResChange
)

func (s State) String() string {
	switch s {
	case NoSignal:
		return "no-signal"
	case Locking:
		return "locking"
	case Streaming:
		return "streaming"
	case ResChange:
		return "res-change"
	default:
		return "unknown"
	}
}

// Snapshot is one HDMI state transition. Seq increases by one per transition
// in libkvm; Width and Height are the resolution the VI chain is (or is being)
// initialised at, 0 until a signal was ever locked.
type Snapshot struct {
	State  State  `json:"state"`
	Width  uint16 `json:"width"`
	Height uint16 `json:"height"`
	Seq    uint64 `json:"seq"`
}

// WaitResult is the outcome of one blocking wait on the libkvm snapshot.
type WaitResult uint8

const (
	// WaitChanged: a snapshot newer than the one asked for was returned.
	WaitChanged WaitResult = iota
	// WaitTimeout: nothing changed in the interval. It says nothing else.
	WaitTimeout
	// WaitClosed: libkvm is shut down, no further snapshot will come.
	WaitClosed
)

// Source blocks until a snapshot newer than lastSeq exists, a timeout passes
// or the source closes.
type Source func(lastSeq uint64) (Snapshot, WaitResult)

// Subscription receives snapshots. C holds at most one, the latest the
// subscriber has not read yet: a slow reader skips transitions, never sees
// them out of order. The current snapshot, if any, is already in C when
// Subscribe returns.
type Subscription struct {
	C   <-chan Snapshot
	ch  chan Snapshot
	hub *Hub
}

// Close detaches the subscription. C is not closed.
func (s *Subscription) Close() {
	s.hub.mu.Lock()
	defer s.hub.mu.Unlock()
	for i, other := range s.hub.subs {
		if other == s {
			s.hub.subs = append(s.hub.subs[:i], s.hub.subs[i+1:]...)
			return
		}
	}
}

// Hub holds the current snapshot and its subscribers.
type Hub struct {
	mu   sync.Mutex
	cur  Snapshot
	have bool
	subs []*Subscription
}

func New() *Hub {
	return &Hub{}
}

// Publish stores s and delivers it to every subscriber. A snapshot whose Seq
// is not above the current one is stale and dropped (false).
func (h *Hub) Publish(s Snapshot) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.have && s.Seq <= h.cur.Seq {
		return false
	}
	h.cur = s
	h.have = true
	for _, sub := range h.subs {
		deliver(sub.ch, s)
	}
	return true
}

// deliver puts s in the one-slot channel, replacing an unread older snapshot.
// Only Publish sends, and it holds the hub lock, so the send cannot block.
func deliver(ch chan Snapshot, s Snapshot) {
	select {
	case ch <- s:
		return
	default:
	}
	select {
	case <-ch:
	default:
	}
	ch <- s
}

// Current is the latest snapshot; false until the first one was published.
func (h *Hub) Current() (Snapshot, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.cur, h.have
}

// Subscribe attaches a subscriber and primes it with the current snapshot.
func (h *Hub) Subscribe() *Subscription {
	h.mu.Lock()
	defer h.mu.Unlock()

	ch := make(chan Snapshot, 1)
	sub := &Subscription{C: ch, ch: ch, hub: h}
	if h.have {
		ch <- h.cur
	}
	h.subs = append(h.subs, sub)
	return sub
}

// WaitSettled blocks until a snapshot with Seq above after reports Streaming
// or NoSignal, the two states that answer "is there a picture". Locking and
// ResChange are transitions on the way to one of them.
func (h *Hub) WaitSettled(ctx context.Context, after uint64) (Snapshot, error) {
	sub := h.Subscribe()
	defer sub.Close()

	for {
		select {
		case s := <-sub.C:
			if s.Seq > after && (s.State == Streaming || s.State == NoSignal) {
				return s, nil
			}
		case <-ctx.Done():
			return Snapshot{}, ctx.Err()
		}
	}
}

// Run publishes what src returns until ctx is done or src closes. A timeout
// from src is "no change" and the wait is simply asked again.
func (h *Hub) Run(ctx context.Context, src Source) {
	var last uint64
	for ctx.Err() == nil {
		s, res := src(last)
		switch res {
		case WaitChanged:
			if h.Publish(s) {
				last = s.Seq
			}
		case WaitTimeout:
		case WaitClosed:
			return
		}
	}
}

var defaultHub = New()

// Default is the process-wide hub the server uses.
func Default() *Hub {
	return defaultHub
}
