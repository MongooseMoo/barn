package store

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func TestVerbMutationsStampVerbVersion(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	if _, errCode := store.AddVerb(0, NewVerb("look", []string{"look"}, 0, VerbRead|VerbExecute, VerbArgs{This: "none", Prep: "none", That: "none"}, nil)); errCode != types.E_NONE {
		t.Fatalf("AddVerb failed: %v", errCode)
	}

	added := verbVersionForTest(t, store, 0, "look")
	if added == 0 {
		t.Fatalf("added verb version = 0, want stamped version")
	}
	if errCode := store.DirectTxn().SetVerbCode(0, "look", []string{"return 1;"}); errCode != types.E_NONE {
		t.Fatalf("SetVerbCode failed: %v", errCode)
	}
	code := verbVersionForTest(t, store, 0, "look")
	if code <= added {
		t.Fatalf("version after SetVerbCode = %d, want > %d", code, added)
	}
	if errCode := store.SetVerbArgs(0, "look", VerbArgs{This: "this", Prep: "none", That: "none"}); errCode != types.E_NONE {
		t.Fatalf("SetVerbArgs failed: %v", errCode)
	}
	args := verbVersionForTest(t, store, 0, "look")
	if args <= code {
		t.Fatalf("version after SetVerbArgs = %d, want > %d", args, code)
	}
	if errCode := store.SetVerbInfo(0, "look", 0, VerbRead|VerbExecute, []string{"inspect"}); errCode != types.E_NONE {
		t.Fatalf("SetVerbInfo failed: %v", errCode)
	}
	info := verbVersionForTest(t, store, 0, "inspect")
	if info <= args {
		t.Fatalf("version after SetVerbInfo = %d, want > %d", info, args)
	}
	if errCode := store.DirectTxn().SetVerbCodeByIndex(0, 0, []string{"return 2;"}); errCode != types.E_NONE {
		t.Fatalf("SetVerbCodeByIndex failed: %v", errCode)
	}
	byIndex := verbVersionForTest(t, store, 0, "inspect")
	if byIndex <= info {
		t.Fatalf("version after SetVerbCodeByIndex = %d, want > %d", byIndex, info)
	}
}

func TestAddVerbAllowsDuplicateAliasesAndFindsFirstDefinition(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	first := NewVerb("look", []string{"look", "examine"}, 0, VerbRead|VerbExecute, VerbArgs{This: "none", Prep: "none", That: "none"}, []string{"return 1;"})
	if _, errCode := store.AddVerb(0, first); errCode != types.E_NONE {
		t.Fatalf("AddVerb initial failed: %v", errCode)
	}

	index, errCode := store.AddVerb(0, NewVerb("examine", []string{"examine"}, 0, VerbRead|VerbExecute, VerbArgs{This: "none", Prep: "none", That: "none"}, []string{"return 2;"}))
	if errCode != types.E_NONE {
		t.Fatalf("AddVerb duplicate alias failed: %v", errCode)
	}
	if index != 2 {
		t.Fatalf("AddVerb duplicate alias index = %d, want 2", index)
	}

	verb, _, err := store.DirectTxn().FindVerb(0, "examine")
	if err != nil {
		t.Fatalf("FindVerb duplicate alias failed: %v", err)
	}
	if got := verb.Names[0]; got != "look" {
		t.Fatalf("FindVerb duplicate alias returned %q, want first definition look", got)
	}
}

func TestAddVerbAllowsDuplicatePrimaryNameAndFindsFirst(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	if _, errCode := store.AddVerb(0, NewVerb("look", []string{"look", "examine"}, 0, VerbRead|VerbExecute, VerbArgs{This: "none", Prep: "none", That: "none"}, nil)); errCode != types.E_NONE {
		t.Fatalf("AddVerb initial failed: %v", errCode)
	}

	if _, errCode := store.AddVerb(0, NewVerb("LOOK", []string{"LOOK"}, 0, VerbRead|VerbExecute, VerbArgs{This: "none", Prep: "none", That: "none"}, nil)); errCode != types.E_NONE {
		t.Fatalf("AddVerb duplicate primary = %v, want E_NONE", errCode)
	}
	verb, _, err := store.DirectTxn().FindVerb(0, "look")
	if err != nil || verb.Names[0] != "look" {
		t.Fatalf("FindVerb duplicate primary = (%q, %v), want first definition", verb.Names, err)
	}
}

func newOverlappingVerbStore(t *testing.T) *Store {
	t.Helper()
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	if _, errCode := store.AddVerb(0, NewVerb("look", []string{"look", "glance"}, 0, VerbRead|VerbExecute, VerbArgs{This: "none", Prep: "none", That: "none"}, nil)); errCode != types.E_NONE {
		t.Fatalf("AddVerb first failed: %v", errCode)
	}
	if _, errCode := store.AddVerb(0, NewVerb("glance", []string{"glance", "peek"}, 0, VerbRead|VerbExecute, VerbArgs{This: "none", Prep: "none", That: "none"}, nil)); errCode != types.E_NONE {
		t.Fatalf("AddVerb second failed: %v", errCode)
	}
	return store
}

func TestDeleteResolvedVerbUsesOriginalResolution(t *testing.T) {
	tests := []struct {
		name    string
		resolve func(*testing.T, *Store) ResolvedVerb
	}{
		{
			name: "string",
			resolve: func(t *testing.T, store *Store) ResolvedVerb {
				t.Helper()
				resolved, err := store.DirectTxn().ResolveVerbOnObject(0, "peek")
				if err != nil {
					t.Fatalf("ResolveVerbOnObject: %v", err)
				}
				return resolved
			},
		},
		{
			name: "index",
			resolve: func(t *testing.T, store *Store) ResolvedVerb {
				t.Helper()
				resolved, errCode := store.DirectTxn().ResolveVerbByIndex(0, 1)
				if errCode != types.E_NONE {
					t.Fatalf("ResolveVerbByIndex: %v", errCode)
				}
				return resolved
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newOverlappingVerbStore(t)
			if errCode := store.DirectTxn().DeleteResolvedVerb(test.resolve(t, store)); errCode != types.E_NONE {
				t.Fatalf("DeleteResolvedVerb: %v", errCode)
			}
			if _, err := store.DirectTxn().FindVerbOnObject(0, "look"); err != nil {
				t.Fatalf("earlier overlapping verb was deleted: %v", err)
			}
			if _, err := store.DirectTxn().FindVerbOnObject(0, "peek"); err == nil {
				t.Fatal("originally resolved verb still exists")
			}
		})
	}
}

func TestDeleteResolvedVerbRejectsStaleListWithoutRetargeting(t *testing.T) {
	store := newOverlappingVerbStore(t)
	resolved, errCode := store.DirectTxn().ResolveVerbByIndex(0, 1)
	if errCode != types.E_NONE {
		t.Fatalf("ResolveVerbByIndex: %v", errCode)
	}
	if errCode := store.DeleteVerb(0, "look"); errCode != types.E_NONE {
		t.Fatalf("DeleteVerb first: %v", errCode)
	}

	if errCode := store.DirectTxn().DeleteResolvedVerb(resolved); errCode != types.E_VERBNF {
		t.Fatalf("DeleteResolvedVerb stale reference = %v, want E_VERBNF", errCode)
	}
	if _, err := store.DirectTxn().FindVerbOnObject(0, "peek"); err != nil {
		t.Fatalf("stale resolved reference deleted a different verb: %v", err)
	}
}

func TestDeleteResolvedVerbAuthorizedUsesCurrentLiveAuthority(t *testing.T) {
	tests := []struct {
		name       string
		programmer types.ObjID
		isWizard   bool
		configure  func(*testing.T, *Store)
		want       types.ErrorCode
	}{
		{
			name:       "owner",
			programmer: 0,
			configure:  func(_ *testing.T, _ *Store) {},
			want:       types.E_NONE,
		},
		{
			name:       "write flag",
			programmer: 9,
			configure: func(t *testing.T, store *Store) {
				t.Helper()
				if errCode := store.DirectTxn().SetObjectFlag(0, FlagWrite, true); errCode != types.E_NONE {
					t.Fatalf("SetObjectFlag: %v", errCode)
				}
			},
			want: types.E_NONE,
		},
		{
			name:       "wizard",
			programmer: 9,
			isWizard:   true,
			configure:  func(_ *testing.T, _ *Store) {},
			want:       types.E_NONE,
		},
		{
			name:       "revoked owner",
			programmer: 0,
			configure: func(t *testing.T, store *Store) {
				t.Helper()
				if errCode := store.DirectTxn().SetObjectOwner(0, 1); errCode != types.E_NONE {
					t.Fatalf("SetObjectOwner: %v", errCode)
				}
			},
			want: types.E_PERM,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newOverlappingVerbStore(t)
			resolved, err := store.DirectTxn().ResolveVerbOnObject(0, "peek")
			if err != nil {
				t.Fatalf("ResolveVerbOnObject: %v", err)
			}
			test.configure(t, store)

			if errCode := store.DirectTxn().DeleteResolvedVerbAuthorized(resolved, test.programmer, test.isWizard); errCode != test.want {
				t.Fatalf("DeleteResolvedVerbAuthorized = %v, want %v", errCode, test.want)
			}
			_, err = store.DirectTxn().FindVerbOnObject(0, "peek")
			if test.want == types.E_NONE && err == nil {
				t.Fatal("authorized delete left the resolved verb in the store")
			}
			if test.want != types.E_NONE && err != nil {
				t.Fatalf("denied delete removed the resolved verb: %v", err)
			}
		})
	}
}

func TestDeleteResolvedVerbAuthorizedPreservesStaleIdentityPrecedence(t *testing.T) {
	store := newOverlappingVerbStore(t)
	resolved, errCode := store.DirectTxn().ResolveVerbByIndex(0, 1)
	if errCode != types.E_NONE {
		t.Fatalf("ResolveVerbByIndex: %v", errCode)
	}
	if errCode := store.DeleteVerb(0, "look"); errCode != types.E_NONE {
		t.Fatalf("DeleteVerb first: %v", errCode)
	}

	if errCode := store.DirectTxn().DeleteResolvedVerbAuthorized(resolved, 9, false); errCode != types.E_VERBNF {
		t.Fatalf("DeleteResolvedVerbAuthorized stale unauthorized reference = %v, want E_VERBNF", errCode)
	}
	if _, err := store.DirectTxn().FindVerbOnObject(0, "peek"); err != nil {
		t.Fatalf("stale authorized delete retargeted another verb: %v", err)
	}
}

func TestStoreTxnVerbDeleteCommitAndRenewValidatesBeforeMutation(t *testing.T) {
	store := newOverlappingVerbStore(t)
	tx := store.BeginSnapshot(0)
	if _, errCode := tx.ObjectOwner(0); errCode != types.E_NONE {
		t.Fatalf("ObjectOwner authority read: %v", errCode)
	}
	if _, errCode := tx.ObjectFlags(0); errCode != types.E_NONE {
		t.Fatalf("ObjectFlags authority read: %v", errCode)
	}
	resolved, err := tx.ResolveVerbOnObject(0, "peek")
	if err != nil {
		t.Fatalf("ResolveVerbOnObject: %v", err)
	}
	if errCode := tx.DeleteResolvedVerb(resolved); errCode != types.E_NONE {
		t.Fatalf("DeleteResolvedVerb stage: %v", errCode)
	}
	if !tx.HasStagedTopology() {
		t.Fatal("staged verb deletion is not reported as topology")
	}
	if _, err := store.DirectTxn().FindVerbOnObject(0, "peek"); err != nil {
		t.Fatalf("staged deletion mutated live store: %v", err)
	}

	if errCode := store.DirectTxn().SetObjectOwner(0, 1); errCode != types.E_NONE {
		t.Fatalf("SetObjectOwner(conflict): %v", errCode)
	}
	next, published, errCode := tx.CommitAndRenew()
	if errCode != types.E_INVARG || !tx.ValidationFailed() {
		t.Fatalf("CommitAndRenew after authority conflict = %v, validationFailed=%v; want E_INVARG, true", errCode, tx.ValidationFailed())
	}
	if next != tx || published {
		t.Fatalf("conflicted CommitAndRenew = next %p, published %v; want original %p, false", next, published, tx)
	}
	if _, err := store.DirectTxn().FindVerbOnObject(0, "peek"); err != nil {
		t.Fatalf("conflicted topology flush deleted verb: %v", err)
	}
}

func TestStoreTxnVerbDeleteCommitAndRenewAppliesAfterValidation(t *testing.T) {
	store := newOverlappingVerbStore(t)
	tx := store.BeginSnapshot(0)
	resolved, err := tx.ResolveVerbOnObject(0, "peek")
	if err != nil {
		t.Fatalf("ResolveVerbOnObject: %v", err)
	}
	if errCode := tx.DeleteResolvedVerb(resolved); errCode != types.E_NONE {
		t.Fatalf("DeleteResolvedVerb stage: %v", errCode)
	}

	next, published, errCode := tx.CommitAndRenew()
	if errCode != types.E_NONE {
		t.Fatalf("CommitAndRenew: %v", errCode)
	}
	if next == tx || !published {
		t.Fatalf("CommitAndRenew = next %p, published %v; want replacement, true", next, published)
	}
	if tx.HasWrites() {
		t.Fatal("successful validated topology flush retained staged writes")
	}
	if _, err := store.DirectTxn().FindVerbOnObject(0, "peek"); err == nil {
		t.Fatal("validated topology flush left staged deletion target live")
	}
}

func TestDeleteVerbHandlesLoadedMultiAliasRawName(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	if _, errCode := store.AddVerb(0, NewVerb("look l*ook glance", []string{"look", "l*ook", "glance"}, 0, VerbRead|VerbExecute, VerbArgs{This: "none", Prep: "none", That: "none"}, nil)); errCode != types.E_NONE {
		t.Fatalf("AddVerb loaded-style verb failed: %v", errCode)
	}

	if errCode := store.DeleteVerb(0, "glance"); errCode != types.E_NONE {
		t.Fatalf("DeleteVerb loaded-style alias: %v", errCode)
	}
	if _, err := store.DirectTxn().FindVerbOnObject(0, "glance"); err == nil {
		t.Fatal("loaded-style multi-alias verb still exists")
	}
}

func verbVersionForTest(t *testing.T, store *Store, objID types.ObjID, name string) uint64 {
	t.Helper()

	store.mu.RLock()
	defer store.mu.RUnlock()

	verb, _, err := store.findVerbLocked(objID, name)
	if err != nil {
		t.Fatalf("findVerbLocked(%q) failed: %v", name, err)
	}
	return verb.version
}

// TestDeleteVerbOfInheritedVerbIsVerbNotFound checks that DeleteVerb on an
// object that only inherits the named verb returns E_VERBNF and leaves the
// ancestor's verb in place.
func TestDeleteVerbOfInheritedVerbIsVerbNotFound(t *testing.T) {
	s := NewStore()

	// #0: parent with verb "look"
	if err := s.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add #0: %v", err)
	}
	if _, ec := s.AddVerb(0, NewVerb("look", []string{"look"}, 0, VerbRead|VerbExecute, VerbArgs{This: "none", Prep: "none", That: "none"}, nil)); ec != types.E_NONE {
		t.Fatalf("AddVerb look on #0: %v", ec)
	}

	// #1: child inheriting from #0
	childID, ec := s.DirectTxn().CreateObject([]types.ObjID{0}, 0, false)
	if ec != types.E_NONE {
		t.Fatalf("CreateObject child: %v", ec)
	}

	// Precondition: child does NOT have "look" defined locally.
	if s.HasLocalVerb(childID, "look") {
		t.Fatal("precondition failed: child should not have look locally")
	}

	// DeleteVerb on the child for an inherited verb must return E_VERBNF,
	// because the verb is not defined on the child itself.
	ec = s.DeleteVerb(childID, "look")
	if ec == types.E_NONE {
		t.Errorf("DeleteVerb on inherited verb returned E_NONE (silent success); want E_VERBNF")
	}

	// Additionally confirm the parent's verb was not touched.
	_, definer, err := s.DirectTxn().FindVerb(0, "look")
	if err != nil {
		t.Errorf("parent verb 'look' missing after DeleteVerb on child: %v", err)
	} else if definer != 0 {
		t.Errorf("parent verb 'look' found on #%d, want #0", definer)
	}
}

// TestSetVerbCodeOfInheritedVerbLeavesAncestor checks that SetVerbCode on an
// object that only inherits the named verb returns E_VERBNF and leaves the
// ancestor's code unchanged.
func TestSetVerbCodeOfInheritedVerbLeavesAncestor(t *testing.T) {
	s := NewStore()

	// #0: parent with verb "look" having specific code
	if err := s.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add #0: %v", err)
	}
	originalCode := []string{"return 1;"}
	if _, ec := s.AddVerb(0, NewVerb("look", []string{"look"}, 0, VerbRead|VerbExecute, VerbArgs{This: "none", Prep: "none", That: "none"}, originalCode)); ec != types.E_NONE {
		t.Fatalf("AddVerb look on #0: %v", ec)
	}

	// #1: child inheriting from #0
	childID, ec := s.DirectTxn().CreateObject([]types.ObjID{0}, 0, false)
	if ec != types.E_NONE {
		t.Fatalf("CreateObject child: %v", ec)
	}

	newCode := []string{"return 2;"}
	ec = s.DirectTxn().SetVerbCode(childID, "look", newCode)
	if ec == types.E_NONE {
		t.Errorf("SetVerbCode on inherited verb returned E_NONE; want E_VERBNF")
	}

	// Even if SetVerbCode returns E_NONE, verify the parent's code is untouched.
	parentVerb, err := s.DirectTxn().FindVerbOnObject(0, "look")
	if err != nil {
		t.Fatalf("parent verb 'look' not found: %v", err)
	}
	if len(parentVerb.Code) > 0 && parentVerb.Code[0] == newCode[0] {
		t.Errorf("SetVerbCode on child mutated parent verb: parent code[0] = %q, want %q",
			parentVerb.Code[0], originalCode[0])
	}
}

// TestSetVerbInfoOfInheritedVerbLeavesAncestor checks that SetVerbInfo on an
// object that only inherits the named verb returns E_VERBNF and leaves the
// ancestor's permissions unchanged.
func TestSetVerbInfoOfInheritedVerbLeavesAncestor(t *testing.T) {
	s := NewStore()

	// #0: parent with verb "look" that is readable AND executable
	if err := s.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add #0: %v", err)
	}
	originalPerms := VerbRead | VerbExecute
	if _, ec := s.AddVerb(0, NewVerb("look", []string{"look"}, 0, originalPerms, VerbArgs{This: "none", Prep: "none", That: "none"}, nil)); ec != types.E_NONE {
		t.Fatalf("AddVerb look on #0: %v", ec)
	}

	// #1: child inheriting from #0
	childID, ec := s.DirectTxn().CreateObject([]types.ObjID{0}, 0, false)
	if ec != types.E_NONE {
		t.Fatalf("CreateObject child: %v", ec)
	}

	// SetVerbInfo stripping the read permission via the child
	newPerms := VerbExecute // no VerbRead
	ec = s.SetVerbInfo(childID, "look", 0, newPerms, []string{"look"})
	if ec == types.E_NONE {
		t.Errorf("SetVerbInfo on inherited verb returned E_NONE; want E_VERBNF")
	}

	// Verify the parent verb is untouched.
	parentVerb, err := s.DirectTxn().FindVerbOnObject(0, "look")
	if err != nil {
		t.Fatalf("parent verb 'look' not found: %v", err)
	}
	if !parentVerb.Perms.Has(VerbRead) {
		t.Errorf("SetVerbInfo on child stripped VerbRead from parent's verb")
	}
}
