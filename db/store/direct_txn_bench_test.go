package store

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// Direct-transaction reads and writes over the 6-deep chain the resolve
// benchmarks use: the paths a server task takes before it binds a snapshot
// (verb lookup, command matching, limits) and the coarse direct writes.

func BenchmarkDirectFindCallableVerb(b *testing.B) {
	s := benchChainStore(b, 6, 8)
	d := s.DirectTxn()
	b.ReportAllocs()
	for b.Loop() {
		if _, _, err := d.FindCallableVerb(5, "target"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDirectFindProperty(b *testing.B) {
	s := benchChainStore(b, 6, 8)
	d := s.DirectTxn()
	b.ReportAllocs()
	for b.Loop() {
		if _, ec := d.FindProperty(5, "targetprop"); ec != types.E_NONE {
			b.Fatal(ec)
		}
	}
}

func BenchmarkDirectContents(b *testing.B) {
	s := benchChainStore(b, 6, 8)
	d := s.DirectTxn()
	for id := types.ObjID(1); id < 6; id++ {
		if ec := d.MoveObject(id, 0, 0); ec != types.E_NONE {
			b.Fatal(ec)
		}
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, ec := d.Contents(0); ec != types.E_NONE {
			b.Fatal(ec)
		}
	}
}

func BenchmarkDirectSetPropertyValue(b *testing.B) {
	s := benchChainStore(b, 6, 8)
	d := s.DirectTxn()
	b.ReportAllocs()
	i := int64(0)
	for b.Loop() {
		i++
		if ec := d.SetPropertyValue(5, "targetprop", types.NewInt(i)); ec != types.E_NONE {
			b.Fatal(ec)
		}
	}
}

func BenchmarkDirectSetObjectName(b *testing.B) {
	s := benchChainStore(b, 6, 8)
	d := s.DirectTxn()
	names := [2]string{"a", "b"}
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		i++
		if ec := d.SetObjectName(5, names[i%2]); ec != types.E_NONE {
			b.Fatal(ec)
		}
	}
}

func BenchmarkDirectMoveObject(b *testing.B) {
	s := benchChainStore(b, 6, 8)
	d := s.DirectTxn()
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		i++
		if ec := d.MoveObject(5, types.ObjID(i%2), 0); ec != types.E_NONE {
			b.Fatal(ec)
		}
	}
}
