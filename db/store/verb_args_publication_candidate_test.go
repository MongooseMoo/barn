package store

import (
	"reflect"
	"sync"
	"testing"

	"github.com/MongooseMoo/barn/sourcekey"
	"github.com/MongooseMoo/barn/types"
)

func TestVerbArgsCandidateSharesOnlyDetachedNodes(t *testing.T) {
	s := newVerbArgsPublicationStore(t, 2, 2)
	if ec := s.SetVerbArgs(0, "v000", publicationArgsB); ec != types.E_NONE {
		t.Fatal(ec)
	}
	borrowedNames, borrowedCode := []string{"late", "late_alias"}, []string{"return 91;"}
	if _, ec := s.AddVerb(0, NewVerb("late", borrowedNames, 0, VerbRead|VerbExecute,
		publicationArgsA, borrowedCode)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	old := s.load(0)
	before := ownPublicationView(old.verbList[0].View())
	if ec := s.SetVerbArgs(0, "alias_v000", publicationArgsA); ec != types.E_NONE {
		t.Fatal(ec)
	}
	fresh := s.load(0)
	if fresh == old || fresh.verbList[0] == old.verbList[0] {
		t.Fatal("argument update reused the old image or selected node")
	}
	if fresh.verbList[1] != old.verbList[1] {
		t.Fatal("already detached nonselected node was copied again")
	}
	if fresh.verbList[2] == old.verbList[2] {
		t.Fatal("newly borrowed nonselected node was shared")
	}
	for _, node := range fresh.verbList {
		if fresh.verbs[node.mapKey()] != node {
			t.Fatal("map and ordered list disagree about replacement identity")
		}
	}
	if fresh.verbIdx != old.verbIdx {
		t.Fatal("argument-only update rebuilt the unchanged alias index")
	}
	requirePublicationView(t, old.verbList[0].View(), before)
	late := ownPublicationView(fresh.verbList[2].View())
	borrowedNames[0], borrowedCode[0] = "external-late", "return 999;"
	requirePublicationView(t, fresh.verbList[2].View(), late)
}

func TestVerbArgsCandidateSelectsNonMapWinnerByAlias(t *testing.T) {
	s := newVerbArgsPublicationStore(t, 0, 0)
	for _, alias := range []string{"first_alias", "second_alias"} {
		if _, ec := s.AddVerb(0, NewVerb("look", []string{"look", alias}, 0,
			VerbRead|VerbExecute, publicationArgsA, []string{"return 17;"})); ec != types.E_NONE {
			t.Fatal(ec)
		}
	}
	if ec := s.SetVerbArgs(0, "first_alias", publicationArgsA); ec != types.E_NONE {
		t.Fatal(ec)
	}
	old := s.load(0)
	if ec := s.SetVerbArgs(0, "second_alias", publicationArgsB); ec != types.E_NONE {
		t.Fatal(ec)
	}
	fresh := s.load(0)
	if fresh.verbs["look"] != old.verbList[0] || fresh.verbList[0] != old.verbList[0] {
		t.Fatal("updating a shadowed primary name changed its first map winner")
	}
	if fresh.verbList[1] == old.verbList[1] || fresh.verbList[1].argSpec != publicationArgsB || old.verbList[1].argSpec != publicationArgsA {
		t.Fatal("alias did not update exactly the selected private node")
	}
}

func TestVerbArgsCandidateOldReadersRemainStableDuringLaterWrites(t *testing.T) {
	s := newVerbArgsPublicationStore(t, 3, 4)
	if ec := s.SetVerbArgs(0, "v000", publicationArgsB); ec != types.E_NONE {
		t.Fatal(ec)
	}
	tx := s.BeginSnapshot(0)
	defer tx.Release()
	old := tx.object(0)
	want := make([]VerbView, len(old.verbList))
	for i, node := range old.verbList {
		want[i] = ownPublicationView(node.View())
	}
	start, done := make(chan struct{}), make(chan struct{})
	observed := make(chan struct{})
	mismatches := make(chan string, 1)
	var readers sync.WaitGroup
	readers.Add(1)
	go func() {
		defer readers.Done()
		<-start
		firstRead := true
		for {
			for i, node := range old.verbList {
				got := node.View()
				if !reflect.DeepEqual(got, want[i]) || got.CodeKey != sourcekey.Of(got.Code) {
					mismatches <- "retained reader changed during later publication"
					if firstRead {
						close(observed)
					}
					return
				}
			}
			if firstRead {
				close(observed)
				firstRead = false
			}
			select {
			case <-done:
				return
			default:
			}
		}
	}()
	close(start)
	defer func() {
		close(done)
		readers.Wait()
		select {
		case mismatch := <-mismatches:
			t.Error(mismatch)
		default:
		}
	}()
	// Wait for a complete initial observation before starting writers. A first
	// observation mismatch also acknowledges, so failure cannot strand either
	// goroutine waiting for this handshake.
	<-observed
	select {
	case mismatch := <-mismatches:
		t.Fatal(mismatch)
	default:
	}
	for i := 0; i < 64; i++ {
		if ec := s.SetVerbArgs(0, "v000", publicationArgsA); ec != types.E_NONE {
			t.Fatal(ec)
		}
		if ec := s.DirectTxn().SetVerbCode(0, "v001", []string{"return 73;"}); ec != types.E_NONE {
			t.Fatal(ec)
		}
		if ec := s.SetVerbArgs(0, "v000", publicationArgsB); ec != types.E_NONE {
			t.Fatal(ec)
		}
		v := publicationVerb(t, s, 1)
		if v.CodeKey != sourcekey.Of(v.Code) {
			t.Fatal("later code edit lost its content key")
		}
	}
}

func TestVerbArgsCandidateCodeInstallClearsOwnershipProof(t *testing.T) {
	for _, tc := range []struct {
		name    string
		install func(*Verb, []string)
	}{
		{"owned", func(v *Verb, code []string) { v.setCodeOwned(code) }},
		{"copy", func(v *Verb, code []string) { v.setCodeCopy(code) }},
		{"loader", func(v *Verb, code []string) { v.SetCode(code) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			borrowed := NewVerb("look", []string{"look"}, 0, VerbRead, publicationArgsA, []string{"return 1;"})
			if borrowed.backingOwned {
				t.Fatal("constructor input was treated as defensively copied")
			}
			v := cloneVerbForReadTxn(&borrowed)
			if !v.backingOwned {
				t.Fatal("existing defensive clone did not establish ownership proof")
			}
			tc.install(v, []string{"return 73;"})
			if v.backingOwned || v.codeKey != sourcekey.Of(v.code) || !v.hasProgram {
				t.Fatal("code installation kept stale proof or lost source metadata")
			}
		})
	}
}

func TestVerbArgsCandidateRepairsProofAfterProductionCodeEdits(t *testing.T) {
	for _, mode := range []string{"direct-name", "direct-index", "snapshot", "coarse"} {
		t.Run(mode, func(t *testing.T) {
			s := newVerbArgsPublicationStore(t, 2, 2)
			if ec := s.SetVerbArgs(0, "v000", publicationArgsB); ec != types.E_NONE {
				t.Fatal(ec)
			}
			code := []string{"return 73;"}
			var ec types.ErrorCode
			switch mode {
			case "direct-name":
				ec = s.DirectTxn().SetVerbCode(0, "v001", code)
			case "direct-index":
				ec = s.DirectTxn().SetVerbCodeByIndex(0, 1, code)
			default:
				tx := s.BeginSnapshot(0)
				defer tx.Release()
				if ec = tx.SetVerbCode(0, "v001", code); ec != types.E_NONE {
					t.Fatal(ec)
				}
				if mode == "coarse" {
					tx.MarkLiveMutated()
				}
				ec = tx.Commit()
			}
			if ec != types.E_NONE {
				t.Fatal(ec)
			}
			edited := s.load(0).verbList[1]
			if edited.backingOwned {
				t.Fatal("production code edit kept the previous ownership proof")
			}
			if ec := s.SetVerbArgs(0, "v000", publicationArgsA); ec != types.E_NONE {
				t.Fatal(ec)
			}
			detached := s.load(0).verbList[1]
			if detached == edited || !detached.backingOwned {
				t.Fatal("next metadata publication did not detach the edited node")
			}
			if ec := s.SetVerbArgs(0, "v000", publicationArgsB); ec != types.E_NONE {
				t.Fatal(ec)
			}
			if s.load(0).verbList[1] != detached {
				t.Fatal("re-established owned node was not shared")
			}
		})
	}
}

func TestVerbArgsCandidateInvalidatesWarmDispatchMemo(t *testing.T) {
	s := newVerbArgsPublicationStore(t, 2, 1)
	if err := s.Add(NewObject(1, 0)); err != nil {
		t.Fatal(err)
	}
	if ec := s.DirectTxn().DefineProperty(1, "guard", NewProperty(types.NewInt(0), 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	if ec := s.SetVerbArgs(0, "v000", publicationArgsA); ec != types.E_NONE {
		t.Fatal(ec)
	}
	s.resetVerbDispatchMemoForTest()
	first := s.BeginSnapshot(0)
	if _, _, err := first.findVerb(0, "v000", true); err != nil {
		first.Release()
		t.Fatal(err)
	}
	first.Release()
	stale := s.BeginSnapshot(0)
	defer stale.Release()
	old, _, err := stale.findVerb(0, "v000", true)
	if err != nil || !stale.usedVerbMemo {
		t.Fatal("fixture did not resolve through the warmed dispatch memo")
	}
	// Read-only commits need not validate. Stage an independent write so this
	// memo dependency must participate in publication validation.
	if ec := stale.SetPropertyValue(1, "guard", types.NewInt(1)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	if ec := s.SetVerbArgs(0, "v000", publicationArgsB); ec != types.E_NONE {
		t.Fatal(ec)
	}
	if old.argSpec != publicationArgsA {
		t.Fatal("memo's old node was mutated")
	}
	if ec := stale.Commit(); ec != types.E_INVARG || !stale.ValidationFailed() {
		t.Fatal("argument publication did not invalidate a prior memo dependency")
	}
	fresh := s.BeginSnapshot(0)
	defer fresh.Release()
	node, _, err := fresh.findVerb(0, "v000", true)
	if err != nil || node.argSpec != publicationArgsB || node == old {
		t.Fatal("fresh resolution retained the old memo node or argument specification")
	}
}
