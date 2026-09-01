package store

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// Every path that writes Object.name must keep the shared nameVal box in step,
// otherwise `.name` reads (ObjectNameValue) would hand out a stale string.
// This drives each write site and checks the boxed value against the string.

func assertNameValueMatches(t *testing.T, s *Store, id types.ObjID, want string) {
	t.Helper()
	tx := s.DirectTxn()
	name, ec := tx.ObjectName(id)
	if ec != types.E_NONE {
		t.Fatalf("ObjectName(#%d): %v", id, ec)
	}
	val, ec := tx.ObjectNameValue(id)
	if ec != types.E_NONE {
		t.Fatalf("ObjectNameValue(#%d): %v", id, ec)
	}
	if name != want {
		t.Fatalf("ObjectName(#%d) = %q, want %q", id, name, want)
	}
	if val.Type() != types.TYPE_STR || val.Str() != want {
		t.Fatalf("ObjectNameValue(#%d) = %v, want %q", id, val, want)
	}
	// MVCC view must agree too.
	rtx := s.BeginReadOnly(0)
	mval, ec := rtx.ObjectNameValue(id)
	if ec != types.E_NONE || mval.Str() != want {
		t.Fatalf("MVCC ObjectNameValue(#%d) = %v, %v; want %q", id, mval, ec, want)
	}
}

func TestObjectNameValueTracksEveryWriteSite(t *testing.T) {
	s := NewStore()

	// Builder path.
	b := NewObjectBuilder(0)
	b.SetName("built")
	if err := s.Add(b.Build()); err != nil {
		t.Fatalf("Add: %v", err)
	}
	assertNameValueMatches(t, s, 0, "built")

	// NewObject constructor: empty name is boxed, not left as a zero Value.
	if err := s.Add(NewObject(1, 0)); err != nil {
		t.Fatalf("Add #1: %v", err)
	}
	assertNameValueMatches(t, s, 1, "")

	// Direct store setter.
	if ec := s.DirectTxn().SetObjectName(0, "direct"); ec != types.E_NONE {
		t.Fatalf("direct SetObjectName: %v", ec)
	}
	assertNameValueMatches(t, s, 0, "direct")

	// MVCC: mutable copy inside the txn, then commit (coarse republish).
	tx := s.BeginReadOnly(0)
	if ec := tx.SetObjectName(0, "mvcc"); ec != types.E_NONE {
		t.Fatalf("mvcc SetObjectName: %v", ec)
	}
	inTxn, ec := tx.ObjectNameValue(0)
	if ec != types.E_NONE || inTxn.Str() != "mvcc" {
		t.Fatalf("in-txn ObjectNameValue = %v, %v; want %q", inTxn, ec, "mvcc")
	}
	if ec := tx.Commit(); ec != types.E_NONE {
		t.Fatalf("Commit: %v", ec)
	}
	assertNameValueMatches(t, s, 0, "mvcc")

	// The box is shared across reads (no per-read allocation).
	allocs := testing.AllocsPerRun(100, func() {
		if _, ec := s.DirectTxn().ObjectNameValue(0); ec != types.E_NONE {
			t.Fatal(ec)
		}
	})
	if allocs != 0 {
		t.Errorf("ObjectNameValue allocates %.1f per read, want 0", allocs)
	}
}

// A stale box (name written around setName) must not leak: nameValue falls back
// to boxing the current string.
func TestObjectNameValueStaleBoxFallsBack(t *testing.T) {
	o := NewObject(5, 0)
	o.setName("fresh")
	o.name = "bypassed" // simulate a missed write site
	if got := o.nameValue().Str(); got != "bypassed" {
		t.Fatalf("nameValue() = %q, want %q", got, "bypassed")
	}
	var zero Object
	zero.name = "literal"
	if got := zero.nameValue().Str(); got != "literal" {
		t.Fatalf("zero-value nameValue() = %q, want %q", got, "literal")
	}
}
