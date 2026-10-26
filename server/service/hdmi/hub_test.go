package hdmi

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

const testWait = 2 * time.Second

func recv(t *testing.T, sub *Subscription) Snapshot {
	t.Helper()
	select {
	case s := <-sub.C:
		return s
	case <-time.After(testWait):
		t.Fatal("no snapshot delivered")
		return Snapshot{}
	}
}

func TestPublishFansOutToEverySubscriber(t *testing.T) {
	h := New()
	a, b := h.Subscribe(), h.Subscribe()
	defer a.Close()
	defer b.Close()

	want := Snapshot{State: Streaming, Width: 1920, Height: 1080, Seq: 1}
	if !h.Publish(want) {
		t.Fatal("first snapshot rejected")
	}
	if got := recv(t, a); got != want {
		t.Fatalf("a got %+v, want %+v", got, want)
	}
	if got := recv(t, b); got != want {
		t.Fatalf("b got %+v, want %+v", got, want)
	}
}

func TestStaleSeqIsDropped(t *testing.T) {
	h := New()
	h.Publish(Snapshot{State: Streaming, Seq: 5})
	if h.Publish(Snapshot{State: NoSignal, Seq: 5}) || h.Publish(Snapshot{State: NoSignal, Seq: 4}) {
		t.Fatal("snapshot with Seq <= current was accepted")
	}
	if cur, _ := h.Current(); cur.State != Streaming || cur.Seq != 5 {
		t.Fatalf("current %+v changed by a stale publish", cur)
	}
}

func TestLateSubscriberGetsCurrentSnapshot(t *testing.T) {
	h := New()
	h.Publish(Snapshot{State: Locking, Seq: 1})
	h.Publish(Snapshot{State: Streaming, Width: 1280, Height: 720, Seq: 2})

	sub := h.Subscribe()
	defer sub.Close()
	if got := recv(t, sub); got.Seq != 2 || got.State != Streaming || got.Width != 1280 {
		t.Fatalf("late subscriber got %+v, want the seq 2 snapshot", got)
	}
}

func TestEmptyHubPrimesNothing(t *testing.T) {
	h := New()
	if _, ok := h.Current(); ok {
		t.Fatal("empty hub reports a current snapshot")
	}
	sub := h.Subscribe()
	defer sub.Close()
	select {
	case s := <-sub.C:
		t.Fatalf("empty hub delivered %+v", s)
	default:
	}
}

func TestSlowSubscriberSeesLatestInOrder(t *testing.T) {
	h := New()
	sub := h.Subscribe()
	defer sub.Close()

	for seq := uint64(1); seq <= 10; seq++ {
		h.Publish(Snapshot{State: ResChange, Seq: seq})
	}
	if got := recv(t, sub); got.Seq != 10 {
		t.Fatalf("slow subscriber got seq %d, want the latest (10)", got.Seq)
	}
}

func TestSeqOrderingUnderConcurrentPublish(t *testing.T) {
	h := New()
	sub := h.Subscribe()
	defer sub.Close()

	var seen []uint64
	done := make(chan struct{})
	go func() {
		defer close(done)
		for s := range sub.C {
			seen = append(seen, s.Seq)
			if s.Seq == 200 {
				return
			}
		}
	}()

	var wg sync.WaitGroup
	for seq := uint64(1); seq <= 200; seq++ {
		wg.Add(1)
		go func(seq uint64) {
			defer wg.Done()
			h.Publish(Snapshot{Seq: seq})
		}(seq)
	}
	wg.Wait()
	h.Publish(Snapshot{Seq: 200})

	select {
	case <-done:
	case <-time.After(testWait):
		cur, _ := h.Current()
		t.Fatalf("subscriber never saw the final snapshot; current %+v", cur)
	}
	for i := 1; i < len(seen); i++ {
		if seen[i] <= seen[i-1] {
			t.Fatalf("seq went backwards: %v", seen)
		}
	}
}

func TestClosedSubscriptionGetsNothing(t *testing.T) {
	h := New()
	sub := h.Subscribe()
	sub.Close()
	h.Publish(Snapshot{Seq: 1})
	select {
	case s := <-sub.C:
		t.Fatalf("closed subscription got %+v", s)
	default:
	}
}

func TestWaitSettledSkipsTransitionsAndOldSeq(t *testing.T) {
	h := New()
	h.Publish(Snapshot{State: Streaming, Seq: 3})

	go func() {
		h.Publish(Snapshot{State: Locking, Seq: 4})
		h.Publish(Snapshot{State: ResChange, Seq: 5})
		h.Publish(Snapshot{State: Streaming, Width: 1024, Height: 768, Seq: 6})
	}()

	ctx, cancel := context.WithTimeout(context.Background(), testWait)
	defer cancel()
	got, err := h.WaitSettled(ctx, 3)
	if err != nil {
		t.Fatal(err)
	}
	if got.Seq < 4 || (got.State != Streaming && got.State != NoSignal) {
		t.Fatalf("WaitSettled returned %+v", got)
	}
}

func TestWaitSettledHonoursContext(t *testing.T) {
	h := New()
	h.Publish(Snapshot{State: Streaming, Seq: 3})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := h.WaitSettled(ctx, 3); err == nil {
		t.Fatal("WaitSettled returned without a newer snapshot or an error")
	}
}

func TestRunPublishesSourceAndAsksAgainOnTimeout(t *testing.T) {
	h := New()
	calls := []uint64{}
	step := 0
	src := func(last uint64) (Snapshot, WaitResult) {
		calls = append(calls, last)
		step++
		switch step {
		case 1:
			return Snapshot{State: NoSignal, Seq: 1}, WaitChanged
		case 2:
			return Snapshot{}, WaitTimeout
		case 3:
			return Snapshot{State: Streaming, Width: 1920, Height: 1080, Seq: 2}, WaitChanged
		default:
			return Snapshot{}, WaitClosed
		}
	}
	h.Run(context.Background(), src)

	if want := []uint64{0, 1, 1, 2}; len(calls) != len(want) {
		t.Fatalf("source asked with %v, want %v", calls, want)
	} else {
		for i := range want {
			if calls[i] != want[i] {
				t.Fatalf("source asked with %v, want %v", calls, want)
			}
		}
	}
	if cur, _ := h.Current(); cur.Seq != 2 || cur.State != Streaming {
		t.Fatalf("current %+v after run", cur)
	}
}

func TestSocketSendsCurrentThenEachTransition(t *testing.T) {
	h := New()
	h.Publish(Snapshot{State: Streaming, Width: 1920, Height: 1080, Seq: 1})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	path := filepath.Join(t.TempDir(), "hdmi.sock")
	served := make(chan error, 1)
	go func() { served <- h.ServeSocket(ctx, path) }()

	var conn net.Conn
	var err error
	deadline := time.Now().Add(testWait)
	for {
		conn, err = net.Dial("unix", path)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("socket never came up: %v", err)
		}
		select {
		case e := <-served:
			t.Fatalf("ServeSocket returned: %v", e)
		case <-time.After(10 * time.Millisecond):
		}
	}
	defer conn.Close()

	r := bufio.NewReader(conn)
	readSnap := func() Snapshot {
		_ = conn.SetReadDeadline(time.Now().Add(testWait))
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		var s Snapshot
		if err := json.Unmarshal([]byte(line), &s); err != nil {
			t.Fatalf("line %q: %v", line, err)
		}
		return s
	}

	first := readSnap()
	if first.Seq != 1 || first.State != Streaming || first.Width != 1920 {
		t.Fatalf("first line %+v", first)
	}
	h.Publish(Snapshot{State: NoSignal, Width: 1920, Height: 1080, Seq: 2})
	if next := readSnap(); next.Seq != 2 || next.State != NoSignal {
		t.Fatalf("second line %+v", next)
	}
}
