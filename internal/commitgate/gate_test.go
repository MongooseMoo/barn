package commitgate

import (
	"context"
	"runtime"
	"testing"
	"time"
)

func waitQueued(t *testing.T, g *Gate, n int) {
	t.Helper()
	until := time.Now().Add(time.Second)
	for g.Queued() != n {
		if time.Now().After(until) {
			t.Fatalf("queued = %d, want %d", g.Queued(), n)
		}
		runtime.Gosched()
	}
}

// An exclusive request records the labelled owner and the requests queued in
// front of it; a shared request granted at once is not blocked, one queued
// behind the exclusive owner is.
func TestGrantRecordsAheadAndBlocked(t *testing.T) {
	var g Gate
	holder, _ := g.AcquireLabeled(context.Background(), Exclusive, "#9655:arm by #0:bf_exec exec()")
	shared := make(chan *Grant, 1)
	go func() { r, _ := g.Acquire(context.Background(), Shared); shared <- r }()
	waitQueued(t, &g, 1)
	queuedWriter := make(chan *Grant, 1)
	go func() {
		r, _ := g.AcquireLabeled(context.Background(), Exclusive, "#2:say by retry budget spent")
		queuedWriter <- r
	}()
	waitQueued(t, &g, 2)
	last := make(chan *Grant, 1)
	go func() { r, _ := g.AcquireLabeled(context.Background(), Exclusive, "last"); last <- r }()
	waitQueued(t, &g, 3)

	holder.Release()
	s := <-shared
	if !s.Blocked() {
		t.Error("shared request queued behind the exclusive owner is not blocked")
	}
	s.Release()
	w := <-queuedWriter
	w.Release()
	l := <-last
	defer l.Release()
	want := Ahead{HolderExclusive: true, Holder: "#9655:arm by #0:bf_exec exec()", Depth: 2,
		Exclusive: []string{"#2:say by retry budget spent"}, Shared: 1}
	got := l.Ahead()
	if got.HolderExclusive != want.HolderExclusive || got.Holder != want.Holder || got.Depth != want.Depth ||
		got.Shared != want.Shared || len(got.Exclusive) != 1 || got.Exclusive[0] != want.Exclusive[0] {
		t.Errorf("ahead = %+v, want %+v", got, want)
	}
	if holder.Blocked() {
		t.Error("an exclusive grant taken on an idle gate is blocked")
	}
}

func TestFIFOSharedCohortsCannotPassOlderExclusive(t *testing.T) {
	var g Gate
	holder, _ := g.Acquire(context.Background(), Exclusive)
	first := make(chan *Grant, 1)
	writer := make(chan *Grant, 1)
	last := make(chan *Grant, 1)
	go func() { r, _ := g.Acquire(context.Background(), Shared); first <- r }()
	waitQueued(t, &g, 1)
	go func() { r, _ := g.Acquire(context.Background(), Exclusive); writer <- r }()
	waitQueued(t, &g, 2)
	go func() { r, _ := g.Acquire(context.Background(), Shared); last <- r }()
	waitQueued(t, &g, 3)
	holder.Release()
	r := <-first
	select {
	case <-writer:
		t.Fatal("writer overlapped reader")
	default:
	}
	select {
	case <-last:
		t.Fatal("reader overtook writer")
	default:
	}
	r.Release()
	w := <-writer
	select {
	case <-last:
		t.Fatal("reader overlapped writer")
	default:
	}
	w.Release()
	(<-last).Release()
	if g.Queued() != 0 {
		t.Fatal("queue not drained")
	}
}

func TestCancellationDoesNotUnlockAnOwner(t *testing.T) {
	var g Gate
	ctx, cancel := context.WithCancel(context.Background())
	holder, _ := g.Acquire(ctx, Exclusive)
	cancel()
	waitCtx, stop := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := g.Acquire(waitCtx, Shared); done <- err }()
	waitQueued(t, &g, 1)
	stop()
	if err := <-done; err != context.Canceled {
		t.Fatalf("cancel = %v", err)
	}
	if g.Queued() != 0 {
		t.Fatal("cancelled waiter retained")
	}
	if !holder.Release() || holder.Release() {
		t.Fatal("release must consume ownership exactly once")
	}
	next, err := g.Acquire(context.Background(), Exclusive)
	if err != nil {
		t.Fatal(err)
	}
	next.Release()
}

func TestSharedCohortAndCancelledWriter(t *testing.T) {
	var g Gate
	first, _ := g.Acquire(context.Background(), Shared)
	ctx, cancel := context.WithCancel(context.Background())
	writer := make(chan error, 1)
	go func() { _, err := g.Acquire(ctx, Exclusive); writer <- err }()
	waitQueued(t, &g, 1)
	reader := make(chan *Grant, 1)
	go func() { r, _ := g.Acquire(context.Background(), Shared); reader <- r }()
	waitQueued(t, &g, 2)
	cancel()
	if err := <-writer; err != context.Canceled {
		t.Fatal(err)
	}
	// Removing the writer joins this reader to the existing shared cohort.
	second := <-reader
	if !first.Owns(&g, Shared) || !second.Owns(&g, Shared) {
		t.Fatal("shared cohort not active")
	}
	second.Release()
	first.Release()
}

func TestGrantCancellationRaceLeavesGateReusable(t *testing.T) {
	for i := 0; i < 100; i++ {
		var g Gate
		held, _ := g.Acquire(context.Background(), Exclusive)
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			defer close(done)
			r, err := g.Acquire(ctx, Shared)
			if err == nil {
				r.Release()
			}
		}()
		cancel()
		held.Release()
		<-done
		ctx2, stop := context.WithTimeout(context.Background(), time.Second)
		next, err := g.Acquire(ctx2, Exclusive)
		stop()
		if err != nil {
			t.Fatal(err)
		}
		next.Release()
	}
}
