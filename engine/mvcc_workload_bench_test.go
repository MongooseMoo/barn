package engine

// MVCC concurrency-redesign measurement harness (Phase 0 of
// plans/mvcc-concurrency-redesign-2026-07-21.md).
//
// This is the "truth instrument" the plan demands BEFORE any engine change: a
// mongoose-shaped workload of simulated players driven through the REAL VM /
// txn / commit / retry machinery, reporting ABSOLUTE 32-core goodput
// (committed commands/sec) with abort rate, p99 latency, and allocation
// pressure — never the serial/pool ratio (the June metric trap: per-task-work
// cuts move serial and pool equally and cannot move a ratio).
//
// Faithfulness to the phases it must measure:
//   - Phase 2 (alias immutable reads): `look` is a WIDE+DEEP read — it iterates
//     every occupant of the player's room reading name+desc, and desc falls
//     through inheritance to the shared root, so every read deep-clones today.
//   - Phase 3 (move off stop-the-world): `move` uses the REAL move() builtin,
//     which marks the live store mutated → coarse EXCLUSIVE commit today.
//   - Phase 4 (precise ancestry deps): occupant `o.desc` is an inherited-property
//     read that stamps a whole-object scan dep on the shared root; the optional
//     `churn` shape writes a property on that same root, so it conflicts with
//     every in-flight look/say (the pathological false-conflict case).
//
// The concurrency model matches production's hot path: interactive commands run
// on per-connection goroutines calling runTask synchronously and truly in
// parallel (input_processor.go:669), NOT the batch scheduler. So the driver
// spawns one goroutine per active player, each calling s.runTask directly.
//
// Gated behind BARN_MVCC_BENCH so it never runs in the normal (short) suite.

import (
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/config"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// --- fixture ---------------------------------------------------------------

type mongooseFixture struct {
	store       *dbstore.Store
	wizardID    types.ObjID
	sysID       types.ObjID
	rootID      types.ObjID
	roomGenID   types.ObjID
	playerGenID types.ObjID
	rooms       []types.ObjID
	players     []types.ObjID
	poolLen     int
}

// buildMongooseFixture builds a small MOO core: a shared root generic that
// every object inherits `desc` from, a room generic carrying look/announce
// verbs, a player generic, then R rooms, P players (each placed in a Zipfian
// room), and objsPerRoom*R inert objects distributed Zipfian across the rooms
// so a `look` in a hot room iterates many occupants. Object ids are captured
// from CreateObject, never hardcoded.
func buildMongooseFixture(t *testing.T, rooms, players, objsPerRoom int) *mongooseFixture {
	t.Helper()
	store := dbstore.NewStore()

	wiz := dbstore.NewObjectBuilder(3)
	wiz.SetOwner(3)
	wiz.SetFlags(dbstore.FlagWizard | dbstore.FlagProgrammer | dbstore.FlagUser)
	if err := store.Add(wiz.Build()); err != nil {
		t.Fatalf("Add wizard failed: %v", err)
	}
	wizardID := types.ObjID(3)

	mk := func(parents []types.ObjID) types.ObjID {
		id, ec := store.DirectTxn().CreateObject(parents, wizardID, false)
		if ec != types.E_NONE {
			t.Fatalf("CreateObject(%v) failed: %v", parents, ec)
		}
		return id
	}
	sysID := mk(nil)
	rootID := mk(nil)
	roomGenID := mk([]types.ObjID{rootID})
	playerGenID := mk([]types.ObjID{rootID})

	defInt := func(id types.ObjID, name string, perms dbstore.PropertyPerms) {
		p := dbstore.NewProperty(types.NewInt(0), wizardID, perms, false, true)
		if ec := store.DirectTxn().DefineProperty(id, name, p); ec != types.E_NONE {
			t.Fatalf("DefineProperty(%s on #%d) failed: %v", name, id, ec)
		}
	}
	// desc: inherited read target (fall-through to root -> scan dep on root).
	defInt(rootID, "desc", dbstore.PropRead)
	// churn: the Phase-4 ancestor-write target (bumps root propertyVersion).
	defInt(rootID, "churn", dbstore.PropRead|dbstore.PropWrite)
	// tick: an alternative ancestor-write target on the room generic.
	defInt(roomGenID, "tick", dbstore.PropRead|dbstore.PropWrite)
	// last_activity: per-player timestamp write target.
	defInt(playerGenID, "last_activity", dbstore.PropRead|dbstore.PropWrite)

	addVerb := func(id types.ObjID, name string, code []string) {
		v := dbstore.NewVerb(name, []string{name}, wizardID,
			dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug,
			dbstore.VerbArgs{This: "this", Prep: "none", That: "none"}, code)
		if _, ec := store.AddVerb(id, v); ec != types.E_NONE {
			t.Fatalf("AddVerb(%s on #%d) failed: %v", name, id, ec)
		}
	}
	// look: wide+deep read. Iterate this.contents; read each occupant's builtin
	// name and inherited desc. Resolving `look` walks ancestry to roomGen.
	addVerb(roomGenID, "look", []string{
		"c = this.contents;",
		"n = 0;",
		"for o in (c)",
		"  nm = o.name;",
		"  d = o.desc;",
		"  n = n + 1;",
		"endfor",
		"return n;",
	})
	// announce: say-shaped read (contents + names, no write, no desc).
	addVerb(roomGenID, "announce", []string{
		"c = this.contents;",
		"n = 0;",
		"for o in (c)",
		"  nm = o.name;",
		"  n = n + 1;",
		"endfor",
		"return n;",
	})

	// Rooms.
	roomIDs := make([]types.ObjID, rooms)
	for i := range roomIDs {
		roomIDs[i] = mk([]types.ObjID{roomGenID})
	}

	// Zipfian destination pool: hot rooms appear many times. reps ~ R/(rank+1).
	var pool []types.Value
	for k := 0; k < rooms; k++ {
		reps := rooms / (k + 1)
		if reps < 1 {
			reps = 1
		}
		for r := 0; r < reps; r++ {
			pool = append(pool, types.NewObj(roomIDs[k]))
		}
	}
	poolProp := dbstore.NewProperty(types.NewList(pool), wizardID, dbstore.PropRead, false, true)
	if ec := store.DirectTxn().DefineProperty(sysID, "destpool", poolProp); ec != types.E_NONE {
		t.Fatalf("DefineProperty(destpool) failed: %v", ec)
	}

	// Deterministic setup RNG so fixtures are reproducible across runs.
	setupRNG := rand.New(rand.NewSource(0x5eed))

	// Players placed Zipfian.
	playerIDs := make([]types.ObjID, players)
	for i := range playerIDs {
		pid := mk([]types.ObjID{playerGenID})
		dest := pool[setupRNG.Intn(len(pool))].ID()
		if ec := store.DirectTxn().MoveObject(pid, dest, 0); ec != types.E_NONE {
			t.Fatalf("MoveObject(player) failed: %v", ec)
		}
		playerIDs[i] = pid
	}

	// Inert objects placed Zipfian across rooms.
	total := objsPerRoom * rooms
	for i := 0; i < total; i++ {
		oid := mk([]types.ObjID{rootID})
		dest := pool[setupRNG.Intn(len(pool))].ID()
		if ec := store.DirectTxn().MoveObject(oid, dest, 0); ec != types.E_NONE {
			t.Fatalf("MoveObject(obj) failed: %v", ec)
		}
	}

	return &mongooseFixture{
		store: store, wizardID: wizardID, sysID: sysID, rootID: rootID,
		roomGenID: roomGenID, playerGenID: playerGenID,
		rooms: roomIDs, players: playerIDs, poolLen: len(pool),
	}
}

// --- workload driver -------------------------------------------------------

// shape indices.
const (
	shLook = iota
	shSay
	shMove
	shStamp
	shBuild
	shChurn
	shapeCount
)

// workloadMix is the per-command probability weights (need not sum to 100).
type workloadMix struct {
	look, say, move, stamp, build, churn int
}

func (m workloadMix) weights() [shapeCount]int {
	return [shapeCount]int{m.look, m.say, m.move, m.stamp, m.build, m.churn}
}

// realisticMix: look 40 / say 30 / move 20 / stamp 7 / build 3, no ancestor churn.
var realisticMix = workloadMix{look: 40, say: 30, move: 20, stamp: 7, build: 3}

// churnStressMix adds ancestor-property writes — the Phase-4 pathological case.
var churnStressMix = workloadMix{look: 40, say: 30, move: 15, stamp: 5, churn: 10}

type workloadResult struct {
	players, rooms int
	committed      int64
	errored        int64
	dur            time.Duration
	goodput        float64
	delta          commitCounterDelta
	p50, p99, max  time.Duration
	allocsPerOp    float64
	bytesPerOp     float64
	numGC          uint32
	pauseTotalMs   float64
}

func (r workloadResult) abortRate() float64 {
	if r.delta.attempts == 0 {
		return 0
	}
	return float64(r.delta.retries) / float64(r.delta.attempts)
}

// compileShapes precompiles all six command programs for one player. The
// programs are immutable and reused across every task the player runs, so
// compilation (MOO parsing) is kept OUT of the timed window.
func compileShapes(t *testing.T, reg *builtins.Registry, fx *mongooseFixture, playerID types.ObjID) [shapeCount]*bytecode.Program {
	t.Helper()
	src := [shapeCount]string{
		shLook:  fmt.Sprintf("loc = #%d.location; loc:look(); return 1;", playerID),
		shSay:   fmt.Sprintf("loc = #%d.location; loc:announce(); return 1;", playerID),
		shMove:  fmt.Sprintf("dest = #%d.destpool[random(%d)]; move(#%d, dest); return 1;", fx.sysID, fx.poolLen, playerID),
		shStamp: fmt.Sprintf("#%d.last_activity = #%d.last_activity + 1; return 1;", playerID, playerID),
		shBuild: fmt.Sprintf("o = create(#%d); recycle(o); return 1;", fx.rootID),
		shChurn: fmt.Sprintf("#%d.churn = #%d.churn + 1; return 1;", fx.rootID, fx.rootID),
	}
	var progs [shapeCount]*bytecode.Program
	for i, code := range src {
		progs[i] = compileTestProgram(t, reg, code)
	}
	return progs
}

// buildLiveTask constructs a fresh, ready-to-run task around a precompiled
// program — the cheap allocation-only path (no MOO parsing), mirroring
// buildBenchTask minus the *testing.T.
func buildLiveTask(s *Runtime, id int64, wizardID types.ObjID, prog *bytecode.Program) *task.Task {
	tk := task.NewTaskFull(id, wizardID, prog, 1<<50, 1e9)
	s.populateTaskContextDependencies(tk.Context)
	tk.Context.IsWizard = true
	tk.Programmer = wizardID
	tk.Context.Programmer = wizardID
	tk.StartTime = time.Now().Add(-time.Second)
	tk.Done = make(chan struct{})
	tk.ForkCreator = s
	return tk
}

// pickShape does weighted selection over the mix.
func pickShape(rng *rand.Rand, w [shapeCount]int, total int) int {
	x := rng.Intn(total)
	for i := 0; i < shapeCount; i++ {
		if x < w[i] {
			return i
		}
		x -= w[i]
	}
	return shLook
}

// runMongooseWorkload drives `active` players concurrently for a warm-up window
// (discarded) then a timed measurement window, returning the full result.
func runMongooseWorkload(t *testing.T, fx *mongooseFixture, active int, mix workloadMix, warmup, measure time.Duration) workloadResult {
	t.Helper()
	if active > len(fx.players) {
		active = len(fx.players)
	}
	s := newRuntimeWithWorkerCount(fx.store, config.Options{}, runtime.GOMAXPROCS(0))
	defer s.Stop()

	weights := mix.weights()
	total := 0
	for _, w := range weights {
		total += w
	}
	if total == 0 {
		t.Fatalf("empty mix")
	}

	// Precompile per active player.
	progs := make([][shapeCount]*bytecode.Program, active)
	for i := 0; i < active; i++ {
		progs[i] = compileShapes(t, s.registry, fx, fx.players[i])
	}

	var taskCounter int64 = 1_000_000

	// One driver goroutine per active player. `phase` gates warm-up vs measure.
	// reservoir sampling bounds latency memory per goroutine.
	const maxSamples = 40000
	type gstat struct {
		committed int64
		errored   int64
		lats      []time.Duration
		seen      int64
	}
	stats := make([]gstat, active)

	runWindow := func(dur time.Duration, record bool) {
		var wg sync.WaitGroup
		start := time.Now()
		deadline := start.Add(dur)
		for i := 0; i < active; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				rng := rand.New(rand.NewSource(int64(0xC0FFEE + idx)))
				st := &stats[idx]
				for time.Now().Before(deadline) {
					sh := pickShape(rng, weights, total)
					id := atomic.AddInt64(&taskCounter, 1)
					tk := buildLiveTask(s, id, fx.wizardID, progs[idx][sh])
					t0 := time.Now()
					err := s.runTask(tk)
					lat := time.Since(t0)
					ok := err == nil && tk.Result.Flow == types.FlowReturn
					if !record {
						continue
					}
					if ok {
						st.committed++
						// reservoir sample the latency
						st.seen++
						if len(st.lats) < maxSamples {
							st.lats = append(st.lats, lat)
						} else if j := rng.Int63n(st.seen); j < maxSamples {
							st.lats[j] = lat
						}
					} else {
						st.errored++
					}
				}
			}(i)
		}
		wg.Wait()
	}

	// Warm up (not recorded).
	runWindow(warmup, false)

	// Measurement window.
	runtime.GC()
	var m0 runtime.MemStats
	runtime.ReadMemStats(&m0)
	before := sampleCommitCounters(fx.store)
	measStart := time.Now()
	runWindow(measure, true)
	elapsed := time.Since(measStart)
	delta := sampleCommitCounters(fx.store).sub(before)
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	// Aggregate.
	var committed, errored int64
	var allLats []time.Duration
	for i := range stats {
		committed += stats[i].committed
		errored += stats[i].errored
		allLats = append(allLats, stats[i].lats...)
	}
	sort.Slice(allLats, func(a, b int) bool { return allLats[a] < allLats[b] })
	pick := func(q float64) time.Duration {
		if len(allLats) == 0 {
			return 0
		}
		idx := int(q * float64(len(allLats)))
		if idx >= len(allLats) {
			idx = len(allLats) - 1
		}
		return allLats[idx]
	}

	res := workloadResult{
		players: active, rooms: len(fx.rooms),
		committed: committed, errored: errored, dur: elapsed,
		goodput: float64(committed) / elapsed.Seconds(),
		delta:   delta,
		p50:     pick(0.50), p99: pick(0.99),
		numGC:        m1.NumGC - m0.NumGC,
		pauseTotalMs: float64(m1.PauseTotalNs-m0.PauseTotalNs) / 1e6,
	}
	if len(allLats) > 0 {
		res.max = allLats[len(allLats)-1]
	}
	if committed > 0 {
		res.allocsPerOp = float64(m1.Mallocs-m0.Mallocs) / float64(committed)
		res.bytesPerOp = float64(m1.TotalAlloc-m0.TotalAlloc) / float64(committed)
	}
	return res
}

// median goodput/abort over repeats (odd count -> middle element).
func medianResult(rs []workloadResult) workloadResult {
	sort.Slice(rs, func(a, b int) bool { return rs[a].goodput < rs[b].goodput })
	return rs[len(rs)/2]
}

// --- baseline sweep --------------------------------------------------------

// TestMVCCBaselineCurve records the Phase-0 master baseline: goodput / abort /
// p99 / allocs across a players x rooms grid, for the realistic mix and the
// Phase-4 churn-stress mix. Gated behind BARN_MVCC_BENCH=1.
func TestMVCCBaselineCurve(t *testing.T) {
	if os.Getenv("BARN_MVCC_BENCH") == "" {
		t.Skip("set BARN_MVCC_BENCH=1 to run the MVCC baseline curve")
	}
	warmup := envDuration("BARN_MVCC_WARMUP", 500*time.Millisecond)
	measure := envDuration("BARN_MVCC_MEASURE", 2*time.Second)
	repeats := envInt("BARN_MVCC_REPEATS", 5)

	playersList := envIntList("BARN_MVCC_PLAYERS", []int{1, 4, 16, 32})
	roomsList := envIntList("BARN_MVCC_ROOMS", []int{4, 16, 64})
	const objsPerRoom = 6

	t.Logf("GOMAXPROCS=%d warmup=%s measure=%s repeats=%d objsPerRoom=%d",
		runtime.GOMAXPROCS(0), warmup, measure, repeats, objsPerRoom)

	scenarios := []struct {
		name string
		mix  workloadMix
	}{
		{"realistic", realisticMix},
		{"churn-stress", churnStressMix},
	}

	for _, sc := range scenarios {
		t.Logf("=== scenario %s (mix look/say/move/stamp/build/churn = %d/%d/%d/%d/%d/%d) ===",
			sc.name, sc.mix.look, sc.mix.say, sc.mix.move, sc.mix.stamp, sc.mix.build, sc.mix.churn)
		t.Logf("%-8s %-7s %12s %8s %9s %8s %9s %10s %9s %6s",
			"players", "rooms", "goodput/s", "abort%", "p50", "p99", "max", "allocs/op", "bytes/op", "GCs")
		for _, rooms := range roomsList {
			for _, players := range playersList {
				fx := buildMongooseFixture(t, rooms, players, objsPerRoom)
				runs := make([]workloadResult, repeats)
				for r := 0; r < repeats; r++ {
					runs[r] = runMongooseWorkload(t, fx, players, sc.mix, warmup, measure)
				}
				m := medianResult(runs)
				t.Logf("%-8d %-7d %12.0f %7.2f%% %9s %9s %9s %10.1f %9.0f %6d",
					players, rooms, m.goodput, m.abortRate()*100,
					latStr(m.p50), latStr(m.p99), latStr(m.max),
					m.allocsPerOp, m.bytesPerOp, m.numGC)
			}
		}
	}
}

// latStr formats a latency with resolution appropriate to its magnitude, so
// sub-microsecond ops don't collapse to "0s" and millisecond tails stay legible.
func latStr(d time.Duration) string {
	switch {
	case d < 100*time.Microsecond:
		return fmt.Sprintf("%.2fus", float64(d.Nanoseconds())/1000)
	case d < time.Millisecond:
		return fmt.Sprintf("%.1fus", float64(d.Nanoseconds())/1000)
	default:
		return fmt.Sprintf("%.2fms", float64(d.Microseconds())/1000)
	}
}

// --- small env helpers -----------------------------------------------------

func envDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 {
			return n
		}
	}
	return def
}

func envIntList(key string, def []int) []int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	var out []int
	for _, part := range splitComma(v) {
		var n int
		if _, err := fmt.Sscanf(part, "%d", &n); err == nil && n > 0 {
			out = append(out, n)
		}
	}
	if len(out) == 0 {
		return def
	}
	return out
}

func splitComma(s string) []string {
	var out []string
	cur := ""
	for _, c := range s {
		if c == ',' {
			out = append(out, cur)
			cur = ""
		} else {
			cur += string(c)
		}
	}
	out = append(out, cur)
	return out
}
