// Package mooprof collects a sampled profile whose stacks are MOO verbs rather
// than Go functions, and encodes it in pprof's format so `go tool pprof` and
// its flame graphs work on MOO code unchanged.
//
// The VM takes samples at its existing 1,024-tick checkpoint and when an
// execution loop returns, and only while a Session is active. With no session
// the cost is one atomic load at each of those points.
package mooprof

import (
	"errors"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/pprof/profile"
)

// SampleTicks is the VM's checkpoint interval, recorded as the profile period.
const SampleTicks = 1024

// LabelVerb is the sample label naming a task's entry verb. It matches the Go
// CPU profile's pprof label so the two profiles can be lined up.
const LabelVerb = "moo.verb"

// ErrBusy is returned by Start while another session is collecting.
var ErrBusy = errors.New("mooprof: a profile session is already active")

// Frame is one MOO activation in a sample, innermost first.
type Frame struct {
	Func string // "#<verb location>:<verb name>"
	Line int64  // MOO source line
}

type sample struct {
	stack []Frame
	label string
	ticks int64
	wall  int64
}

// Session accumulates samples until Stop.
type Session struct {
	start time.Time

	mu      sync.Mutex
	stopped bool
	samples map[string]*sample
	order   []*sample
}

var active atomic.Pointer[Session]

// Active returns the collecting session, or nil.
func Active() *Session {
	return active.Load()
}

// Start begins a session. Only one session may collect at a time.
func Start() (*Session, error) {
	s := &Session{start: time.Now(), samples: make(map[string]*sample)}
	if !active.CompareAndSwap(nil, s) {
		return nil, ErrBusy
	}
	return s, nil
}

// Add records ticks and wall nanoseconds against stack. Samples that arrive
// after Stop are dropped.
func (s *Session) Add(stack []Frame, label string, ticks, wall int64) {
	if len(stack) == 0 || (ticks <= 0 && wall <= 0) {
		return
	}
	var key strings.Builder
	key.WriteString(label)
	for _, f := range stack {
		key.WriteByte(0)
		key.WriteString(f.Func)
		key.WriteByte(0)
		key.WriteString(strconv.FormatInt(f.Line, 10))
	}
	k := key.String()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return
	}
	agg := s.samples[k]
	if agg == nil {
		agg = &sample{stack: append([]Frame(nil), stack...), label: label}
		s.samples[k] = agg
		s.order = append(s.order, agg)
	}
	agg.ticks += ticks
	agg.wall += wall
}

// Stop ends the session and returns what it collected.
func (s *Session) Stop() *profile.Profile {
	active.CompareAndSwap(s, nil)
	s.mu.Lock()
	s.stopped = true
	order := s.order
	// VMs keep a pointer to the last session they sampled for; drop the
	// samples so an idle VM does not retain them.
	s.samples, s.order = nil, nil
	s.mu.Unlock()
	return s.build(order, time.Now())
}

func (s *Session) build(order []*sample, end time.Time) *profile.Profile {
	p := &profile.Profile{
		SampleType: []*profile.ValueType{
			{Type: "ticks", Unit: "count"},
			{Type: "wall", Unit: "nanoseconds"},
		},
		DefaultSampleType: "ticks",
		PeriodType:        &profile.ValueType{Type: "ticks", Unit: "count"},
		Period:            SampleTicks,
		TimeNanos:         s.start.UnixNano(),
		DurationNanos:     end.Sub(s.start).Nanoseconds(),
	}
	funcs := make(map[string]*profile.Function)
	type locKey struct {
		fn   string
		line int64
	}
	locs := make(map[locKey]*profile.Location)
	for _, agg := range order {
		ps := &profile.Sample{Value: []int64{agg.ticks, agg.wall}}
		if agg.label != "" {
			ps.Label = map[string][]string{LabelVerb: {agg.label}}
		}
		for _, f := range agg.stack {
			loc := locs[locKey{f.Func, f.Line}]
			if loc == nil {
				fn := funcs[f.Func]
				if fn == nil {
					fn = &profile.Function{ID: uint64(len(p.Function) + 1), Name: f.Func, SystemName: f.Func}
					funcs[f.Func] = fn
					p.Function = append(p.Function, fn)
				}
				loc = &profile.Location{
					ID:   uint64(len(p.Location) + 1),
					Line: []profile.Line{{Function: fn, Line: f.Line}},
				}
				locs[locKey{f.Func, f.Line}] = loc
				p.Location = append(p.Location, loc)
			}
			ps.Location = append(ps.Location, loc)
		}
		p.Sample = append(p.Sample, ps)
	}
	return p
}
