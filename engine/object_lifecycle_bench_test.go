package engine

import (
	"fmt"
	"sync"
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// Each sample drains every command before reading the existing gate and conflict
// counters. Setup and final relationship checks are outside the timed interval.
func BenchmarkRecycleScaling(b *testing.B) {
	for _, committed := range []bool{false, true} {
		for _, shared := range []bool{false, true} {
			for _, hook := range []bool{false, true} {
				for _, workers := range []int{1, 2, 4, 8} {
					b.Run(fmt.Sprintf("committed=%t/shared=%t/hook=%t/workers=%d", committed, shared, hook, workers), func(b *testing.B) {
						const batch = 16
						var commands, holds, holdNS, sharedWaitNS, retries, wastedTicks int64
						b.ReportAllocs()
						for b.Loop() {
							b.StopTimer()
							store := protectedRedirectStore(b)
							addVerb := func(id types.ObjID, name, body string) {
								if _, ec := store.AddVerb(id, dbstore.NewVerb([]string{name}, 2,
									dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug,
									dbstore.VerbArgs{This: "this", Prep: "none", That: "none"}, []string{body})); ec != types.E_NONE {
									b.Fatal(ec)
								}
							}
							if hook {
								addVerb(1, "recycle", "return 0;")
							}
							body := "o = create(#1); move(o, args[1]); "
							if committed {
								body = "o = args[2]; "
							}
							addVerb(0, "measure_recycle", body+"recycle(o); n = create(#1); recycle(n); return !valid(o) && !valid(n);")
							rooms := make([]types.ObjID, workers)
							objects := make([][batch]types.ObjID, workers)
							for w := range workers {
								if shared && w > 0 {
									rooms[w] = rooms[0]
									continue
								}
								var ec types.ErrorCode
								rooms[w], ec = store.DirectTxn().CreateObject(nil, 2)
								if ec != types.E_NONE {
									b.Fatal(ec)
								}
							}
							if committed {
								for w := range workers {
									for i := range batch {
										id, ec := store.DirectTxn().CreateObject([]types.ObjID{1}, 2)
										if ec != types.E_NONE {
											b.Fatal(ec)
										}
										objects[w][i] = id
										if ec := store.DirectTxn().MoveObject(id, rooms[w], 0); ec != types.E_NONE {
											b.Fatal(ec)
										}
									}
								}
							}
							rt := NewRuntime(store)
							rt.SetDiagnosticsTracking(true)
							beforeShared := rt.SharedGate()
							start := make(chan struct{})
							errors := make(chan error, workers)
							var done sync.WaitGroup
							for w := range workers {
								done.Go(func() {
									<-start
									for i := range batch {
										result, err := rt.RunServerVerbTask(0, "measure_recycle", []types.Value{types.NewObj(rooms[w]), types.NewObj(objects[w][i])}, 2)
										if err != nil || result.Flow != types.FlowReturn || !result.Val.Truthy() {
											errors <- fmt.Errorf("worker %d command %d: result=%+v err=%v", w, i, result, err)
											return
										}
									}
								})
							}
							b.StartTimer()
							close(start)
							done.Wait()
							b.StopTimer()
							close(errors)
							for err := range errors {
								rt.Stop()
								b.Fatal(err)
							}
							commands += int64(workers * batch)
							for _, h := range rt.GateHolds() {
								holds += h.Count
								holdNS += int64(h.Total)
							}
							sharedWaitNS += rt.SharedGate().Sub(beforeShared).WaitNS
							retries += int64(store.CommitRetries())
							wastedTicks += store.ConflictCensus().WastedTicks
							for _, room := range rooms {
								contents, ec := store.DirectTxn().Contents(room)
								if ec != types.E_NONE || len(contents) != 0 {
									rt.Stop()
									b.Fatalf("room #%d contents=%v error=%v", room, contents, ec)
								}
							}
							children, ec := store.DirectTxn().Children(1)
							if ec != types.E_NONE || len(children) != 0 {
								rt.Stop()
								b.Fatalf("prototype children=%v error=%v", children, ec)
							}
							rt.Stop()
							b.StartTimer()
						}
						b.ReportMetric(float64(commands)/b.Elapsed().Seconds(), "commands/s")
						b.ReportMetric(float64(holds)/float64(commands), "gate-holds/cmd")
						b.ReportMetric(float64(holdNS)/float64(commands), "gate-ns/cmd")
						b.ReportMetric(float64(sharedWaitNS)/float64(commands), "shared-wait-ns/cmd")
						b.ReportMetric(float64(retries)/float64(commands), "retries/cmd")
						b.ReportMetric(float64(wastedTicks)/float64(commands), "wasted-ticks/cmd")
					})
				}
			}
		}
	}
}

// RunServerVerbTask exercises production transactions and VM lifecycle
// continuations. Fresh runtimes bound retained identities in every iteration.
func BenchmarkObjectLifecycleVM(b *testing.B) {
	for _, anonymous := range []int{0, 1} {
		for _, slots := range []int{0, 150} {
			for _, operation := range []string{"Create", "Recycle", "Pair", "Collect"} {
				if operation == "Collect" && anonymous == 0 {
					continue
				}
				for _, batch := range []int{1, 32} {
					b.Run(fmt.Sprintf("anon=%d/slots=%d/%s/batch=%d", anonymous, slots, operation, batch), func(b *testing.B) {
						b.ReportAllocs()
						for b.Loop() {
							b.StopTimer()
							store := protectedRedirectStore(b)
							for i := range slots {
								if ec := store.DirectTxn().DefineProperty(1, fmt.Sprintf("p%03d", i), dbstore.NewProperty(types.NewInt(int64(i)), 2, dbstore.PropRead, false, true)); ec != types.E_NONE {
									b.Fatal(ec)
								}
							}
							for _, name := range []string{"held", "recycled"} {
								if ec := store.DirectTxn().DefineProperty(0, name, dbstore.NewProperty(types.NewInt(0), 2, dbstore.PropRead|dbstore.PropWrite, false, true)); ec != types.E_NONE {
									b.Fatal(ec)
								}
							}
							if _, ec := store.AddVerb(1, dbstore.NewVerb([]string{"recycle"}, 2, dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug, dbstore.VerbArgs{This: "this", Prep: "none", That: "none"}, []string{"#0.recycled = #0.recycled + 1;"})); ec != types.E_NONE {
								b.Fatal(ec)
							}
							create := fmt.Sprintf("v = {}; for i in [1..%d] v = {@v, create(#1, %d)}; endfor #0.held = v; return length(v);", batch, anonymous)
							code := create
							switch operation {
							case "Recycle":
								code = "n = 0; for o in (#0.held) recycle(o); n = n + !valid(o); endfor #0.held = 0; return n;"
							case "Pair":
								code = fmt.Sprintf("n = 0; for i in [1..%d] o = create(#1, %d); recycle(o); n = n + !valid(o); endfor return n;", batch, anonymous)
							case "Collect":
								code = "#0.held = 0; run_gc(); return #0.recycled;"
							}
							for name, body := range map[string]string{"setup": create, "measure": code} {
								if _, ec := store.AddVerb(0, dbstore.NewVerb([]string{name}, 2, dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug, dbstore.VerbArgs{This: "this", Prep: "none", That: "none"}, []string{body})); ec != types.E_NONE {
									b.Fatal(ec)
								}
							}
							store.ShareLoadedPropertySlots(nil)
							rt := NewRuntime(store)
							run := func(name string) {
								result, err := rt.RunServerVerbTask(0, name, nil, 2)
								if err != nil || result.Flow != types.FlowReturn || result.Val.Type() != types.TYPE_INT || result.Val.Int() != int64(batch) {
									b.Fatalf("%s: result=%+v err=%v, want %d", name, result, err, batch)
								}
							}
							if operation == "Recycle" || operation == "Collect" {
								run("setup")
							}
							b.StartTimer()
							run("measure")
							b.StopTimer()
							if operation == "Recycle" || operation == "Pair" || operation == "Collect" {
								prop, ec := store.DirectTxn().FindProperty(0, "recycled")
								if ec != types.E_NONE || prop.Value.Int() != int64(batch) {
									b.Fatalf("recycle hooks: %+v %v, want %d", prop, ec, batch)
								}
							}
							rt.Stop()
							b.StartTimer()
						}
						b.ReportMetric(float64(batch), "objects/op")
					})
				}
			}
		}
	}
}
