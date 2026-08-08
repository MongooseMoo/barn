package scheduler

// Real-database Mongoose workload harness.
//
// Unlike mvcc_workload_bench_test.go (synthetic fixture, precompiled eval
// programs), this loads the REAL mongoose database and drives REAL command
// lines through the production per-line path:
//
//   command.ParsePlayerCommand -> #0:do_command (full Mongoose dispatch)
//     -> fallback command.FindVerb -> ExecuteVerbTaskSync
//
// exactly as server/input_processor.go:processCommand does, minus the network
// connection plumbing. notify() is a successful no-op via a stub connection
// manager that reports the simulated players as connected (so @who-style
// verbs see a realistic player list) but has no live connections.
//
// Gated behind BARN_MONGOOSE_BENCH=1. Knobs:
//   BARN_MONGOOSE_DB          database path (default ../mongoose.db.new)
//   BARN_MONGOOSE_PLAYERS     comma list of concurrency levels (default 1,4,16)
//   BARN_MONGOOSE_WARMUP      warm-up window   (default 2s)
//   BARN_MONGOOSE_MEASURE     measure window   (default 8s)
//   BARN_MONGOOSE_PROMOTE     "0" disables PROMOTE_NUMBERS (default ON, as deployed)
//   BARN_MONGOOSE_CPUPROFILE  write CPU profile of the measure windows
//   BARN_MONGOOSE_MEMPROFILE  write heap profile after the run

import (
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"runtime/pprof"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/command"
	"github.com/MongooseMoo/barn/config"
	dbformat "github.com/MongooseMoo/barn/db/format"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// --- stub connection manager ------------------------------------------------

// benchConnection is a real-enough Connection for the simulated players:
// output sinks to nowhere, but idle/connected times behave like production so
// db code calling idle_seconds()/connected_seconds() (@who, room renders) gets
// answers instead of E_INVARG (network.go resolveConnection nil → E_INVARG).
type benchConnection struct {
	connectedAt time.Time
	lastActive  atomic.Int64 // unix seconds
}

func (c *benchConnection) Send(string) error         { return nil }
func (c *benchConnection) Buffer(string)             {}
func (c *benchConnection) Flush() error              { return nil }
func (c *benchConnection) RemoteAddr() string        { return "bench-harness" }
func (c *benchConnection) GetOutputPrefix() string   { return "" }
func (c *benchConnection) GetOutputSuffix() string   { return "" }
func (c *benchConnection) BufferedOutputLength() int { return 0 }
func (c *benchConnection) ConnectedSeconds() int64 {
	return int64(time.Since(c.connectedAt).Seconds())
}
func (c *benchConnection) IdleSeconds() int64 {
	idle := time.Now().Unix() - c.lastActive.Load()
	if idle < 0 {
		idle = 0
	}
	return idle
}
func (c *benchConnection) GetResolvedName() string { return "bench-harness" }
func (c *benchConnection) ListenerPort() (int64, bool) {
	return 7777, true
}

func (c *benchConnection) touch() { c.lastActive.Store(time.Now().Unix()) }

type benchConnManager struct {
	players []types.ObjID
	conns   map[types.ObjID]*benchConnection
}

func newBenchConnManager(players []types.ObjID) *benchConnManager {
	m := &benchConnManager{players: players, conns: make(map[types.ObjID]*benchConnection, len(players))}
	now := time.Now()
	for _, p := range players {
		c := &benchConnection{connectedAt: now}
		c.touch()
		m.conns[p] = c
	}
	return m
}

func (m *benchConnManager) GetConnection(p types.ObjID) builtins.Connection {
	if c, ok := m.conns[p]; ok {
		return c
	}
	return nil
}
func (m *benchConnManager) ConnectedPlayers(bool) []types.ObjID { return m.players }
func (m *benchConnManager) BootPlayer(types.ObjID) error        { return fmt.Errorf("bench: no boot") }
func (m *benchConnManager) RecyclePlayer(types.ObjID) error     { return nil }
func (m *benchConnManager) SwitchPlayer(oldPlayer, newPlayer types.ObjID) error {
	return nil
}
func (m *benchConnManager) GetListenPort() int { return 7777 }

// ListenerInfos reports #0 listening on $network.port (7777) so mongoose's
// $prod() — "#0 in slice(listeners($network.port), \"object\")" — sees the
// prod deployment shape and #0:server_started activates $sql_utils (which
// opens the SQLite handles `say` depends on).
func (m *benchConnManager) ListenerInfos() []builtins.ListenerInfo {
	return []builtins.ListenerInfo{{Object: 0, Port: 7777, PrintMessages: true}}
}

// AddListener accepts silently: server_started's prod branch calls
// listen(#0, $network.alternate_port) OUTSIDE any try — an error here would
// kill the whole boot verb before services start.
func (m *benchConnManager) AddListener(spec builtins.ListenerSpec) (builtins.ListenerDescriptor, error) {
	return builtins.ListenerDescriptor{Port: spec.Port}, nil
}
func (m *benchConnManager) RemoveListener(builtins.ListenerDescriptor) error { return nil }
func (m *benchConnManager) OpenNetworkConnection(string, int64) (types.ObjID, error) {
	return types.ObjNothing, fmt.Errorf("bench: no outbound")
}
func (m *benchConnManager) ConnectionNameLookup(types.ObjID, bool) (string, error) {
	return "bench-harness", nil
}

// --- command shapes ----------------------------------------------------------

type realShape struct {
	name   string
	line   string
	weight int
}

// Mix follows notes/moo-workload-characterization-2026-07-21.md: commands are
// dominated by wide reads (look/say/inventory/@who); "home" is the always-
// available real move() write path (teleport through the real verb tree).
var realShapes = defaultRealShapes()

// defaultRealShapes returns the command mix; BARN_MONGOOSE_ONLY=<name> narrows
// the mix to a single shape for differential diagnosis.
func defaultRealShapes() []realShape {
	all := []realShape{
		{"look", "look", 35},
		{"say", "say Hello there, this is a benchmark message!", 30},
		{"inventory", "i", 10},
		{"who", "@who", 10},
		{"home", "home", 15},
	}
	if only := os.Getenv("BARN_MONGOOSE_ONLY"); only != "" {
		for _, sh := range all {
			if sh.name == only {
				return []realShape{sh}
			}
		}
	}
	return all
}

// --- per-command execution (mirrors input_processor.processCommand) ----------

func runRealCommandLine(s *Scheduler, st *dbstore.Store, player types.ObjID, line string) (ok bool, failure string) {
	loc, ec := st.Location(player)
	if ec != types.E_NONE {
		return false, "location:" + ec.String()
	}
	cmd := command.ParsePlayerCommand(st, player, loc, line)
	if cmd == nil || cmd.Verb == "" {
		return false, "parse"
	}
	words := cmd.Words
	if len(words) == 0 {
		words = append([]string{cmd.Verb}, cmd.Args...)
	}
	args := make([]types.Value, len(words))
	for i, w := range words {
		args[i] = types.NewStr(w)
	}
	// Mirror server/input_login.go callDoCommand: a do_command exception is
	// logged and treated as "not handled" — the server falls through to the
	// native parser. (On this DB do_command currently raises E_INVARG under
	// Barn; see notes/mongoose-perf-hunt-2026-07-27.md, conformance lead.)
	res := s.CallVerbWithArgstr(0, "do_command", args, player, line)
	if res.Flow == types.FlowReturn && res.Val.Truthy() {
		return true, ""
	}
	match := command.FindVerb(st, player, loc, cmd)
	if match == nil {
		// Mirror production (input_processor.go:654): no match falls through to
		// the huh verb. Several mongoose player classes (e.g. parent #410) have
		// no `home` verb — that is legitimate dispatch, not a failure.
		if huh := command.FindHuhVerb(st, player, loc, false); huh != nil {
			if err := s.ExecuteVerbTaskSync(player, huh, cmd, ""); err != nil {
				return false, "huh-exec:" + err.Error()
			}
			return true, ""
		}
		return false, "no-verb-match"
	}
	if err := s.ExecuteVerbTaskSync(player, match, cmd, ""); err != nil {
		return false, "exec:" + err.Error()
	}
	return true, ""
}

// --- the benchmark -----------------------------------------------------------

func TestMongooseRealWorkload(t *testing.T) {
	if os.Getenv("BARN_MONGOOSE_BENCH") == "" {
		t.Skip("set BARN_MONGOOSE_BENCH=1 to run the real-database mongoose workload")
	}
	dbPath := os.Getenv("BARN_MONGOOSE_DB")
	if dbPath == "" {
		dbPath = "../mongoose.db.new"
	}
	warmup := envDuration("BARN_MONGOOSE_WARMUP", 2*time.Second)
	measure := envDuration("BARN_MONGOOSE_MEASURE", 8*time.Second)
	levels := envIntList("BARN_MONGOOSE_PLAYERS", []int{1, 4, 16})

	loadStart := time.Now()
	database, err := dbformat.LoadDatabase(dbPath)
	if err != nil {
		t.Fatalf("LoadDatabase(%s): %v", dbPath, err)
	}
	store := database.NewStoreFromDatabase()
	t.Logf("loaded %s in %s", dbPath, time.Since(loadStart))

	// Mirror server boot (server.go LoadServerOptions/LoadProtectedBuiltins):
	// mongoose protects valid/create/match/... and overrides them with
	// #0-reachable bf_* wrappers. Skipping this ran the raw builtins and
	// manufactured E_TYPE storms production Barn never produces.
	builtins.LoadServerOptionsFromStore(store)
	builtins.LoadProtectedBuiltinsFromStore(store)
	t.Cleanup(func() {
		builtins.LoadServerOptionsFromStore(nil)
		builtins.LoadProtectedBuiltinsFromStore(nil)
	})

	// Pick real player objects with a valid location.
	var candidates []types.ObjID
	for _, pid := range store.Players() {
		if loc, ec := store.Location(pid); ec == types.E_NONE && loc > 0 {
			candidates = append(candidates, pid)
		}
	}
	sort.Slice(candidates, func(a, b int) bool { return candidates[a] < candidates[b] })
	t.Logf("players with valid location: %d", len(candidates))
	maxLevel := 0
	for _, l := range levels {
		if l > maxLevel {
			maxLevel = l
		}
	}
	if len(candidates) < maxLevel {
		t.Fatalf("only %d candidate players, need %d", len(candidates), maxLevel)
	}

	if cp := os.Getenv("BARN_MONGOOSE_CPUPROFILE"); cp != "" {
		f, err := os.Create(cp)
		if err != nil {
			t.Fatalf("create cpuprofile: %v", err)
		}
		defer f.Close()
		// Profile covers measurement windows only (started after warm-up below
		// would need restarts per level; instead start now and note that DB
		// load is excluded and warm-up windows are included but identifiable).
		if err := pprof.StartCPUProfile(f); err != nil {
			t.Fatalf("start cpuprofile: %v", err)
		}
		defer pprof.StopCPUProfile()
	}

	// The Mongoose deployment runs `barn --promote-numbers` (the mongoose
	// toaststunt fork's PROMOTE_NUMBERS mode; see plans/barn-toast-mongoose-
	// convergence-workstreams.md line 42 and notes-mongoose-promote-and-login.md).
	// Strict arithmetic manufactures per-look E_TYPE storms this database's code
	// never sees in production. Default ON; BARN_MONGOOSE_PROMOTE=0 for strict.
	opts := config.Options{PromoteNumbers: os.Getenv("BARN_MONGOOSE_PROMOTE") != "0"}
	s := newSchedulerWithWorkerCount(store, opts, runtime.GOMAXPROCS(0))

	// Mirror server boot's #0:server_started (server.go callServerStarted) —
	// it activates services including $sql_utils, which opens the SQLite
	// handles ($sound_handler etc.). Without it, every `say` raises
	// "This database is not open" through #3882::execute, an error the
	// production server only sees in the restart window. The boot conn
	// manager must be installed FIRST: $prod() checks listeners().
	// SQLite/fileio confine paths to a CWD-relative files/ sandbox, so run
	// from the repo root like the production server does.
	if err := os.Chdir(".."); err != nil {
		t.Fatalf("chdir to repo root: %v", err)
	}
	s.Registry().SetConnectionManager(newBenchConnManager(nil))
	if store.HasLocalVerb(0, "server_started") {
		if _, err := s.RunServerVerbTask(0, "server_started", nil, 0); err != nil {
			t.Logf("#0:server_started failed: %v", err)
		}
		// Services start via fork(0); give them a moment to open their dbs.
		time.Sleep(2 * time.Second)
	}
	// Known genuine errors from this snapshot, reproduced faithfully (see
	// experiments/2026-07-27-mongoose-contention-census.md): `say` raises
	// "This database is not open" (#2585.sql is a waif orphaned from the
	// $sql_utils registry IN THE DUMP — waif indices 4035 vs 6277-6279), and
	// #410-cohort @who raises E_PROPNF (wizard #36 lacks .cloaked; Toast
	// raises identically). Both feed #0:handle_uncaught_error, exactly as on
	// a production server booted from this snapshot.
	defer s.Stop()

	totalWeight := 0
	for _, sh := range realShapes {
		totalWeight += sh.weight
	}

	t.Logf("GOMAXPROCS=%d warmup=%s measure=%s mix=%v",
		runtime.GOMAXPROCS(0), warmup, measure, realShapes)

	for _, active := range levels {
		players := candidates[:active]
		cm := newBenchConnManager(players)
		s.Registry().SetConnectionManager(cm)

		type shapeStat struct {
			ok, fail          int64
			totalLat          time.Duration
			attempts, retries uint64
		}
		type gstat struct {
			shapes   []shapeStat
			lats     []time.Duration
			seen     int64
			failMsgs []string
		}
		stats := make([]gstat, active)
		for i := range stats {
			stats[i].shapes = make([]shapeStat, len(realShapes))
		}
		const maxSamples = 20000

		runWindow := func(dur time.Duration, record bool) {
			var wg sync.WaitGroup
			deadline := time.Now().Add(dur)
			for i := 0; i < active; i++ {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()
					rng := rand.New(rand.NewSource(int64(0xBEEF + idx)))
					st := &stats[idx]
					for time.Now().Before(deadline) {
						x := rng.Intn(totalWeight)
						sh := 0
						for j, cand := range realShapes {
							if x < cand.weight {
								sh = j
								break
							}
							x -= cand.weight
						}
						// Per-command counter deltas are only attributable when a
						// single player runs (sequential commands); with more
						// players the deltas interleave and are recorded anyway
						// as an aggregate approximation.
						c0 := sampleCommitCounters(store)
						t0 := time.Now()
						if conn := cm.conns[players[idx]]; conn != nil {
							conn.touch() // typing a command resets idle, as in production
						}
						ok, failure := runRealCommandLine(s, store, players[idx], realShapes[sh].line)
						lat := time.Since(t0)
						cd := sampleCommitCounters(store).sub(c0)
						if !record {
							continue
						}
						ss := &st.shapes[sh]
						ss.attempts += cd.attempts
						ss.retries += cd.retries
						if ok {
							ss.ok++
							ss.totalLat += lat
							st.seen++
							if len(st.lats) < maxSamples {
								st.lats = append(st.lats, lat)
							} else if j := rng.Int63n(st.seen); j < maxSamples {
								st.lats[j] = lat
							}
						} else {
							ss.fail++
							if len(st.failMsgs) < 5 {
								st.failMsgs = append(st.failMsgs,
									realShapes[sh].name+" -> "+failure)
							}
						}
					}
				}(i)
			}
			wg.Wait()
		}

		runWindow(warmup, false)

		runtime.GC()
		var m0 runtime.MemStats
		runtime.ReadMemStats(&m0)
		before := sampleCommitCounters(store)
		measStart := time.Now()
		runWindow(measure, true)
		elapsed := time.Since(measStart)
		delta := sampleCommitCounters(store).sub(before)
		var m1 runtime.MemStats
		runtime.ReadMemStats(&m1)

		var committed, failed int64
		var allLats []time.Duration
		shapeAgg := make([]shapeStat, len(realShapes))
		var failSamples []string
		for i := range stats {
			for j := range stats[i].shapes {
				shapeAgg[j].ok += stats[i].shapes[j].ok
				shapeAgg[j].fail += stats[i].shapes[j].fail
				shapeAgg[j].totalLat += stats[i].shapes[j].totalLat
				shapeAgg[j].attempts += stats[i].shapes[j].attempts
				shapeAgg[j].retries += stats[i].shapes[j].retries
				committed += stats[i].shapes[j].ok
				failed += stats[i].shapes[j].fail
			}
			allLats = append(allLats, stats[i].lats...)
			failSamples = append(failSamples, stats[i].failMsgs...)
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

		goodput := float64(committed) / elapsed.Seconds()
		abortRate := 0.0
		if delta.attempts > 0 {
			abortRate = float64(delta.retries) / float64(delta.attempts) * 100
		}
		allocsPerOp, bytesPerOp := 0.0, 0.0
		if committed > 0 {
			allocsPerOp = float64(m1.Mallocs-m0.Mallocs) / float64(committed)
			bytesPerOp = float64(m1.TotalAlloc-m0.TotalAlloc) / float64(committed)
		}
		t.Logf("players=%d goodput=%.0f/s failed=%d abort=%.2f%% p50=%s p99=%s max=%s allocs/op=%.0f bytes/op=%.0f GCs=%d",
			active, goodput, failed, abortRate,
			latStr(pick(0.50)), latStr(pick(0.99)), latStr(pick(0.999)),
			allocsPerOp, bytesPerOp, m1.NumGC-m0.NumGC)
		for j, sh := range realShapes {
			agg := shapeAgg[j]
			avg := time.Duration(0)
			if agg.ok > 0 {
				avg = agg.totalLat / time.Duration(agg.ok)
			}
			t.Logf("  shape %-10s ok=%-8d fail=%-6d avg=%s attempts=%d retries=%d",
				sh.name, agg.ok, agg.fail, latStr(avg), agg.attempts, agg.retries)
		}
		// Idle probe: with NO commands running, do background tasks (forks
		// spawned by earlier commands) keep committing? Distinguishes a
		// persistent self-rescheduling background writer from per-command
		// fork churn.
		idle0 := sampleCommitCounters(store)
		time.Sleep(2 * time.Second)
		idleDelta := sampleCommitCounters(store).sub(idle0)
		t.Logf("  idle 2s: attempts=%d successes=%d retries=%d",
			idleDelta.attempts, idleDelta.successes, idleDelta.retries)

		if len(failSamples) > 0 {
			uniq := map[string]int{}
			for _, m := range failSamples {
				uniq[m]++
			}
			var keys []string
			for k := range uniq {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				t.Logf("  failure sample (x%d): %s", uniq[k], k)
			}
		}
	}

	if mp := os.Getenv("BARN_MONGOOSE_MEMPROFILE"); mp != "" {
		f, err := os.Create(mp)
		if err != nil {
			t.Fatalf("create memprofile: %v", err)
		}
		runtime.GC()
		if err := pprof.WriteHeapProfile(f); err != nil {
			t.Fatalf("write memprofile: %v", err)
		}
		f.Close()
	}
	_ = strings.TrimSpace("")
}
