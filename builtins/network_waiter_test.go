package builtins

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func suspendedHTTPWaiter(id int64) httpReadWaiter {
	t := task.NewState(id, 7, 1000, 5)
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
	r := ctxWithConnManager(&stubConnManager{conn: &stubConn{id: 7}}).Session
	r.setConnectionOption(7, nil, "hold-input", types.NewInt(1))
	state := &httpHeldInput{buffer: []byte("partial"), invalidCount: 2, waiters: waiters}
	r.runtime.heldHTTPInput.byPlayer[7] = state
	return r, state
}

type optionProbeConnManager struct {
	stubConnManager
	lookups int
}

func (m *optionProbeConnManager) GetConnection(player types.ObjID) Connection {
	m.lookups++
	return m.stubConnManager.GetConnection(player)
}

func TestHeldInputUsesProducerIDWithoutManagerLookup(t *testing.T) {
	const connectionID int64 = 91
	manager := &optionProbeConnManager{stubConnManager: stubConnManager{conn: &stubConn{id: connectionID}}}
	session := ctxWithConnManager(manager).Session
	if handled, _ := session.HandleHeldInput(7, connectionID, "ordinary input", false); handled {
		t.Fatal("default input was held")
	}
	session.setConnectionOption(connectionID, nil, "hold-input", types.NewInt(1))
	request := "GET / HTTP/1.1~0D~0A~0D~0A"
	if handled, _ := session.HandleHeldInput(7, connectionID, request, false); !handled {
		t.Fatal("producer's hold-input option was ignored")
	}
	if value, complete := session.prepareHTTPRead(7, connectionID, "request", nil); !complete || value.Type() != types.TYPE_MAP {
		t.Fatalf("buffered read = %v, complete=%v", value, complete)
	}
	session.setConnectionOption(connectionID, nil, "flush-command", types.NewStr(".flush"))
	if handled, lines := session.HandleHeldInput(7, connectionID, ".flush", false); !handled || !reflect.DeepEqual(lines, []string{request}) {
		t.Fatalf("flush = %v, %q", handled, lines)
	}
	if manager.lookups != 0 {
		t.Fatalf("producer-ID operations performed %d manager lookups", manager.lookups)
	}
}

func TestReadUsesAdmittedPhysicalIDWithoutSecondLookup(t *testing.T) {
	const connectionID int64 = 91
	manager := &optionProbeConnManager{stubConnManager: stubConnManager{conn: &stubConn{id: connectionID}}}
	ctx := ctxWithConnManager(manager)
	ctx.IsWizard = true
	args := []types.Value{types.NewObj(7), types.NewInt(1)}
	if result := builtinRead(ctx, args); result.IsError() || result.Val.Int() != 0 {
		t.Fatalf("nonblocking read with defaults = %+v", result)
	}
	ctx.Session.setConnectionOption(connectionID, nil, "hold-input", types.NewInt(1))
	if result := builtinRead(ctx, args); !result.IsError() || result.Error != types.E_INVARG {
		t.Fatalf("read while producer holds input = %+v", result)
	}
	if manager.lookups != 2 {
		t.Fatalf("two reads performed %d lookups, want one admission lookup each", manager.lookups)
	}
}

func TestHTTPCancellationUsesReplacementBeforeNewInput(t *testing.T) {
	for _, replacementHeld := range []bool{false, true} {
		t.Run(map[bool]string{false: "replacement defaults", true: "replacement held"}[replacementHeld], func(t *testing.T) {
			const oldID, replacementID int64 = 91, 92
			manager := &optionProbeConnManager{stubConnManager: stubConnManager{conn: &stubConn{id: oldID}}}
			session := ctxWithConnManager(manager).Session
			session.setConnectionOption(oldID, nil, "hold-input", types.NewInt(1))
			removed, survivor := suspendedHTTPWaiter(9040), suspendedHTTPWaiter(9041)
			if _, complete := session.prepareHTTPRead(7, oldID, "request", removed.task); complete {
				t.Fatal("empty HTTP read completed")
			}
			state := session.runtime.heldHTTPInput.byPlayer[7]
			state.waiters = append(state.waiters, survivor)
			state.buffer, state.invalidCount = []byte("partial"), 1
			// Replacement changes the current mapping before any new input/read.
			manager.conn = &stubConn{id: replacementID}
			if replacementHeld {
				session.setConnectionOption(replacementID, nil, "hold-input", types.NewInt(1))
			}
			session.CancelHTTPReadTask(9999)
			if string(state.buffer) != "partial" || len(state.waiters) != 2 || state.invalidCount != 1 {
				t.Fatal("unrelated cancellation changed retained input")
			}
			session.CancelHTTPReadTask(removed.task.ID)
			if len(state.waiters) != 1 || state.waiters[0] != survivor || state.buffer != nil || state.invalidCount != 0 {
				t.Fatal("matched cancellation changed survivor or failed to clear input")
			}
			session.CancelHTTPReadTask(survivor.task.ID)
			if retained := session.runtime.heldHTTPInput.byPlayer[7] != nil; retained != replacementHeld {
				t.Fatalf("empty state retained=%v, replacement held=%v", retained, replacementHeld)
			}
			if manager.lookups != 3 {
				t.Fatalf("cancellation lookups=%d, want 3", manager.lookups)
			}
			session.CloseHeldHTTPInput(7)
			if session.runtime.heldHTTPInput.byPlayer[7] != nil {
				t.Fatal("teardown retained HTTP state")
			}
		})
	}
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
				wakes := r.collectHTTPWakeupsLocked(7, 7, state)
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
