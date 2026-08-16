package store

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// benchChainStore builds a `depth`-deep single-inheritance chain with the target
// verb and property defined only on the root, so every resolution from the leaf
// walks the whole chain — the shape of real MOO dispatch, where $string_utils
// and friends sit several hops above the object being called.
func benchChainStore(b *testing.B, depth int, verbsPerObject int) *Store {
	b.Helper()
	s := NewStore()
	for id := types.ObjID(0); id < types.ObjID(depth); id++ {
		if err := s.Add(NewObject(id, 0)); err != nil {
			b.Fatalf("Add #%d: %v", id, err)
		}
		if id > 0 {
			if ec := s.ChangeParents(id, []types.ObjID{id - 1}); ec != types.E_NONE {
				b.Fatalf("ChangeParents #%d: %v", id, ec)
			}
		}
		for v := 0; v < verbsPerObject; v++ {
			name := "filler" + string(rune('a'+v)) + string(rune('0'+int(id)%10))
			verb := NewVerb(name, []string{name}, 0, VerbRead|VerbExecute,
				VerbArgs{This: "none", Prep: "none", That: "none"}, []string{"return 0;"})
			if _, ec := s.AddVerb(id, verb); ec != types.E_NONE {
				b.Fatalf("AddVerb #%d %q: %v", id, name, ec)
			}
		}
	}
	target := NewVerb("target", []string{"target"}, 0, VerbRead|VerbExecute,
		VerbArgs{This: "none", Prep: "none", That: "none"}, []string{"return 1;"})
	if _, ec := s.AddVerb(0, target); ec != types.E_NONE {
		b.Fatalf("AddVerb target: %v", ec)
	}
	if ec := s.DirectTxn().DefineProperty(0, "targetprop", NewProperty(types.NewInt(1), 0, PropRead, false, true)); ec != types.E_NONE {
		b.Fatalf("DefineProperty: %v", ec)
	}
	return s
}

func BenchmarkTxnFindVerbAncestry(b *testing.B) {
	const depth = 6
	s := benchChainStore(b, depth, 8)
	leaf := types.ObjID(depth - 1)
	tx := s.BeginReadOnly(0)
	defer tx.Release()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := tx.findVerb(leaf, "target", true); err != nil {
			b.Fatalf("findVerb: %v", err)
		}
	}
}

func BenchmarkTxnFindVerbMissing(b *testing.B) {
	const depth = 6
	s := benchChainStore(b, depth, 8)
	leaf := types.ObjID(depth - 1)
	tx := s.BeginReadOnly(0)
	defer tx.Release()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tx.findVerb(leaf, "absent", true)
	}
}

// The *NoMemo variants privatize an object first, which is what every writing
// task does and what permanently disables the resolution memo. They therefore
// measure the reusable-scratch change on its own, with no memoization at all —
// the pessimistic case, and directly comparable to the same benchmark run
// against master.
func BenchmarkTxnFindVerbAncestryNoMemo(b *testing.B) {
	const depth = 6
	s := benchChainStore(b, depth, 8)
	leaf := types.ObjID(depth - 1)
	tx := s.BeginReadOnly(0)
	defer tx.Release()
	tx.mutableObject(leaf)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := tx.findVerb(leaf, "target", true); err != nil {
			b.Fatalf("findVerb: %v", err)
		}
	}
}

func BenchmarkTxnFindPropertyAncestryNoMemo(b *testing.B) {
	const depth = 6
	s := benchChainStore(b, depth, 8)
	leaf := types.ObjID(depth - 1)
	tx := s.BeginReadOnly(0)
	defer tx.Release()
	tx.mutableObject(leaf)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, ec := tx.findProperty(leaf, "targetprop"); ec != types.E_NONE {
			b.Fatalf("findProperty: %v", ec)
		}
	}
}

func BenchmarkTxnFindPropertyAncestry(b *testing.B) {
	const depth = 6
	s := benchChainStore(b, depth, 8)
	leaf := types.ObjID(depth - 1)
	tx := s.BeginReadOnly(0)
	defer tx.Release()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, ec := tx.findProperty(leaf, "targetprop"); ec != types.E_NONE {
			b.Fatalf("findProperty: %v", ec)
		}
	}
}
