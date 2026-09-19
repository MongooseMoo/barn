package admission

import (
	"context"
	"testing"
	"time"
)

func TestServiceDebtAndReservationsPreventMonopoly(t *testing.T) {
	c := New(Options{Limit: 1, PrincipalLimit: 1, InputWeight: 3, BackgroundWeight: 1})
	a := Key{Principal: 10, Class: Background}
	b := Key{Principal: 20, Class: Input}
	first, err := c.Acquire(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	qa := c.Enqueue(a)
	qb := c.Enqueue(b)
	first.Finish(10*time.Millisecond, 0)
	select {
	case r := <-qb.Ready():
		r.Finish(time.Millisecond, 0)
	default:
		t.Fatal("uncharged principal lost to charged principal")
	}
	select {
	case r := <-qa.Ready():
		r.Finish(time.Millisecond, 0)
	default:
		t.Fatal("background principal did not make progress")
	}
	if s := c.Snapshot(); s.Active != 0 || s.Queued != 0 {
		t.Fatalf("leaked reservations: %+v", s)
	}
}

func TestAdmissionCancellationDoesNotReleaseAnExecutingReservation(t *testing.T) {
	c := New(Options{Limit: 1})
	ctx, cancel := context.WithCancel(context.Background())
	r, err := c.Acquire(ctx, Key{Principal: 1})
	if err != nil {
		t.Fatal(err)
	}
	q := c.Enqueue(Key{Principal: 2})
	cancel()
	select {
	case <-q.Ready():
		t.Fatal("cancellation released a running owner")
	default:
	}
	r.Finish(time.Millisecond, 0)
	(<-q.Ready()).Finish(0, 0)
}

func TestPrincipalCapDoesNotSerializeAllPrincipals(t *testing.T) {
	c := New(Options{Limit: 3, PrincipalLimit: 2})
	r1, _ := c.Acquire(context.Background(), Key{Principal: 1})
	r2, _ := c.Acquire(context.Background(), Key{Principal: 1})
	blocked := c.Enqueue(Key{Principal: 1})
	other := c.Enqueue(Key{Principal: 2})
	select {
	case r := <-other.Ready():
		r.Finish(time.Millisecond, 0)
	default:
		t.Fatal("eligible principal blocked by cap")
	}
	select {
	case <-blocked.Ready():
		t.Fatal("principal cap exceeded")
	default:
	}
	blocked.Cancel()
	r1.Finish(time.Millisecond, 0)
	r2.Finish(time.Millisecond, 0)
}

func TestGateWaitIsNotExecutionService(t *testing.T) {
	c := New(Options{Limit: 1})
	r, _ := c.Acquire(context.Background(), Key{Principal: 1})
	r.Finish(100*time.Millisecond, 99*time.Millisecond)
	s := c.Snapshot()
	if s.Service != time.Millisecond || s.GateWait != 99*time.Millisecond {
		t.Fatalf("accounting = %+v", s)
	}
	r.Finish(time.Second, 0)
	if c.Snapshot().Service != time.Millisecond {
		t.Fatal("duplicate settlement charged twice")
	}
}

func TestWeightedClassesShareServiceWithinOnePrincipal(t *testing.T) {
	c := New(Options{Limit: 1, InputWeight: 3, BackgroundWeight: 1})
	held, _ := c.Acquire(context.Background(), Key{Principal: 1})
	queues := [2][]*Request{}
	for i := 0; i < 400; i++ {
		for cl := Input; cl <= Background; cl++ {
			queues[cl] = append(queues[cl], c.Enqueue(Key{Principal: 1, Class: cl}))
		}
	}
	held.Finish(0, 0)
	counts := [2]int{}
	for i := 0; i < 320; i++ {
		found := false
		for cl := Input; cl <= Background; cl++ {
			select {
			case r := <-queues[cl][counts[cl]].Ready():
				counts[cl]++
				r.Finish(time.Millisecond, 0)
				found = true
			default:
			}
			if found {
				break
			}
		}
		if !found {
			t.Fatal("eligible work not dispatched")
		}
	}
	if counts[Input] < 238 || counts[Input] > 242 {
		t.Fatalf("3:1 share=%v", counts)
	}
	for cl := Input; cl <= Background; cl++ {
		for _, q := range queues[cl][counts[cl]:] {
			if !q.Cancel() {
				(<-q.Ready()).Finish(0, 0)
			}
		}
	}
	if got := c.Snapshot(); got.Active != 0 || got.Queued != 0 {
		t.Fatalf("leak=%+v", got)
	}
}

func TestAdmissionCancellationRacingGrantReclaimsOnlyUnstartedWork(t *testing.T) {
	for i := 0; i < 100; i++ {
		c := New(Options{Limit: 1})
		held, _ := c.Acquire(context.Background(), Key{Principal: 1})
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			defer close(done)
			r, err := c.Acquire(ctx, Key{Principal: 2})
			if err == nil {
				r.Finish(time.Microsecond, 0)
			}
		}()
		cancel()
		held.Finish(0, 0)
		<-done
		if got := c.Snapshot(); got.Active != 0 || got.Queued != 0 {
			t.Fatalf("race leak=%+v", got)
		}
	}
}

func TestPauseDrainsOwnersAndHoldsNewGrantsUntilResume(t *testing.T) {
	c := New(Options{Limit: 1})
	held, _ := c.Acquire(context.Background(), Key{Principal: 1})
	paused := make(chan func(), 1)
	go func() {
		resume, err := c.Pause(context.Background())
		if err != nil {
			panic(err)
		}
		paused <- resume
	}()
	deadline := time.Now().Add(time.Second)
	for {
		c.mu.Lock()
		waiting := c.paused != 0
		c.mu.Unlock()
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("pause not registered")
		}
		time.Sleep(time.Millisecond)
	}
	next := c.Enqueue(Key{Principal: 2})
	select {
	case <-paused:
		t.Fatal("pause ignored active owner")
	default:
	}
	held.Finish(time.Millisecond, 0)
	resume := <-paused
	select {
	case <-next.Ready():
		t.Fatal("grant escaped checkpoint pause")
	default:
	}
	resume()
	resume()
	(<-next.Ready()).Finish(time.Millisecond, 0)
	if got := c.Snapshot(); got.Active != 0 || got.Queued != 0 {
		t.Fatalf("leak=%+v", got)
	}
}

func TestCancelledPauseReopensAdmissionWithoutRevokingOwner(t *testing.T) {
	c := New(Options{Limit: 2})
	held, _ := c.Acquire(context.Background(), Key{Principal: 1})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if resume, err := c.Pause(ctx); err != context.Canceled || resume != nil {
		t.Fatalf("cancelled pause: resume=%v err=%v", resume != nil, err)
	}
	if !held.Active() {
		t.Fatal("pause cancellation revoked active owner")
	}
	next, _ := c.Acquire(context.Background(), Key{Principal: 2})
	next.Finish(0, 0)
	held.Finish(0, 0)
}

func TestYieldLendsReservationToWaiterAndReadmitsOwner(t *testing.T) {
	c := New(Options{Limit: 1})
	owner, err := c.Enter(context.Background(), Key{Principal: 10, Class: Background})
	if err != nil {
		t.Fatal(err)
	}
	owner.Start()
	if waited := owner.Yield(); waited != 0 {
		t.Fatalf("uncontended yield waited %v", waited)
	}
	waiter := c.Enqueue(Key{Principal: 20, Class: Input})
	if waited := owner.Yield(); waited != 0 {
		t.Fatal("yield before a full quantum")
	}
	time.Sleep(Quantum)
	yielded := make(chan time.Duration, 1)
	go func() { yielded <- owner.Yield() }()
	var granted *Reservation
	select {
	case granted = <-waiter.Ready():
	case <-time.After(time.Second):
		t.Fatal("waiter was not granted the yielded reservation")
	}
	if s := c.Snapshot(); s.Active != 1 || s.Preempted != 1 || s.Preemptions != 1 || s.Service <= 0 {
		t.Fatalf("during handoff = %+v", s)
	}
	time.Sleep(5 * time.Millisecond)
	granted.Finish(time.Millisecond, 0)
	select {
	case waited := <-yielded:
		if waited <= 0 {
			t.Fatalf("readmission wait = %v", waited)
		}
	case <-time.After(time.Second):
		t.Fatal("preempted owner was not readmitted")
	}
	owner.Finish()
	if s := c.Snapshot(); s.Active != 0 || s.Preempted != 0 || s.Queued != 0 {
		t.Fatalf("after settlement = %+v", s)
	}
}

// A checkpoint must see every started slice at a real boundary. A preempted
// segment is still in flight: Pause waits for it, and readmits it (while still
// excluding new work) so that the wait can finish.
func TestPauseWaitsForPreemptedSegmentAndReadmitsIt(t *testing.T) {
	c := New(Options{Limit: 1})
	owner, _ := c.Enter(context.Background(), Key{Principal: 10, Class: Background})
	owner.Start()
	other := c.Enqueue(Key{Principal: 20, Class: Input})
	time.Sleep(Quantum)
	yielded := make(chan struct{})
	go func() { owner.Yield(); close(yielded) }()
	granted := <-other.Ready()

	paused := make(chan func(), 1)
	go func() {
		resume, err := c.Pause(context.Background())
		if err != nil {
			t.Error(err)
		}
		paused <- resume
	}()
	for registered := false; !registered; {
		c.mu.Lock()
		registered = c.paused == 1
		c.mu.Unlock()
		time.Sleep(time.Millisecond)
	}
	fresh := c.Enqueue(Key{Principal: 30, Class: Input})
	granted.Finish(time.Millisecond, 0)
	select {
	case <-yielded:
	case <-time.After(time.Second):
		t.Fatal("pause refused to readmit the preempted segment")
	}
	select {
	case <-paused:
		t.Fatal("pause completed while a preempted segment was in flight")
	case <-fresh.Ready():
		t.Fatal("pause admitted new work")
	case <-time.After(20 * time.Millisecond):
	}
	owner.Finish()
	var resume func()
	select {
	case resume = <-paused:
	case <-time.After(time.Second):
		t.Fatal("pause did not complete after the preempted segment finished")
	}
	select {
	case <-fresh.Ready():
		t.Fatal("new work admitted before resume")
	default:
	}
	resume()
	(<-fresh.Ready()).Finish(time.Millisecond, 0)
}
