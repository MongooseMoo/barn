package format

import (
	"math"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// Use real construction and writer/reader paths, including shared WAIFs, a WAIF
// cycle, an anonymous self-reference, inherited CLEAR slots and empty programs.
func semanticRoundTripFixture(t *testing.T) *store.Store {
	t.Helper()
	s := store.NewStore()
	w := types.NewWaif(0, 0)
	w.SetProperty("n", types.NewStr("CaseSensitive"))
	w.SetProperty("self", w)
	w.SetProperty("not_declared", types.NewInt(99)) // Not part of the class schema.
	other := types.NewWaif(0, 0)
	other.SetProperty("n", types.NewStr("CaseSensitive"))
	other.SetProperty("self", other)
	root := store.NewObjectBuilder(0)
	root.SetName("Root")
	root.SetOwner(0)
	root.SetFlags(store.FlagUser)
	root.SetLocation(types.ObjNothing)
	root.SetChildren([]types.ObjID{1})
	root.SetContents([]types.ObjID{1})
	names := []string{"Payload", "Alias", ":n", ":self", "Other"}
	root.SetPropOrder(names)
	root.SetPropDefsCount(len(names))
	payload := types.NewList([]types.Value{
		types.NewEmptyMap().MapSet(types.NewStr("Key"), types.NewList([]types.Value{types.NewInt(1), types.NewStr("Last")})),
		types.NewBool(true), types.NewFloat(math.Copysign(0, -1)), types.NewFloat(math.NaN()),
		types.None, types.Unbound, types.NewAnon(37), types.NewAnon(37), types.NewAnon(99), w,
	})
	for i, name := range names {
		value := types.NewInt(int64(i))
		if i == 0 {
			value = payload
		} else if i == 1 {
			value = w
		} else if i == 4 {
			value = other
		}
		root.SetProperty(name, store.NewProperty(value, 0, store.PropertyPerms(3), false, true))
	}
	args := store.VerbArgs{This: "this", Prep: "with", That: "any"}
	root.AppendVerb(store.NewVerb([]string{"run", "r*un"}, 0, store.VerbPerms(5), args, []string{"return 1;"}))
	root.AppendVerb(store.NewVerb([]string{"empty"}, 0, 0, args, nil))
	root.SetVerbCodeByIndex(1, []string{})
	root.AppendVerb(store.NewVerb([]string{"unprogrammed"}, 0, 0, args, nil))
	child := store.NewObjectBuilder(1)
	child.SetName("Child")
	child.SetOwner(0)
	child.SetLocation(0)
	child.SetParents([]types.ObjID{0})
	for _, name := range names {
		child.SetProperty(name, store.NewProperty(types.NewInt(999), 0, 3, true, false))
	}
	for _, obj := range []*store.ObjectBuilder{root, child} {
		if err := s.Add(obj.Build()); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []types.ObjID{37, 99} {
		anon := store.NewObjectBuilder(id)
		anon.SetAnonymous(true)
		anon.SetName("Anonymous")
		anon.SetOwner(0)
		anon.SetLocation(types.ObjNothing)
		anon.SetPropOrder([]string{"nested"})
		anon.SetPropDefsCount(1)
		anon.SetProperty("nested", store.NewProperty(types.NewAnon(id), 0, 1, false, true))
		s.AddAnonymous(anon.Build())
	}
	s.SetPendingFinalizations([]types.Value{types.NewStr("pending")})
	return s
}

func TestCompareRoundTripStoresWriterReader(t *testing.T) {
	original := semanticRoundTripFixture(t)
	reloaded, err := dumpAndReload(original)
	if err != nil {
		t.Fatal(err)
	}
	if err := CompareRoundTripStores(original, reloaded); err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Snapshot().AnonymousObjects) != 2 {
		t.Fatal("fixture did not exercise both anonymous objects")
	}
}

func TestCompareRoundTripStoresTrackedDatabases(t *testing.T) {
	for _, path := range []string{"../../Test_fresh2.db", "testdata/Anon6.db", "testdata/toastcore.db"} {
		t.Run(path, func(t *testing.T) {
			database, err := LoadDatabase(path)
			if err != nil {
				t.Fatal(err)
			}
			s, err := database.NewStoreFromDatabase()
			if err != nil {
				t.Fatal(err)
			}
			// As in FuzzReadDatabase, the first dump normalizes loaded slot
			// layouts. Check the semantic idempotence of the next dump.
			first, err := dumpAndReload(s)
			if err != nil {
				t.Fatal(err)
			}
			second, err := dumpAndReload(first)
			if err != nil {
				t.Fatal(err)
			}
			if err := CompareRoundTripStores(first, second); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCompareRoundTripStoresDetectsMutation(t *testing.T) {
	property := func(s *store.Snapshot, change func(*store.PropertyView)) {
		p := s.Objects[0].Properties["payload"]
		change(&p)
		s.Objects[0].Properties["payload"] = p
	}
	verb := func(s *store.Snapshot) *store.VerbView { return &s.Objects[0].VerbList[0] }
	waif := func(s *store.Snapshot, class, owner types.ObjID, value string) {
		w := types.NewWaif(class, owner)
		w.SetProperty("n", types.NewStr(value))
		w.SetProperty("self", w)
		p := s.Objects[0].Properties["alias"]
		p.Value = w
		s.Objects[0].Properties["alias"] = p
		property(s, func(p *store.PropertyView) { p.Value = p.Value.Set(10, w) })
	}
	cases := []struct {
		name, witness string
		mutate        func(*store.Snapshot)
	}{
		{"max object", "max object", func(s *store.Snapshot) { s.MaxObject++ }},
		{"same player count", "player identities", func(s *store.Snapshot) { s.Players[0] = 1 }},
		{"object missing", "existence", func(s *store.Snapshot) { delete(s.Objects, 1) }},
		{"name case", "name/flags/owner", func(s *store.Snapshot) { s.Objects[0].Name = "root" }},
		{"flags", "name/flags/owner", func(s *store.Snapshot) { s.Objects[0].Flags ^= store.FlagRead }},
		{"owner", "name/flags/owner", func(s *store.Snapshot) { s.Objects[0].Owner = 1 }},
		{"location", "relationships", func(s *store.Snapshot) { s.Objects[1].Location = -1 }},
		{"parents", "relationships", func(s *store.Snapshot) { s.Objects[1].Parents = nil }},
		{"children", "relationships", func(s *store.Snapshot) { s.Objects[0].Children = nil }},
		{"contents", "relationships", func(s *store.Snapshot) { s.Objects[0].Contents = nil }},
		{"last move", "last_move", func(s *store.Snapshot) { s.Objects[0].LastMove = types.NewInt(1) }},
		{"definitions", "property definitions/order", func(s *store.Snapshot) { s.Objects[0].PropDefsCount-- }},
		{"property display case", "property definitions/order", func(s *store.Snapshot) { s.PropertyNames[0][0] = "payload" }},
		{"property order", "property definitions/order", func(s *store.Snapshot) {
			s.PropertyNames[0][0], s.PropertyNames[0][1] = s.PropertyNames[0][1], s.PropertyNames[0][0]
		}},
		{"property value", "value/metadata", func(s *store.Snapshot) { property(s, func(p *store.PropertyView) { p.Value = types.NewInt(0) }) }},
		{"property owner", "value/metadata", func(s *store.Snapshot) { property(s, func(p *store.PropertyView) { p.Owner = 1 }) }},
		{"property perms", "value/metadata", func(s *store.Snapshot) { property(s, func(p *store.PropertyView) { p.Perms ^= 1 }) }},
		{"property clear", "value/metadata", func(s *store.Snapshot) { property(s, func(p *store.PropertyView) { p.Clear = true }) }},
		{"boolean type", "value/metadata", func(s *store.Snapshot) {
			property(s, func(p *store.PropertyView) { p.Value = p.Value.Set(2, types.NewInt(1)) })
		}},
		{"signed zero", "value/metadata", func(s *store.Snapshot) {
			property(s, func(p *store.PropertyView) { p.Value = p.Value.Set(3, types.NewFloat(0)) })
		}},
		{"NaN replaced", "value/metadata", func(s *store.Snapshot) {
			property(s, func(p *store.PropertyView) { p.Value = p.Value.Set(4, types.NewFloat(0)) })
		}},
		{"unbound versus clear", "value/metadata", func(s *store.Snapshot) {
			property(s, func(p *store.PropertyView) { p.Value = p.Value.Set(6, types.None) })
		}},
		{"last list element", "value/metadata", func(s *store.Snapshot) {
			property(s, func(p *store.PropertyView) { p.Value = p.Value.Set(p.Value.Len(), types.NewInt(0)) })
		}},
		{"nested map value case", "value/metadata", func(s *store.Snapshot) {
			property(s, func(p *store.PropertyView) {
				p.Value = p.Value.Set(1, types.NewEmptyMap().MapSet(types.NewStr("Key"), types.NewList([]types.Value{types.NewInt(1), types.NewStr("last")})))
			})
		}},
		{"verb alias", "verb 0", func(s *store.Snapshot) { verb(s).Names = []string{"different"} }},
		{"verb owner", "verb 0", func(s *store.Snapshot) { verb(s).Owner = 1 }},
		{"verb perms", "verb 0", func(s *store.Snapshot) { verb(s).Perms ^= 1 }},
		{"verb args", "verb 0", func(s *store.Snapshot) { verb(s).ArgSpec.This = "any" }},
		{"verb prep", "verb 0", func(s *store.Snapshot) { verb(s).ArgSpec.Prep = "at/to" }},
		{"verb source", "verb 0", func(s *store.Snapshot) { verb(s).Code = []string{"return 2;"} }},
		{"empty program presence", "verb 1", func(s *store.Snapshot) { s.Objects[0].VerbList[1].HasProgram = false }},
		{"anonymous missing", "anonymous objects", func(s *store.Snapshot) { s.AnonymousObjects = s.AnonymousObjects[:1] }},
		{"anonymous identity", "identity", func(s *store.Snapshot) { s.AnonymousObjects[0].ID++ }},
		{"anonymous value", "value/metadata", func(s *store.Snapshot) {
			o := s.AnonymousObjects[0]
			p := o.Properties["nested"]
			p.Value = types.NewInt(0)
			o.Properties["nested"] = p
		}},
		{"anonymous alias", "value/metadata", func(s *store.Snapshot) {
			property(s, func(p *store.PropertyView) { p.Value = p.Value.Set(8, types.NewAnon(s.AnonymousObjects[1].ID)) })
		}},
		{"waif alias split", "value/metadata", func(s *store.Snapshot) {
			w := types.NewWaif(0, 0)
			w.SetProperty("n", types.NewStr("CaseSensitive"))
			w.SetProperty("self", w)
			p := s.Objects[0].Properties["alias"]
			p.Value = w
			s.Objects[0].Properties["alias"] = p
		}},
		{"waif alias merge", "value/metadata", func(s *store.Snapshot) {
			p := s.Objects[0].Properties["other"]
			p.Value = s.Objects[0].Properties["alias"].Value
			s.Objects[0].Properties["other"] = p
		}},
		{"waif class", "value/metadata", func(s *store.Snapshot) { waif(s, 1, 0, "CaseSensitive") }},
		{"waif owner", "value/metadata", func(s *store.Snapshot) { waif(s, 0, 1, "CaseSensitive") }},
		{"waif override case", "value/metadata", func(s *store.Snapshot) { waif(s, 0, 0, "casesensitive") }},
		{"pending finalization", "pending finalizations", func(s *store.Snapshot) { s.PendingFinalizations = nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, b := semanticRoundTripFixture(t).Snapshot(), semanticRoundTripFixture(t).Snapshot()
			if err := compareRoundTripSnapshots(a, b); err != nil {
				t.Fatalf("unmodified fixture: %v", err)
			}
			tc.mutate(&b)
			if err := compareRoundTripSnapshots(a, b); err == nil || !strings.Contains(err.Error(), tc.witness) {
				t.Fatalf("mutation %q: got %v, want witness %q", tc.name, err, tc.witness)
			}
		})
	}
}
