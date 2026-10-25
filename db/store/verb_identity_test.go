package store

import (
	"reflect"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

var verbIdentityArgs = VerbArgs{This: "this", Prep: "none", That: "none"}

// verbIdentityLoadedStore builds object #0 the way the loader does: verbs
// "foo" and "foo bar" appended through ObjectBuilder, so both share the first
// name "foo".
func verbIdentityLoadedStore(t *testing.T) *Store {
	t.Helper()
	s := NewStore()
	b := NewObjectBuilder(0)
	b.SetName("obj")
	b.AppendVerb(NewVerb("foo", []string{"foo"}, 0, VerbRead|VerbExecute, verbIdentityArgs, []string{"return 1;"}))
	b.AppendVerb(NewVerb("foo bar", []string{"foo", "bar"}, 0, VerbRead|VerbExecute, verbIdentityArgs, []string{"return 2;"}))
	if err := s.Add(b.Build()); err != nil {
		t.Fatal(err)
	}
	return s
}

// verbIdentityAddedStore builds the same two verbs through Store.AddVerb, the
// add_verb path.
func verbIdentityAddedStore(t *testing.T) *Store {
	t.Helper()
	s := NewStore()
	b := NewObjectBuilder(0)
	b.SetName("obj")
	if err := s.Add(b.Build()); err != nil {
		t.Fatal(err)
	}
	for _, names := range [][]string{{"foo"}, {"foo", "bar"}} {
		code := []string{"return 1;"}
		if len(names) == 2 {
			code = []string{"return 2;"}
		}
		if _, ec := s.AddVerb(0, NewVerb("", names, 0, VerbRead|VerbExecute, verbIdentityArgs, code)); ec != types.E_NONE {
			t.Fatalf("AddVerb(%v) = %v", names, ec)
		}
	}
	return s
}

func verbIdentityCodes(s *Store, objID types.ObjID) [][]string {
	var codes [][]string
	for _, v := range s.Snapshot().Objects[objID].VerbList {
		codes = append(codes, v.Code)
	}
	return codes
}

func verbIdentityCommitOne(t *testing.T, s *Store, stage func(tx *StoreTxn) types.ErrorCode) {
	t.Helper()
	tx := s.BeginSnapshot(0)
	defer tx.Release()
	if ec := stage(tx); ec != types.E_NONE {
		t.Fatalf("stage = %v", ec)
	}
	if ec := tx.Commit(); ec != types.E_NONE {
		t.Fatalf("commit = %v, conflicts %+v", ec, tx.Conflicts())
	}
}

func TestVerbCodeBySharedFirstAliasLoaded(t *testing.T) {
	s := verbIdentityLoadedStore(t)
	verbIdentityCommitOne(t, s, func(tx *StoreTxn) types.ErrorCode {
		return tx.SetVerbCodeByIndex(0, 1, []string{"return 99;"})
	})
	if got, want := verbIdentityCodes(s, 0), [][]string{{"return 1;"}, {"return 99;"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("after SetVerbCodeByIndex(0, 1): codes = %q, want %q", got, want)
	}

	s = verbIdentityLoadedStore(t)
	verbIdentityCommitOne(t, s, func(tx *StoreTxn) types.ErrorCode {
		return tx.SetVerbCode(0, "bar", []string{"return 98;"})
	})
	if got, want := verbIdentityCodes(s, 0), [][]string{{"return 1;"}, {"return 98;"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("after SetVerbCode(\"bar\"): codes = %q, want %q", got, want)
	}
}

func TestVerbCodeBySharedFirstAliasAdded(t *testing.T) {
	s := verbIdentityAddedStore(t)
	verbIdentityCommitOne(t, s, func(tx *StoreTxn) types.ErrorCode {
		return tx.SetVerbCodeByIndex(0, 1, []string{"return 99;"})
	})
	if got, want := verbIdentityCodes(s, 0), [][]string{{"return 1;"}, {"return 99;"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("after SetVerbCodeByIndex(0, 1): codes = %q, want %q", got, want)
	}
	verbIdentityCommitOne(t, s, func(tx *StoreTxn) types.ErrorCode {
		return tx.SetVerbCode(0, "bar", []string{"return 98;"})
	})
	if got, want := verbIdentityCodes(s, 0), [][]string{{"return 1;"}, {"return 98;"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("after SetVerbCode(\"bar\"): codes = %q, want %q", got, want)
	}
}

func TestDeleteVerbKeepsSiblingStagedCode(t *testing.T) {
	s := verbIdentityLoadedStore(t)
	verbIdentityCommitOne(t, s, func(tx *StoreTxn) types.ErrorCode {
		if ec := tx.SetVerbCodeByIndex(0, 0, []string{"return 77;"}); ec != types.E_NONE {
			return ec
		}
		resolved, ec := tx.ResolveVerbByIndex(0, 1)
		if ec != types.E_NONE {
			return ec
		}
		return tx.DeleteResolvedVerb(resolved)
	})
	if got, want := verbIdentityCodes(s, 0), [][]string{{"return 77;"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("codes = %q, want %q", got, want)
	}
}

// TestStagedVerbCodeSurvivesLiveRename replays add_verb, set_verb_code and
// set_verb_info in one task: the two metadata builtins mutate live and adopt,
// set_verb_code stages on the transaction.
func TestStagedVerbCodeSurvivesLiveRename(t *testing.T) {
	s := NewStore()
	b := NewObjectBuilder(0)
	b.SetName("obj")
	if err := s.Add(b.Build()); err != nil {
		t.Fatal(err)
	}
	tx := s.BeginSnapshot(0)
	defer tx.Release()
	tx.PrepareLiveMutation()
	if _, ec := s.AddVerb(0, NewVerb("old", []string{"old"}, 0, VerbRead|VerbExecute, verbIdentityArgs, nil)); ec != types.E_NONE {
		t.Fatalf("AddVerb = %v", ec)
	}
	tx.MarkLiveMutated()
	if ec := tx.AdoptLiveVerbs(0); ec != types.E_NONE {
		t.Fatalf("AdoptLiveVerbs after add = %v", ec)
	}
	if ec := tx.SetVerbCode(0, "old", []string{"return 3;"}); ec != types.E_NONE {
		t.Fatalf("SetVerbCode = %v", ec)
	}
	tx.PrepareLiveMutation()
	if ec := s.SetVerbInfo(0, "old", 0, VerbRead|VerbExecute, []string{"new", "alt"}); ec != types.E_NONE {
		t.Fatalf("SetVerbInfo = %v", ec)
	}
	tx.MarkLiveMutated()
	if ec := tx.AdoptLiveVerbs(0); ec != types.E_NONE {
		t.Fatalf("AdoptLiveVerbs after rename = %v", ec)
	}
	view, err := tx.FindVerbOnObject(0, "new")
	if err != nil {
		t.Fatalf("FindVerbOnObject(new) = %v", err)
	}
	if !reflect.DeepEqual(view.Code, []string{"return 3;"}) {
		t.Fatalf("txn view of renamed verb: code = %q, want [return 3;]", view.Code)
	}
	if ec := tx.Commit(); ec != types.E_NONE {
		t.Fatalf("commit = %v", ec)
	}
	if got, want := verbIdentityCodes(s, 0), [][]string{{"return 3;"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("codes = %q, want %q", got, want)
	}
}

// TestReadOfShadowedVerbValidates reads the second verb (by "bar") after the
// first one's version moved, and writes a property. Every fresh attempt must
// commit.
func TestReadOfShadowedVerbValidates(t *testing.T) {
	s := verbIdentityLoadedStore(t)
	verbIdentityCommitOne(t, s, func(tx *StoreTxn) types.ErrorCode {
		return tx.SetVerbCodeByIndex(0, 0, []string{"return 5;"})
	})
	if ec := s.DirectTxn().DefineProperty(0, "p", NewProperty(types.NewInt(0), 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
		t.Fatalf("DefineProperty = %v", ec)
	}
	for i := 0; i < 3; i++ {
		tx := s.BeginSnapshot(0)
		if _, _, err := tx.FindVerb(0, "bar"); err != nil {
			tx.Release()
			t.Fatalf("attempt %d: FindVerb(bar) = %v", i, err)
		}
		if ec := tx.SetPropertyValue(0, "p", types.NewInt(int64(i))); ec != types.E_NONE {
			tx.Release()
			t.Fatalf("attempt %d: SetPropertyValue = %v", i, ec)
		}
		if ec := tx.Commit(); ec != types.E_NONE {
			conflicts := tx.Conflicts()
			tx.Release()
			t.Fatalf("attempt %d: commit = %v, conflicts %+v", i, ec, conflicts)
		}
		tx.Release()
	}
}
