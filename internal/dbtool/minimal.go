package dbtool

import (
	"fmt"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// minimalWizard owns every object of the minimal database.
const minimalWizard = types.ObjID(3)

// minimalObjects is the object graph of the classic Minimal.db: a system
// object, the root class the other three descend from, a room, and a wizard
// standing in it. The flags are the ones that file stores. Its verbs
// (#0:do_login_command and friends, #2:eval) exist to serve network logins and
// are left out.
var minimalObjects = []struct {
	name     string
	flags    dbstore.ObjectFlags
	location types.ObjID
	parents  []types.ObjID
	children []types.ObjID
	contents []types.ObjID
}{
	{name: "System Object", flags: dbstore.FlagRead, location: types.ObjNothing, parents: []types.ObjID{1}},
	{name: "Root Class", flags: dbstore.FlagRead, location: types.ObjNothing, children: []types.ObjID{0, 2, 3}},
	{name: "The First Room", location: types.ObjNothing, parents: []types.ObjID{1}, contents: []types.ObjID{minimalWizard}},
	{name: "Wizard", flags: dbstore.FlagUser | dbstore.FlagProgrammer | dbstore.FlagWizard, location: 2, parents: []types.ObjID{1}},
}

// NewMinimalStore builds a fresh in-memory Minimal.db, the database a MOO
// program runs in when no database file is named. Objects go in the way the
// loader puts them in: finished builders, each carrying both ends of its
// parent/child and location/contents links.
func NewMinimalStore() (*dbstore.Store, error) {
	store := dbstore.NewStore()
	for id, object := range minimalObjects {
		b := dbstore.NewObjectBuilder(types.ObjID(id))
		b.SetName(object.name)
		b.SetOwner(minimalWizard)
		b.SetFlags(object.flags)
		b.SetLocation(object.location)
		b.SetParents(append([]types.ObjID(nil), object.parents...))
		b.SetChildren(append([]types.ObjID(nil), object.children...))
		b.SetContents(append([]types.ObjID(nil), object.contents...))
		if err := store.Add(b.Build()); err != nil {
			return nil, fmt.Errorf("add minimal object #%d: %w", id, err)
		}
	}
	return store, nil
}
