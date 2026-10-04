package builtins

import (
	"bytes"
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func httpBufferFixture(kind string, count int, body string) []byte {
	var input strings.Builder
	for i := 0; i < count; i++ {
		if kind == "request" {
			fmt.Fprintf(&input, "GET /%d HTTP/1.1\r\n", i)
		} else {
			fmt.Fprint(&input, "HTTP/1.1 200 OK\r\n")
		}
		fmt.Fprintf(&input, "Content-Length: %d\r\n\r\n%s", len(body), body)
	}
	return []byte(input.String())
}

func httpBufferSession() *Session {
	session := NewSession(NewRegistry(), NoHost())
	session.setConnectionOption(7, "hold-input", types.NewInt(1))
	return session
}

func TestHTTPBufferedDrainsPreserveMessagesAndIncompleteTail(t *testing.T) {
	for _, kind := range []string{"request", "response"} {
		for _, queued := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/queued=%v", kind, queued), func(t *testing.T) {
				session := httpBufferSession()
				input := httpBufferFixture(kind, 5, "body")
				tail := []byte("unfinished")
				state := &httpHeldInput{buffer: append(input, tail...)}
				session.runtime.heldHTTPInput.byPlayer[7] = state
				var wants []types.Value
				remaining := input
				for range 5 {
					value, consumed, complete := parseHTTPMessage(kind, remaining)
					if !complete || consumed <= 0 {
						t.Fatal("fixture did not parse")
					}
					wants = append(wants, value)
					remaining = remaining[consumed:]
				}
				if queued {
					for i := 0; i < 6; i++ {
						waiter := suspendedHTTPWaiter(int64(5000 + i))
						waiter.kind = kind
						state.waiters = append(state.waiters, waiter)
					}
					last := state.waiters[5]
					first := append([]httpReadWaiter(nil), state.waiters[:5]...)
					session.runtime.heldHTTPInput.mu.Lock()
					wakes := session.collectHTTPWakeupsLocked(7, state)
					session.runtime.heldHTTPInput.mu.Unlock()
					if len(wakes) != 5 || len(state.waiters) != 1 || state.waiters[0] != last {
						t.Fatalf("wakes=%d remaining waiters=%d", len(wakes), len(state.waiters))
					}
					for i, wake := range wakes {
						if wake.task != first[i].task || !wake.value.Equal(wants[i]) {
							t.Fatalf("wake %d changed task order or parsed value", i)
						}
					}
				} else {
					for i, want := range wants {
						got, complete := session.prepareHTTPRead(7, kind, nil)
						if !complete || !got.Equal(want) {
							t.Fatalf("immediate read %d changed value", i)
						}
					}
					if _, complete := session.prepareHTTPRead(7, kind, nil); complete {
						t.Fatal("incomplete final message completed")
					}
				}
				if !bytes.Equal(state.buffer, tail) {
					t.Fatalf("remaining bytes=%q, want %q", state.buffer, tail)
				}
			})
		}
	}
}

func TestHTTPConsumedStorageReleasesEmptyAndLargePrefix(t *testing.T) {
	for _, queued := range []bool{false, true} {
		for _, partial := range []bool{false, true} {
			session := httpBufferSession()
			message := httpBufferFixture("request", 1, strings.Repeat("a", 128<<10))
			prefixLength := len(message)
			tail := []byte(nil)
			if partial {
				tail = []byte("GET /partial")
			}
			input := append(message, tail...)
			state := &httpHeldInput{buffer: input}
			session.runtime.heldHTTPInput.byPlayer[7] = state
			if queued {
				state.waiters = []httpReadWaiter{suspendedHTTPWaiter(5100)}
				session.runtime.heldHTTPInput.mu.Lock()
				wakes := session.collectHTTPWakeupsLocked(7, state)
				session.runtime.heldHTTPInput.mu.Unlock()
				if len(wakes) != 1 {
					t.Fatal("large queued message did not complete")
				}
			} else if _, complete := session.prepareHTTPRead(7, "request", nil); !complete {
				t.Fatal("large immediate message did not complete")
			}
			if !bytes.Equal(state.buffer, tail) {
				t.Fatal("large-prefix drain changed unread suffix")
			}
			if !partial && state.buffer != nil {
				t.Fatal("empty buffer retained storage")
			}
			if partial && (&state.buffer[0] == &input[prefixLength] || cap(state.buffer) > 64<<10) {
				t.Fatal("small unread suffix retained the large original allocation")
			}
		}
	}
}

func TestHTTPBufferedMalformedAndFrontInsertion(t *testing.T) {
	session := httpBufferSession()
	valid := httpBufferFixture("request", 1, "body")
	malformed := []byte("invalid\r\n")
	input := append(append([]byte(nil), malformed...), valid...)
	state := &httpHeldInput{buffer: input}
	session.runtime.heldHTTPInput.byPlayer[7] = state
	want, consumed, complete := parseHTTPMessage("request", input)
	if !complete || consumed != len(malformed) {
		t.Fatal("malformed fixture boundary changed")
	}
	got, complete := session.prepareHTTPRead(7, "request", nil)
	if !complete || !got.Equal(want) || !bytes.Equal(state.buffer, valid) {
		t.Fatal("malformed message changed value or consumed bytes")
	}
	front := []byte("GET /front HTTP/1.1\r\n\r\n")
	if handled, _ := session.HandleHeldInput(7, encodeBinaryStr(front), true); !handled {
		t.Fatal("front input was not intercepted")
	}
	for _, next := range [][]byte{front, valid} {
		want, _, _ := parseHTTPMessage("request", next)
		got, complete := session.prepareHTTPRead(7, "request", nil)
		if !complete || !got.Equal(want) {
			t.Fatal("front insertion changed message order or values")
		}
	}
	if state.buffer != nil {
		t.Fatal("front-insert drain retained an empty buffer")
	}
}

var httpBufferValueSink types.Value

func TestHTTPAppendAndCancellationAfterConsumption(t *testing.T) {
	session := httpBufferSession()
	first := httpBufferFixture("request", 1, "body")
	partial := []byte("GET /next HTTP/1.1\r\n")
	state := &httpHeldInput{buffer: append(first, partial...)}
	session.runtime.heldHTTPInput.byPlayer[7] = state
	if _, complete := session.prepareHTTPRead(7, "request", nil); !complete {
		t.Fatal("first message did not complete")
	}
	if handled, _ := session.HandleHeldInput(7, "Content-Length: 4~0D~0A~0D~0Abody", false); !handled {
		t.Fatal("suffix append was not intercepted")
	}
	got, complete := session.prepareHTTPRead(7, "request", nil)
	if !complete || mustStringAt(t, got, "uri") != "/next" || mustStringAt(t, got, "body") != "body" {
		t.Fatal("append after consumption changed the next message")
	}
	state.buffer = append(httpBufferFixture("request", 1, "body"), partial...)
	if _, complete := session.prepareHTTPRead(7, "request", nil); !complete {
		t.Fatal("second prefix did not complete")
	}
	waiter := suspendedHTTPWaiter(5300)
	state.waiters = []httpReadWaiter{waiter}
	session.CancelHTTPReadTask(waiter.task.ID)
	if state.buffer != nil || len(state.waiters) != 0 {
		t.Fatal("cancellation retained consumed input")
	}
	if handled, _ := session.HandleHeldInput(7, encodeBinaryStr(first), false); !handled {
		t.Fatal("fresh input was not intercepted")
	}
	want, _, _ := parseHTTPMessage("request", first)
	got, complete = session.prepareHTTPRead(7, "request", nil)
	if !complete || !got.Equal(want) || state.buffer != nil {
		t.Fatal("fresh parse after cancellation changed input")
	}
}

func TestHTTPDrainingAllocationScalesWithMessageCount(t *testing.T) {
	session := httpBufferSession()
	state := &httpHeldInput{}
	session.runtime.heldHTTPInput.byPlayer[7] = state
	measure := func(messages, repeats int) uint64 {
		fixture := httpBufferFixture("request", messages, "body")
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		for range repeats {
			state.buffer = fixture
			for range messages {
				var complete bool
				httpBufferValueSink, complete = session.prepareHTTPRead(7, "request", nil)
				if !complete {
					t.Fatal("allocation fixture did not drain")
				}
			}
		}
		runtime.ReadMemStats(&after)
		return (after.TotalAlloc - before.TotalAlloc) / uint64(repeats)
	}
	// Warm registry/options/parser paths before measuring and allow generous
	// linear growth for parser/result allocations and background runtime work.
	_ = measure(1, 2)
	small := measure(1, 64)
	large := measure(512, 4)
	if large > small*512*3 {
		t.Fatalf("512-message drain allocated %d bytes; one-message=%d, linear budget=%d", large, small, small*512*3)
	}
}

func TestHTTPLargeAppendAfterConsumptionReleasesNewAllocation(t *testing.T) {
	session := httpBufferSession()
	body := strings.Repeat("a", 128<<10)
	header := []byte(fmt.Sprintf("GET /next HTTP/1.1\r\nContent-Length: %d\r\n\r\n", len(body)))
	state := &httpHeldInput{buffer: append(httpBufferFixture("request", 1, "body"), header...)}
	session.runtime.heldHTTPInput.byPlayer[7] = state
	if _, complete := session.prepareHTTPRead(7, "request", nil); !complete {
		t.Fatal("first message did not complete")
	}
	tail := []byte("GET /unfinished")
	if handled, _ := session.HandleHeldInput(7, encodeBinaryStr(append([]byte(body), tail...)), false); !handled {
		t.Fatal("large suffix append was not intercepted")
	}
	allocation := state.buffer
	if _, complete := session.prepareHTTPRead(7, "request", nil); !complete {
		t.Fatal("appended large body did not complete")
	}
	if !bytes.Equal(state.buffer, tail) || &state.buffer[0] == &allocation[len(header)+len(body)] || cap(state.buffer) > 64<<10 {
		t.Fatal("small tail retained the new large allocation after buffer growth")
	}
}

func BenchmarkHTTPBufferDrain(b *testing.B) {
	for _, kind := range []string{"request", "response"} {
		for _, queued := range []bool{false, true} {
			for _, count := range []int{1, 16, 128, 0} {
				name := fmt.Sprintf("%s/queued=%v/messages=%d", kind, queued, count)
				b.Run(name, func(b *testing.B) {
					session := httpBufferSession()
					state := &httpHeldInput{}
					messages, body := count, "body"
					var tail []byte
					if count == 0 {
						messages, body = 1, strings.Repeat("a", 128<<10)
						tail = []byte("partial")
					}
					fixture := append(httpBufferFixture(kind, messages, body), tail...)
					waiters := make([]httpReadWaiter, messages)
					for i := range waiters {
						waiters[i] = suspendedHTTPWaiter(int64(5200 + i))
						waiters[i].kind = kind
					}
					backing := make([]httpReadWaiter, messages)
					session.runtime.heldHTTPInput.byPlayer[7] = state
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						*state = httpHeldInput{buffer: fixture}
						if queued {
							copy(backing, waiters)
							state.waiters = backing
							session.runtime.heldHTTPInput.mu.Lock()
							wakes := session.collectHTTPWakeupsLocked(7, state)
							session.runtime.heldHTTPInput.mu.Unlock()
							if len(wakes) != messages {
								b.Fatal("queued workload did not drain")
							}
							httpBufferValueSink = wakes[len(wakes)-1].value
						} else {
							for range messages {
								var complete bool
								httpBufferValueSink, complete = session.prepareHTTPRead(7, kind, nil)
								if !complete {
									b.Fatal("immediate workload did not drain")
								}
							}
						}
					}
					b.StopTimer()
					want, _, complete := parseHTTPMessage(kind, httpBufferFixture(kind, 1, body))
					if kind == "request" {
						want, _, complete = parseHTTPMessage(kind, []byte(fmt.Sprintf("GET /%d HTTP/1.1\r\nContent-Length: %d\r\n\r\n%s", messages-1, len(body), body)))
					}
					if !complete || !httpBufferValueSink.Equal(want) || !bytes.Equal(state.buffer, tail) {
						b.Fatal("benchmark output or unread boundary changed")
					}
				})
			}
		}
	}
}
