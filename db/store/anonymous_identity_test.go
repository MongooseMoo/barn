package store

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// Issue #946: anonymous identities came from the numbered id counter, so
// reset_max_object() let a later create() take a live anonymous object's id,
// and every reference to the old object then named the new one. A Toast
// anonymous object has no number (objects.cc bf_create, db_make_anonymous),
// so creating one takes no number either; the expected results are the Toast
// results of moo-conformance-tests builtins/anonymous_identity_numbering.yaml.
func TestAnonymousIdentitiesNeverMeetNumberedIDs(t *testing.T) {
	s := NewStore()
	txn := s.DirectTxn()
	create := func(anonymous bool) types.ObjID {
		t.Helper()
		id, errCode := txn.CreateObject(nil, 0, anonymous)
		if errCode != types.E_NONE {
			t.Fatalf("create(anonymous=%v): %v", anonymous, errCode)
		}
		return id
	}

	a := create(false)
	anon := create(true)
	c := create(false)
	if c != a+1 || txn.MaxObject() != c {
		t.Fatalf("numbered creates around an anonymous create = #%d, #%d, max_object #%d; want consecutive ids", a, c, txn.MaxObject())
	}
	if anon < AnonymousIdentityBase {
		t.Fatalf("anonymous identity #%d is a numbered id", anon)
	}

	if err := s.Recycle(c); err != nil {
		t.Fatal(err)
	}
	s.ResetMaxObject()
	for _, anonymous := range []bool{true, true, false, false} {
		if id := create(anonymous); id == anon {
			t.Fatalf("create(anonymous=%v) after reset_max_object() took live anonymous #%d", anonymous, anon)
		}
	}
	if !txn.Valid(anon) {
		t.Fatalf("anonymous #%d is no longer valid", anon)
	}
}

// A loaded anonymous object is named by its serialization id in its file
// only. Its identity is apart from the numbered ids, so it raises neither
// max_object() nor the next numbered id, as in Toast, where the next create()
// after a restart is one past the saved max_object().
func TestLoadedAnonymousObjectTakesNoNumber(t *testing.T) {
	s := NewStore()
	if err := s.Add(NewObject(0, 0)); err != nil {
		t.Fatal(err)
	}
	builder := NewObjectBuilder(LoadedAnonymousIdentity(1))
	builder.SetAnonymous(true)
	s.AddAnonymous(builder.Build())
	if got := s.NextID(); got != 1 {
		t.Fatalf("next numbered id after loading anonymous serial #1 = #%d, want #1", got)
	}
	if !s.DirectTxn().Valid(LoadedAnonymousIdentity(1)) {
		t.Fatal("loaded anonymous object is not valid under its identity")
	}
	id, errCode := s.DirectTxn().CreateObject(nil, 0, true)
	if errCode != types.E_NONE || id <= LoadedAnonymousIdentity(1) {
		t.Fatalf("anonymous create after load = #%d, %v; want an identity past the loaded one", id, errCode)
	}
}
