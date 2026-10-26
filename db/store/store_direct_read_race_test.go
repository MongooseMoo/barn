package store

import (
	"sync"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// The direct transaction is shared: goroutines call its read methods while
// coarse writers and commits change the objects they read. Run under -race,
// any transaction state a direct read writes shows up as a data race.
func TestDirectReadsRaceWritersAndCommits(t *testing.T) {
	s := oneBodyStore(t)
	d := s.DirectTxn()
	room, _ := d.CreateObject([]types.ObjID{0}, 0)
	parent, _ := d.CreateObject([]types.ObjID{0}, 0)
	child, _ := d.CreateObject([]types.ObjID{parent}, 0)
	thing, _ := d.CreateObject([]types.ObjID{child}, 0)
	anon, _ := d.CreateObject([]types.ObjID{parent}, 0, true)
	if ec := d.DefineProperty(parent, "p_x", NewProperty(types.NewInt(1), 0, PropRead, false, true)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	if _, ec := s.AddVerb(parent, NewVerb([]string{"go"}, 0, VerbRead|VerbExecute, VerbArgs{}, []string{"return 1;"})); ec != types.E_NONE {
		t.Fatal(ec)
	}
	if _, ec := s.AddVerb(child, NewVerb([]string{"go"}, 0, VerbRead|VerbExecute, VerbArgs{}, []string{"return pass();"})); ec != types.E_NONE {
		t.Fatal(ec)
	}

	const rounds = 200
	stop := make(chan struct{})
	var readers sync.WaitGroup
	for range 4 {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				for _, id := range []types.ObjID{room, parent, child, thing, anon} {
					d.ObjectExists(id)
					d.Valid(id)
					d.ObjectName(id)
					d.ObjectNameValue(id)
					d.ObjectOwner(id)
					d.ObjectFlags(id)
					d.HasObjectFlag(id, FlagRead)
					d.ObjectIsAnonymous(id)
					d.IsRecycled(id)
					d.FindProperty(id, "p_x")
					d.PropertyValue(id, "p_x")
					d.PropertyValues(id)
					d.LocalProperty(id, "p_x")
					d.DefinedPropertyNames(id)
					d.TruthyPropertiesWithPrefixInAncestry(id, "p_")
					d.PropertyClearState(id, "p_x")
					d.DefinedPropertyNamesInAncestry(id)
					d.HasDefinedPropertyConflictWithAncestry(id, []types.ObjID{0})
					d.HasChparentDescendantPropertyConflict(id, map[string]bool{"p_x": true})
					d.HasDefinedPropertyInDescendants(id, "p_x")
					d.HasContentDescendant(room, id)
					d.Parent(id)
					d.Parents(id)
					d.Children(id)
					d.Ancestors(id, true)
					d.Descendants(id, true)
					d.HasAncestor(id, parent)
					d.AnonymousChildren(id)
					d.Contents(id)
					d.Location(id)
					d.LastMove(id)
					d.FindVerb(id, "go")
					d.FindCallableVerb(id, "go")
					d.FindVerbOnObject(id, "go")
					d.ResolveVerbOnObject(id, "go")
					d.VerbNames(id)
					d.VerbByIndex(id, 0)
					d.ResolveVerbByIndex(id, 0)
					d.FindParentVerb(id, "go")
					d.ObjectByteEstimate(id)
				}
				d.HasDuplicateDefinedPropertyAmong([]types.ObjID{parent, room})
				d.MaxObject()
				d.ReadTimestamp()
				d.ExpandAnonymousReachability(map[types.ObjID]struct{}{}, map[types.ObjID]struct{}{anon: {}})
			}
		}()
	}

	for i := range rounds {
		tx := s.BeginSnapshot(0)
		tx.SetPropertyValue(child, "p_x", types.NewInt(int64(i)))
		tx.MoveObject(thing, room, 0)
		tx.SetObjectName(thing, "thing")
		tx.Commit()
		tx.Release()
		d.SetPropertyValue(anon, "p_x", types.NewInt(int64(i)))
		d.SetObjectName(child, "child")
		d.MoveObject(thing, types.ObjNothing, 0)
		if _, ec := s.AddVerb(room, NewVerb([]string{"v"}, 0, VerbRead, VerbArgs{}, nil)); ec != types.E_NONE {
			t.Error(ec)
		}
		if ec := s.ChangeParents(thing, []types.ObjID{parent}); ec != types.E_NONE {
			t.Error(ec)
		}
		if ec := s.ChangeParents(thing, []types.ObjID{child}); ec != types.E_NONE {
			t.Error(ec)
		}
	}
	close(stop)
	readers.Wait()
}
