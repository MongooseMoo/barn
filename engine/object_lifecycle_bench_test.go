package engine

import (
	"fmt"
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

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
