package engine

import (
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
	"github.com/MongooseMoo/barn/vm"
)

// installConflictSite gives the slice's transaction a callback naming the verb
// and line machine is executing and the ticks the slice has run since entry,
// for the conflict census. Nothing is installed unless census tracking is on.
func (s *Runtime) installConflictSite(ctx *task.Context, machine *vm.VM, entry int64) {
	if s.store == nil || !s.store.ConflictCensusTracking() || ctx == nil || ctx.StoreTxn == nil {
		return
	}
	ctx.StoreTxn.SetConflictSiteFunc(func() (dbstore.ConflictSite, int64) {
		site := dbstore.ConflictSite{Obj: types.ObjNothing}
		if frame := machine.CurrentFrame(); frame != nil {
			site.Obj = frame.VerbLoc
			site.Verb = frame.Verb
			site.Line = machine.CurrentLine()
		}
		return site, machine.Ticks - entry
	})
}
