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

// A tracked exclusive request records the labelled owner and the requests
// queued in front of it (an untracked exclusive one as unlabeled); a tracked
// shared request granted at once is not blocked, one queued behind the
// exclusive owner is; untracked requests record nothing.
func TestGrantRecordsAheadAndBlocked(t *testing.T) {
	var g Gate
	ctx := context.Background()
	holder, _ := g.AcquireTracked(ctx, Exclusive, "#9655:arm by #0:bf_exec exec()")
	acquire := func(mode Mode, tracked bool, label any) chan *Grant {
		n := g.Queued()
		ch := make(chan *Grant, 1)
		go func() {
			var r *Grant
			if tracked {
				r, _ = g.AcquireTracked(ctx, mode, label)
			} else {
				r, _ = g.Acquire(ctx, mode)
			}
			ch <- r
		}()
		waitQueued(t, &g, n+1)
		return ch
	}
	shared := acquire(Shared, true, nil)
	untrackedShared := acquire(Shared, false, nil)
	queuedWriter := acquire(Exclusive, true, "#2:say by retry budget spent")
	untrackedWriter := acquire(Exclusive, false, nil)
	last := acquire(Exclusive, true, "last")

	holder.Release()
	for _, ch := range []chan *Grant{shared, untrackedShared} {
		s := <-ch
		if want := s.tracked != nil; s.Blocked() != want {
			t.Errorf("shared request (tracked %v) queued behind the exclusive owner: blocked %v", want, s.Blocked())
		}
		s.Release()
	}
	w := <-queuedWriter
	if got := w.Ahead(); got.Holder != "#9655:arm by #0:bf_exec exec()" || got.Depth != 2 || got.Shared != 2 || len(got.Exclusive) != 0 {
		t.Errorf("queued writer ahead = %+v", got)
	}
	w.Release()
	u := <-untrackedWriter
	if got := u.Ahead(); got.Depth != 0 || got.Holder != nil || u.Blocked() {
		t.Errorf("untracked writer recorded ahead %+v blocked %v", got, u.Blocked())
	}
	u.Release()
	l := <-last
	defer l.Release()
	got := l.Ahead()
	if !got.HolderExclusive || got.Holder != "#9655:arm by #0:bf_exec exec()" || got.Depth != 4 ||
		got.Shared != 2 || len(got.Exclusive) != 2 || got.Exclusive[0] != "#2:say by retry budget spent" || got.Exclusive[1] != nil {
		t.Errorf("ahead = %+v", got)
	}
	if holder.Blocked() {
		t.Error("an exclusive grant taken on an idle gate is blocked")
	}
}

// An untracked exclusive owner after a tracked one is unlabeled to a tracked
// request, not named by the earlier owner's label.
func TestUntrackedOwnerIsUnlabeled(t *testing.T) {
	var g Gate
	ctx := context.Background()
	first, _ := g.AcquireTracked(ctx, Exclusive, "first")
	first.Release()
	owner, _ := g.Acquire(ctx, Exclusive)
	ch := make(chan *Grant, 1)
	go func() { r, _ := g.AcquireTracked(ctx, Exclusive, "next"); ch <- r }()
	waitQueued(t, &g, 1)
	owner.Release()
	r := <-ch
	defer r.Release()
	if a := r.Ahead(); !a.HolderExclusive || a.Holder != nil || !r.Blocked() {
		t.Errorf("ahead %+v blocked %v, want an unlabeled exclusive owner", a, r.Blocked())
	}
}

// Ahead looks at no more than maxAheadScan queued requests and names no more
// than MaxAheadLabels exclusive ones; the rest are counted.
func TestAheadIsBounded(t *testing.T) {
	var g Gate
	ctx := context.Background()
	holder, _ := g.AcquireTracked(ctx, Exclusive, "holder")
	queued := maxAheadScan + 5
	grants := make(chan *Grant, queued+1)
	for i := range queued {
		go func() { r, _ := g.AcquireTracked(ctx, Exclusive, i); grants <- r }()
		waitQueued(t, &g, i+1)
	}
	go func() { r, _ := g.AcquireTracked(ctx, Exclusive, "last"); grants <- r }()
	waitQueued(t, &g, queued+1)
	holder.Release()
	for range queued + 1 {
		r := <-grants
		if r.tracked.label == "last" {
			a := r.Ahead()
			if a.Depth != queued || len(a.Exclusive) != MaxAheadLabels || a.ExclusiveMore != maxAheadScan-MaxAheadLabels || a.Unscanned != 5 {
				t.Errorf("ahead depth %d labels %d more %d unscanned %d", a.Depth, len(a.Exclusive), a.ExclusiveMore, a.Unscanned)
			}
		}
		r.Release()
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
