package wasmbuiltins

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/metrics"
	"github.com/MongooseMoo/barn/types"
	"github.com/samyfodil/wazy/component"
)

func poolManager(t *testing.T, modules ...string) *Manager {
	t.Helper()
	dir := t.TempDir()
	for _, module := range modules {
		data, err := os.ReadFile(filepath.Join("testdata", module+".wasm"))
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, module+".wasm"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	m, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := m.Close(); err != nil {
			t.Error(err)
		}
	})
	for _, module := range modules {
		if _, err = m.Load(context.Background(), module); err != nil {
			t.Fatal(err)
		}
	}
	return m
}

// pooled waits until no guest of module is being reset and returns its pool.
func pooled(t *testing.T, m *Manager, module string) []*component.Instance {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		m.mu.Lock()
		g := m.modules[module]
		busy := g.resetting != 0 || g.refs != 0
		pool := append([]*component.Instance(nil), g.pool...)
		m.mu.Unlock()
		if !busy {
			return pool
		}
		if time.Now().After(deadline) {
			t.Fatal("guest reset never finished")
		}
		time.Sleep(time.Millisecond)
	}
}

func callInt(t *testing.T, m *Manager, principal int64, target string, args ...types.Value) int64 {
	t.Helper()
	r := m.Call(context.Background(), principal, target, args)
	if r.IsError() {
		t.Fatalf("%s: %+v", target, r)
	}
	return r.Val.Int()
}

// A reused guest must show nothing of the previous call: not its linear
// memory (a Rust static, a stored word, a grown memory), not its globals,
// whichever principal made it.
func TestPooledGuestStateIsFresh(t *testing.T) {
	m := poolManager(t, "example", "stateful", "scalar")
	for _, tc := range []struct{ module, export string }{
		{"example", "counter"},
		{"stateful", "bump-global"},
		{"stateful", "bump-memory"},
	} {
		first := pooled(t, m, tc.module)
		if !m.modules[tc.module].resettable || len(first) != 1 {
			t.Fatalf("%s: not pooled: resettable=%v pool=%d", tc.module, m.modules[tc.module].resettable, len(first))
		}
		for i := range 6 {
			if got := callInt(t, m, int64(i%3), tc.module+"."+tc.export); got != 1 {
				t.Fatalf("%s.%s call %d: got %d, want 1", tc.module, tc.export, i, got)
			}
			if pool := pooled(t, m, tc.module); len(pool) != 1 || pool[0] != first[0] {
				t.Fatalf("%s.%s call %d: guest not reused: %v vs %v", tc.module, tc.export, i, pool, first)
			}
		}
	}
	// Memory grown by one call is gone for the next: grow returns the
	// previous size in pages, which is the fixture's two.
	for i := range 3 {
		if got := callInt(t, m, int64(i), "scalar.grow", types.NewInt(1)); got != 2 {
			t.Fatalf("grow call %d: previous size %d pages, want 2", i, got)
		}
		pooled(t, m, "scalar")
	}
}

// A guest whose call trapped or ran past its deadline is closed, never
// returned to the pool, and the next call gets fresh state.
func TestFailedGuestIsNotReused(t *testing.T) {
	m := poolManager(t, "example")
	for _, tc := range []struct {
		export string
		code   types.ErrorCode
	}{{"trap", types.E_INVARG}, {"spin", types.E_QUOTA}} {
		if got := callInt(t, m, 0, "example.counter"); got != 1 {
			t.Fatalf("counter: %d", got)
		}
		before := pooled(t, m, "example")
		if len(before) == 0 {
			t.Fatalf("pool before %s is empty", tc.export)
		}
		used := before[len(before)-1] // the pool hands out its last guest
		if r := m.Call(context.Background(), 0, "example."+tc.export, nil); r.Error != tc.code {
			t.Fatalf("%s: got %+v, want %v", tc.export, r, tc.code)
		}
		if after := pooled(t, m, "example"); len(after) != len(before)-1 || slices.Contains(after, used) {
			t.Fatalf("%s: failed guest returned to the pool", tc.export)
		}
		if _, err := used.Call(context.Background(), "counter"); err == nil {
			t.Fatalf("%s: failed guest still open", tc.export)
		}
		if got := callInt(t, m, 0, "example.counter"); got != 1 {
			t.Fatalf("counter after %s: %d", tc.export, got)
		}
	}
}

// Unloading or replacing a generation closes its pooled guests, including one
// still running or being reset when the generation was retired.
func TestUnloadReleasesPooledGuests(t *testing.T) {
	m := poolManager(t, "example")
	callInt(t, m, 0, "example.counter")
	idle := pooled(t, m, "example")
	g := m.modules["example"]
	if _, err := m.Load(context.Background(), "example"); err != nil {
		t.Fatal(err)
	}
	if g.pool != nil {
		t.Fatal("replaced generation kept its pool")
	}
	if _, err := idle[0].Call(context.Background(), "counter"); err == nil {
		t.Fatal("pooled guest of a replaced generation still open")
	}

	// A call in flight at unload, with a clean guest pooled beside it:
	// both are closed once the call returns.
	callInt(t, m, 1, "example.counter")
	pooled(t, m, "example")
	g = m.modules["example"]
	done := make(chan types.Result, 1)
	go func() { done <- m.Call(context.Background(), 0, "example.spin", nil) }()
	leaseDeadline := time.Now().Add(5 * time.Second)
	for {
		m.mu.Lock()
		leased := g.refs != 0
		m.mu.Unlock()
		if leased {
			break
		}
		if time.Now().After(leaseDeadline) {
			t.Fatal("call never acquired lease")
		}
		time.Sleep(time.Millisecond)
	}
	if !m.Unload("example") {
		t.Fatal("unload")
	}
	if r := <-done; r.Error != types.E_QUOTA {
		t.Fatalf("retired call: %+v", r)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		m.mu.Lock()
		refs, pool := g.refs, g.pool
		m.mu.Unlock()
		if refs == 0 && pool == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("retired generation kept refs=%d pool=%d", refs, len(pool))
		}
		time.Sleep(time.Millisecond)
	}
}

// Close waits for guests being reset off the call path.
func TestCloseWaitsForBackgroundReset(t *testing.T) {
	m := poolManager(t, "example")
	g := m.modules["example"]
	for range 20 {
		callInt(t, m, 0, "example.counter")
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	// Close returned after every reset goroutine finished.
	if g.refs != 0 || g.resetting != 0 || g.pool != nil {
		t.Fatalf("after close: refs=%d resetting=%d pool=%d", g.refs, g.resetting, len(g.pool))
	}
}

// inFlightReset takes module's pooled guest and accounts it as a reset in
// flight, as Call's release does, so a test decides when it finishes with
// m.releaseReset.
func inFlightReset(t *testing.T, m *Manager, module string) (*generation, *component.Instance) {
	t.Helper()
	pool := pooled(t, m, module)
	if len(pool) != 1 {
		t.Fatalf("%s: pool has %d guests, want 1", module, len(pool))
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	g := m.modules[module]
	g.pool = nil
	g.resetting++
	g.refs++
	m.active.Add(1)
	return g, pool[0]
}

// waitRefs waits until g has refs references.
func waitRefs(t *testing.T, m *Manager, g *generation, refs int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		m.mu.Lock()
		n := g.refs
		m.mu.Unlock()
		if n == refs {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("refs=%d, want %d", n, refs)
		}
		time.Sleep(time.Millisecond)
	}
}

func callResult(t *testing.T, done <-chan types.Result) types.Result {
	t.Helper()
	select {
	case r := <-done:
		return r
	case <-time.After(5 * time.Second):
		t.Fatal("call waiting for a reset never returned")
		return types.Result{}
	}
}

// A call waiting for a reset whose guest is refused and closed is woken and
// instantiates a fresh guest instead of waiting forever.
func TestWaitForRefusedReset(t *testing.T) {
	m := poolManager(t, "example")
	g, guest := inFlightReset(t, m, "example")
	done := make(chan types.Result, 1)
	go func() { done <- m.Call(context.Background(), 0, "example.counter", nil) }()
	waitRefs(t, m, g, 2)
	m.releaseReset(g, guest, false)
	if r := callResult(t, done); r.IsError() || r.Val.Int() != 1 {
		t.Fatalf("call after refused reset: %+v", r)
	}
	if _, err := guest.Call(context.Background(), "counter"); err == nil {
		t.Fatal("refused guest still open")
	}
	if pool := pooled(t, m, "example"); len(pool) != 1 || pool[0] == guest {
		t.Fatalf("pool after refused reset: %v", pool)
	}
}

// A call waiting for a reset when its generation is unloaded still completes
// on a fresh guest; the reset guest and the generation are then closed.
func TestWaitForResetAcrossUnload(t *testing.T) {
	m := poolManager(t, "example")
	g, guest := inFlightReset(t, m, "example")
	done := make(chan types.Result, 1)
	go func() { done <- m.Call(context.Background(), 0, "example.counter", nil) }()
	waitRefs(t, m, g, 2)
	if !m.Unload("example") {
		t.Fatal("unload")
	}
	m.releaseReset(g, guest, true)
	if r := callResult(t, done); r.IsError() || r.Val.Int() != 1 {
		t.Fatalf("call across unload: %+v", r)
	}
	waitRefs(t, m, g, 0)
	if _, err := guest.Call(context.Background(), "counter"); err == nil {
		t.Fatal("reset guest of an unloaded generation still open")
	}
	m.mu.Lock()
	pool := g.pool
	m.mu.Unlock()
	if pool != nil {
		t.Fatalf("unloaded generation kept %d guests", len(pool))
	}
}

// A call whose context ends while it waits for a reset returns E_QUOTA and
// leaves the clean guest pooled rather than spending it.
func TestWaitForResetContextEnds(t *testing.T) {
	m := poolManager(t, "example")
	g, guest := inFlightReset(t, m, "example")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan types.Result, 1)
	go func() { done <- m.Call(ctx, 0, "example.counter", nil) }()
	waitRefs(t, m, g, 2)
	cancel()
	m.releaseReset(g, guest, true)
	if r := callResult(t, done); r.Error != types.E_QUOTA {
		t.Fatalf("call with ended context: %+v", r)
	}
	if pool := pooled(t, m, "example"); len(pool) != 1 || pool[0] != guest {
		t.Fatalf("clean guest not left pooled: %v", pool)
	}
	if got := callInt(t, m, 0, "example.counter"); got != 1 {
		t.Fatalf("counter after: %d", got)
	}
}

// Concurrent calls never leave more than PoolGuests guests pooled, and every
// call sees fresh state.
func TestPoolBoundUnderConcurrency(t *testing.T) {
	m := poolManager(t, "example")
	errs := make(chan string, 8)
	for p := range 8 {
		go func() {
			for range 50 {
				r := m.Call(context.Background(), int64(p), "example.counter", nil)
				if r.Error == types.E_QUOTA {
					continue // the eight-call session bound, not a guest fault
				}
				if r.IsError() || r.Val.Int() != 1 {
					errs <- fmt.Sprintf("principal %d: %+v", p, r)
					return
				}
			}
			errs <- ""
		}()
	}
	for range 8 {
		if e := <-errs; e != "" {
			t.Fatal(e)
		}
	}
	if pool := pooled(t, m, "example"); len(pool) == 0 || len(pool) > PoolGuests {
		t.Fatalf("pool holds %d guests, want 1..%d", len(pool), PoolGuests)
	}
}

type panicCloser struct{}

func (panicCloser) Close(context.Context) error { panic("close") }

// closeGuest runs outside any call's recover, so it contains a panic itself.
func TestCloseGuestRecoversPanic(t *testing.T) {
	before := metrics.WASMFaultsRecovered.Value()
	closeGuest(panicCloser{})
	if metrics.WASMFaultsRecovered.Value() != before+1 {
		t.Fatal("close panic not recorded as a recovered fault")
	}
}

// A generation without reset instantiates per call and keeps no guest.
func TestUnresettableGenerationKeepsNoGuest(t *testing.T) {
	m := poolManager(t, "example")
	m.mu.Lock()
	g := m.modules["example"]
	g.resettable = false
	m.mu.Unlock()
	for range 3 {
		if got := callInt(t, m, 0, "example.counter"); got != 1 {
			t.Fatalf("counter: %d", got)
		}
	}
	// The validation guest was pooled at load and is reused once (it is
	// clean); after that each call's guest is closed.
	if pool := pooled(t, m, "example"); len(pool) != 0 {
		t.Fatalf("unresettable generation pooled %d guests", len(pool))
	}
}
