package store

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func TestRenumberTransfersStagedObjectWritesWithoutPublishingPrivateView(t *testing.T) {
	for _, selfOwned := range []bool{false, true} {
		t.Run(map[bool]string{false: "other_owner", true: "self_owner"}[selfOwned], func(t *testing.T) {
			s := NewStore()
			for id := types.ObjID(0); id <= 2; id++ {
				obj := NewObject(id, 0)
				obj.setName("before")
				obj.flags = FlagRead | FlagWrite
				if err := s.Add(obj); err != nil {
					t.Fatal(err)
				}
			}
			if err := s.Recycle(1); err != nil {
				t.Fatal(err)
			}
			if _, ec := s.AddVerb(2, NewVerb("probe", []string{"probe", "alias"}, 0,
				VerbRead|VerbWrite|VerbExecute, VerbArgs{This: "this", Prep: "none", That: "this"},
				[]string{"return 1;"})); ec != types.E_NONE {
				t.Fatal(ec)
			}
			before := s.BeginSnapshot(0)
			defer before.Release()
			tx := s.BeginSnapshot(0)
			defer tx.Release()
			check := func(ec types.ErrorCode) {
				t.Helper()
				if ec != types.E_NONE {
					t.Fatal(ec)
				}
			}
			check(tx.SetObjectName(2, "after"))
			owner := types.ObjID(0)
			if selfOwned {
				owner = 2
			}
			check(tx.SetObjectOwner(2, owner))
			check(tx.SetObjectFlag(2, FlagRead, false))
			check(tx.SetObjectFlag(2, FlagWrite, false))
			check(tx.SetObjectFlag(2, FlagFertile, true))
			check(tx.SetVerbCode(2, "alias", []string{"return 42;"}))
			check(tx.SetObjectName(0, "unrelated write"))
			if err := s.Renumber(2, 1); err != nil {
				t.Fatal(err)
			}
			tx.MarkLiveMutated()
			tx.MoveStagedObjectWrites(2, 1)
			tx.ForgetObject(2)
			check(tx.AdoptLiveObject(1))
			tx.ApplyStagedObjectWrites(1)
			if selfOwned {
				owner = 1
			}
			assertWrites := func(view *StoreTxn) {
				t.Helper()
				name, ec := view.ObjectName(1)
				check(ec)
				gotOwner, ec := view.ObjectOwner(1)
				check(ec)
				flags, ec := view.ObjectFlags(1)
				check(ec)
				verb, err := view.FindVerbOnObject(1, "alias")
				if err != nil {
					t.Fatal(err)
				}
				if name != "after" || gotOwner != owner || flags != FlagFertile ||
					len(verb.Code) != 1 || verb.Code[0] != "return 42;" || view.Valid(2) {
					t.Fatalf("renumbered view: name=%q owner=%d flags=%v code=%v validOld=%v",
						name, gotOwner, flags, verb.Code, view.Valid(2))
				}
				unrelated, ec := view.ObjectName(0)
				check(ec)
				if unrelated != "unrelated write" {
					t.Fatalf("unrelated staged name = %q", unrelated)
				}
			}
			assertWrites(tx)
			liveName, ec := s.DirectTxn().ObjectName(1)
			check(ec)
			liveVerb, err := s.DirectTxn().FindVerbOnObject(1, "probe")
			if err != nil {
				t.Fatal(err)
			}
			if liveName != "before" || liveVerb.Code[0] != "return 1;" {
				t.Fatal("restoring the transaction view published staged writes")
			}
			check(tx.Commit())
			assertWrites(s.DirectTxn())
			oldName, ec := before.ObjectName(2)
			check(ec)
			oldVerb, err := before.FindVerbOnObject(2, "alias")
			if err != nil {
				t.Fatal(err)
			}
			if oldName != "before" || oldVerb.Code[0] != "return 1;" || before.Valid(1) {
				t.Fatal("renumber repair mutated the prior snapshot")
			}
		})
	}
}
