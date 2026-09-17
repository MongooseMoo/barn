package store

import (
	"math/rand"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// randomAlias draws from a small alphabet so exact collisions, prefix
// relationships, wildcards, colon prefixes and the catch-all "*" all occur.
func randomAlias(r *rand.Rand) string {
	words := []string{"look", "l", "get", "take", "g", "say", "who", "home", "tell", "t", "abbrev"}
	w := words[r.Intn(len(words))]
	switch r.Intn(6) {
	case 0:
		if len(w) > 1 {
			cut := 1 + r.Intn(len(w)-1)
			return w[:cut] + "*" + w[cut:]
		}
		return w + "*"
	case 1:
		return w + "*"
	case 2:
		return ":" + w
	case 3:
		if r.Intn(4) == 0 {
			return "*"
		}
		return w
	default:
		return w
	}
}

func randomVerbList(r *rand.Rand, n int) []*Verb {
	list := make([]*Verb, 0, n)
	for i := 0; i < n; i++ {
		k := 1 + r.Intn(3)
		names := make([]string, 0, k)
		for j := 0; j < k; j++ {
			names = append(names, randomAlias(r))
		}
		perms := VerbRead
		if r.Intn(3) != 0 {
			perms |= VerbExecute
		}
		v := NewVerb(strings.Join(names, " "), names, 0, perms, VerbArgs{}, nil)
		list = append(list, &v)
	}
	return list
}

func TestVerbIndexMatchesScan(t *testing.T) {
	r := rand.New(rand.NewSource(20260914))
	searches := []string{"look", "l", "lo", "loo", "get", "ge", "take", "ta", "say", "who", "home", "tell", "te", "abbrev", "abb", "x", "", ":look", ":l", ":abbrev", ":abb", "*", "lo*"}
	for round := 0; round < 500; round++ {
		list := randomVerbList(r, 1+r.Intn(12))
		idx := buildVerbIndex(list)
		if idx.n != len(list) {
			t.Fatalf("round %d: n=%d want %d", round, idx.n, len(list))
		}
		for _, s := range searches {
			for _, req := range []bool{false, true} {
				want := scanVerbList(list, s, req)
				got := idx.lookup(list, s, req)
				if want != got {
					var names []string
					for _, v := range list {
						names = append(names, strings.Join(v.lowerNames, ","))
					}
					t.Fatalf("round %d search %q exec=%v: index=%v scan=%v list=%v", round, s, req, verbName(got), verbName(want), names)
				}
			}
		}
	}
}

func verbName(v *Verb) string {
	if v == nil {
		return "<nil>"
	}
	return strings.Join(v.names, " ")
}

// requireIndexCurrent asserts the live image's index is built for its list and
// agrees with the scan for every alias it holds plus a few misses.
func requireIndexCurrent(t *testing.T, s *Store, id types.ObjID) {
	t.Helper()
	obj := s.load(id)
	if obj.verbIdx == nil && len(obj.verbList) == 0 {
		return // a fresh object has no index yet; nil means "scan"
	}
	if obj.verbIdx == nil || obj.verbIdx.n != len(obj.verbList) {
		t.Fatalf("#%d: index stale or missing (n=%v, verbs=%d)", id, obj.verbIdx != nil && obj.verbIdx.n == len(obj.verbList), len(obj.verbList))
	}
	searches := []string{"nosuch", "lo", ":look"}
	for _, v := range obj.verbList {
		searches = append(searches, v.lowerNames...)
	}
	for _, sname := range searches {
		for _, req := range []bool{false, true} {
			if got, want := obj.verbIdx.lookup(obj.verbList, sname, req), scanVerbList(obj.verbList, sname, req); got != want {
				t.Fatalf("#%d search %q exec=%v: index=%v scan=%v", id, sname, req, verbName(got), verbName(want))
			}
		}
	}
}

// The index stays current across every live mutation of a verb list: add,
// rename (set_verb_info), and delete.
func TestVerbIndexTracksLiveMutations(t *testing.T) {
	s, ids := immutFixture(t, 1)
	obj := ids[0]
	requireIndexCurrent(t, s, obj)

	if _, ec := s.AddVerb(obj, NewVerb("look l*ook", []string{"look", "l*ook"}, 0, VerbRead|VerbExecute, VerbArgs{}, nil)); ec != types.E_NONE {
		t.Fatalf("AddVerb: %v", ec)
	}
	if _, ec := s.AddVerb(obj, NewVerb("look", []string{"look"}, 0, VerbRead, VerbArgs{}, nil)); ec != types.E_NONE {
		t.Fatalf("AddVerb dup: %v", ec)
	}
	if _, ec := s.AddVerb(obj, NewVerb("g*et", []string{"g*et"}, 0, VerbRead|VerbExecute, VerbArgs{}, nil)); ec != types.E_NONE {
		t.Fatalf("AddVerb wildcard: %v", ec)
	}
	requireIndexCurrent(t, s, obj)

	if ec := s.SetVerbInfo(obj, "get", 0, VerbRead|VerbExecute, []string{"take", "t*ake"}); ec != types.E_NONE {
		t.Fatalf("SetVerbInfo: %v", ec)
	}
	requireIndexCurrent(t, s, obj)
	if got := s.load(obj).findVerbByAlias("ta", true); got == nil || got.names[0] != "take" {
		t.Fatalf("after rename, lookup ta = %v", verbName(got))
	}
	if got := s.load(obj).findVerbByAlias("ge", true); got != nil {
		t.Fatalf("after rename, old alias still resolves: %v", verbName(got))
	}

	if ec := s.DeleteVerb(obj, "look"); ec != types.E_NONE {
		t.Fatalf("DeleteVerb: %v", ec)
	}
	requireIndexCurrent(t, s, obj)

	// Transactional verb-code write publishes a new image; the index rides along.
	tx := s.BeginReadOnly(0)
	if ec := tx.SetVerbCodeByIndex(obj, 0, []string{"return 1;"}); ec != types.E_NONE {
		t.Fatalf("SetVerbCodeByIndex: %v", ec)
	}
	if ec := tx.Commit(); ec != types.E_NONE {
		t.Fatalf("Commit: %v", ec)
	}
	tx.Release()
	requireIndexCurrent(t, s, obj)
}
