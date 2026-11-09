package store

import (
	"slices"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func TestStoreTxnFlushPostReadsValidate(t *testing.T) {
	for _, concurrent := range []bool{false, true} {
		name := "unchanged"
		if concurrent {
			name = "changed"
		}
		t.Run(name, func(t *testing.T) {
			s := newCoarseAtomicTestStore(t)
			check := func(op string, got types.ErrorCode) {
				t.Helper()
				if got != types.E_NONE {
					t.Fatalf("%s: %v", op, got)
				}
			}
			direct := s.DirectTxn()
			check("define", direct.DefineProperty(0, "counter", NewProperty(types.NewInt(7), 0, PropRead|PropWrite, false, true)))
			_, ec := s.AddVerb(0, NewVerb([]string{"look"}, 0, VerbRead|VerbExecute, VerbArgs{}, []string{"return 7;"}))
			check("add verb", ec)
			tx := s.BeginSnapshot(0)
			defer tx.Release()
			_, ec = tx.ObjectName(0) // This dependency must be dropped by the flush.
			check("pre-flush name", ec)
			check("change pre-flush name", direct.SetObjectName(0, "fresh"))
			check("stage move", tx.MoveObject(1, 0, 0))
			tx.PrepareLiveMutation()
			check("flush", tx.FlushStagedToLive())
			tx.MarkLiveMutated() // Same sequence as flushStagedBeforeCoarse.
			if got, ec := tx.ObjectName(0); ec != types.E_NONE || got != "fresh" {
				t.Fatalf("post-flush name = %q, %v", got, ec)
			}
			if got, ec := tx.Location(1); ec != types.E_NONE || got != 0 {
				t.Fatalf("post-flush location = %v, %v", got, ec)
			}
			if got, ec := tx.PropertyValue(0, "counter"); ec != types.E_NONE || !got.Identical(types.NewInt(7)) {
				t.Fatalf("post-flush property = %v, %v", got, ec)
			}
			if got, ec := tx.DefinedPropertyNames(0); ec != types.E_NONE || !slices.Equal(got, []string{"counter"}) {
				t.Fatalf("post-flush property names = %v, %v", got, ec)
			}
			if _, ec := tx.FindProperty(0, "missing"); ec != types.E_PROPNF {
				t.Fatalf("post-flush missing property = %v", ec)
			}
			if got, ec := tx.VerbNames(0); ec != types.E_NONE || !slices.Equal(got, []string{"look"}) {
				t.Fatalf("post-flush verb names = %v, %v", got, ec)
			}
			if _, ec := tx.VerbByIndex(0, 0); ec != types.E_NONE {
				t.Fatalf("post-flush verb = %v", ec)
			}
			check("stage after flush", tx.SetObjectName(1, "after"))
			if concurrent {
				check("concurrent scalar", direct.SetObjectName(0, "changed"))
				check("concurrent move", direct.MoveObject(1, types.ObjNothing, 0))
				check("concurrent property", direct.SetPropertyValue(0, "counter", types.NewInt(8)))
				check("concurrent definition", direct.DefineProperty(0, "missing", NewProperty(types.NewInt(9), 0, PropRead, false, true)))
				check("concurrent verb", direct.SetVerbCodeByIndex(0, 0, []string{"return 8;"}))
				_, ec := s.AddVerb(0, NewVerb([]string{"other"}, 0, VerbRead, VerbArgs{}, nil))
				check("concurrent verb scan", ec)
			}
			want := types.E_NONE
			if concurrent {
				want = types.E_INVARG
			}
			if got := tx.Commit(); got != want {
				t.Fatalf("post-flush commit = %v, want %v", got, want)
			}
			if concurrent {
				for _, kind := range []ConflictKind{ConflictScalar, ConflictRelationship, ConflictProperty, ConflictPropertyScan, ConflictPropertyShape, ConflictVerb, ConflictVerbScan} {
					if !slices.ContainsFunc(tx.Conflicts(), func(c ReadConflict) bool { return c.Kind == kind }) {
						t.Errorf("missing post-flush conflict kind %v: %v", kind, tx.Conflicts())
					}
				}
			}
		})
	}
}

func TestStoreTxnFailedFlushPreservesPrivateViewAndReadTracking(t *testing.T) {
	s := newCoarseAtomicTestStore(t)
	if _, errCode := s.AddVerb(0, NewVerb([]string{"look"}, 0, VerbRead|VerbExecute, VerbArgs{}, nil)); errCode != types.E_NONE {
		t.Fatalf("AddVerb: %v", errCode)
	}
	tx := s.BeginSnapshot(0)
	if _, _, err := tx.FindVerb(0, "look"); err != nil {
		t.Fatalf("FindVerb memo setup: %v", err)
	}
	if errCode := tx.SetObjectName(0, "private"); errCode != types.E_NONE {
		t.Fatalf("SetObjectName stage: %v", errCode)
	}
	lazySet(&tx.verbWrites, verbKey{objID: 1, id: ^uint64(0)}, verbWrite{code: []string{"return 1;"}}) // no verb has this id

	cached := tx.objects[0]
	wantScalarRead := tx.scalarReads[0]
	if cached == nil || tx.owned[0] == 0 || tx.resolveCacheActive() {
		t.Fatal("test setup did not establish an owned private view with the resolution memo disabled")
	}

	if errCode := tx.FlushStagedToLive(); errCode != types.E_VERBNF {
		t.Fatalf("FlushStagedToLive = %v, want E_VERBNF", errCode)
	}
	if tx.objects[0] != cached {
		t.Error("failed flush replaced the transaction's private object view")
	}
	if got := tx.scalarReads[0]; got != wantScalarRead {
		t.Errorf("failed flush scalar read = %d, want preserved %d", got, wantScalarRead)
	}
	if tx.owned[0] == 0 || tx.resolveCacheActive() {
		t.Error("failed flush discarded ownership or re-enabled the resolution memo")
	}
	if got, errCode := tx.ObjectName(0); errCode != types.E_NONE || got != "private" {
		t.Errorf("failed flush private name = %q, %v; want private, E_NONE", got, errCode)
	}
}
