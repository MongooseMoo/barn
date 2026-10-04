package builtins

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func suspendedHTTPWaiter(id int64) httpReadWaiter {
	t := task.NewTask(id, 7, 1000, 5)
	t.SetState(task.TaskSuspended)
	return httpReadWaiter{task: t, kind: "request"}
}

func assertHTTPWaiterSlotsCleared(t *testing.T, slots []httpReadWaiter) {
	t.Helper()
	for i, slot := range slots {
		if slot != (httpReadWaiter{}) {
			t.Errorf("discarded backing slot %d still holds task=%p kind=%q", i, slot.task, slot.kind)
		}
	}
}

func retainedHTTPWaiterState(waiters []httpReadWaiter) (*Session, *httpHeldInput) {
	r := NewSession(NewRegistry(), NoHost())
	r.setConnectionOption(7, "hold-input", types.NewInt(1))
	state := &httpHeldInput{buffer: []byte("partial"), invalidCount: 2, waiters: waiters}
	r.runtime.heldHTTPInput.byPlayer[7] = state
	return r, state
}

func TestHTTPPruneClearsDiscardedBackingSlots(t *testing.T) {
	for _, survivors := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty retained state", true: "surviving waiters"}[survivors], func(t *testing.T) {
			removed := suspendedHTTPWaiter(9010)
			removed.task.SetState(task.TaskKilled)
			backing := []httpReadWaiter{removed, {kind: "response"}, removed}
			var want []httpReadWaiter
			if survivors {
				want = []httpReadWaiter{suspendedHTTPWaiter(9011), suspendedHTTPWaiter(9012)}
				backing = []httpReadWaiter{removed, want[0], {kind: "response"}, want[1], removed}
			}
			r, state := retainedHTTPWaiterState(backing)
			if pending := r.HasPendingHTTPRead(7); pending != survivors {
				t.Fatalf("pending=%v, want %v", pending, survivors)
			}
			if !reflect.DeepEqual(state.waiters, append(backing[:0:0], want...)) {
				t.Fatalf("remaining waiters=%v, want %v", state.waiters, want)
			}
			if r.runtime.heldHTTPInput.byPlayer[7] != state || string(state.buffer) != "partial" || state.invalidCount != 2 {
				t.Fatal("pruning changed retained state, buffer, or invalid-input count")
			}
			assertHTTPWaiterSlotsCleared(t, backing[len(want):])
		})
	}
}

func TestHTTPCancelClearsDiscardedBackingSlots(t *testing.T) {
	for _, survivors := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty retained state", true: "surviving waiters"}[survivors], func(t *testing.T) {
			removed := suspendedHTTPWaiter(9020)
			backing := []httpReadWaiter{removed, removed}
			var want []httpReadWaiter
			if survivors {
				want = []httpReadWaiter{suspendedHTTPWaiter(9021), suspendedHTTPWaiter(9022)}
				backing = []httpReadWaiter{want[0], removed, want[1], removed}
			}
			r, state := retainedHTTPWaiterState(backing)
			before := append([]httpReadWaiter(nil), backing...)
			r.CancelHTTPReadTask(9999)
			if !reflect.DeepEqual(backing, before) || string(state.buffer) != "partial" || state.invalidCount != 2 {
				t.Fatal("cancelling an unrelated task changed state")
			}
			r.CancelHTTPReadTask(removed.task.ID)
			if !reflect.DeepEqual(state.waiters, append(backing[:0:0], want...)) {
				t.Fatalf("remaining waiters=%v, want %v", state.waiters, want)
			}
			if r.runtime.heldHTTPInput.byPlayer[7] != state || state.buffer != nil || state.invalidCount != 0 {
				t.Fatal("cancellation did not retain held state and reset its input")
			}
			assertHTTPWaiterSlotsCleared(t, backing[len(want):])
		})
	}
}

func TestHTTPWakeClearsConsumedBackingSlots(t *testing.T) {
	for _, kind := range []string{"request", "response", "invalid input"} {
		for _, survivors := range []bool{false, true} {
			name := kind + "/" + map[bool]string{false: "empty retained state", true: "surviving waiter"}[survivors]
			t.Run(name, func(t *testing.T) {
				removed := suspendedHTTPWaiter(9030)
				backing := []httpReadWaiter{removed}
				if survivors {
					backing = append(backing, suspendedHTTPWaiter(9031))
				}
				r, state := retainedHTTPWaiterState(backing)
				state.invalidCount = 0
				state.buffer = []byte("GET / HTTP/1.1\r\n\r\n")
				if kind == "response" {
					backing[0].kind = "response"
					state.buffer = []byte("HTTP/1.1 200 OK\r\n\r\n")
				}
				var wantBuffer []byte
				if kind == "invalid input" {
					state.invalidCount = 1
					state.buffer = []byte("partial")
					wantBuffer = []byte("partial")
				} else if survivors {
					wantBuffer = []byte("GET /unfinished")
					state.buffer = append(state.buffer, wantBuffer...)
				}
				r.runtime.heldHTTPInput.mu.Lock()
				wakes := r.collectHTTPWakeupsLocked(7, state)
				r.runtime.heldHTTPInput.mu.Unlock()
				if len(wakes) != 1 || wakes[0].task != removed.task {
					t.Fatalf("wakes=%v, want the consumed task once", wakes)
				}
				if kind == "invalid input" {
					if wakes[0].value.Type() != types.TYPE_INT || wakes[0].value.Int() != 0 {
						t.Fatalf("invalid-input wake=%v, want zero", wakes[0].value)
					}
				} else if kind == "request" {
					if got := mustStringAt(t, mustMapValue(t, wakes[0].value), "method"); got != "GET" {
						t.Fatalf("wake method=%q", got)
					}
				} else if got := mustIntAt(t, mustMapValue(t, wakes[0].value), "status"); got != 200 {
					t.Fatalf("wake status=%d", got)
				}
				if len(state.waiters) != len(backing)-1 || survivors && state.waiters[0] != backing[1] {
					t.Fatal("wake changed the surviving waiter or its order")
				}
				if r.runtime.heldHTTPInput.byPlayer[7] != state || !bytes.Equal(state.buffer, wantBuffer) || state.invalidCount != 0 {
					t.Fatal("wake changed retained state or remaining input")
				}
				assertHTTPWaiterSlotsCleared(t, backing[:1])
			})
		}
	}
}
