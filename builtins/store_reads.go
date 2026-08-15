package builtins

import (
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

func readTxn(ctx *Execution) *dbstore.StoreTxn {
	ctx.ensureStoreTxn()
	return ctx.StoreTxn
}

// markLiveStoreMutated records that this builtin mutated the live Store directly,
// outside the task's transaction. It flags both the context (so the engine will
// not retry the task — a retry would re-apply the un-rollback-able mutation) and the
// transaction (so commit uses the coarse path). Each mutating builtin separately
// adopts only the object facets changed by its own mutation.
func markLiveStoreMutated(ctx *Execution) {
	if ctx == nil {
		return
	}
	ctx.LiveStoreMutated = true
	readTxn(ctx).MarkLiveMutated()
}

// flushStagedBeforeCoarse ensures the live store reflects this task's staged
// decentralized writes (a prior create/move/recycle) before a COARSE builtin
// (renumber/chparent/add_verb/...) reads or mutates the live store mid-task, so the
// coarse builtin never observes stale live state. After it the task is treated as
// having mutated the live store directly (non-retryable, coarse commit). A flush
// failure is returned without marking the task live-mutated. Read-set validation
// conflicts remain retryable with staged writes intact; terminal operation-preflight
// failures retain the private view but make the transaction non-recommittable. No-op
// when nothing is staged.
func flushStagedBeforeCoarse(ctx *Execution) types.ErrorCode {
	tx := readTxn(ctx)
	if !tx.HasStagedTopology() {
		return types.E_NONE
	}
	if tx.HasStagedVerbDeletes() {
		next, published, errCode := tx.CommitAndRenew()
		if errCode != types.E_NONE {
			return errCode
		}
		ctx.StoreTxn = next
		if published {
			markLiveStoreMutated(ctx)
		}
		return types.E_NONE
	}
	if errCode := tx.FlushStagedToLive(); errCode != types.E_NONE {
		return errCode
	}
	markLiveStoreMutated(ctx)
	return types.E_NONE
}

func objectExistsForRead(ctx *Execution, objID types.ObjID) types.ErrorCode {
	return readTxn(ctx).ObjectExists(objID)
}

func validForRead(ctx *Execution, objID types.ObjID) bool {
	return readTxn(ctx).Valid(objID)
}

func isRecycledForRead(ctx *Execution, objID types.ObjID) bool {
	return readTxn(ctx).IsRecycled(objID)
}

func objectOwnerForRead(ctx *Execution, objID types.ObjID) (types.ObjID, types.ErrorCode) {
	return readTxn(ctx).ObjectOwner(objID)
}

func objectIsAnonymousForRead(ctx *Execution, objID types.ObjID) (bool, types.ErrorCode) {
	return readTxn(ctx).ObjectIsAnonymous(objID)
}

func hasObjectFlagForRead(ctx *Execution, objID types.ObjID, flag dbstore.ObjectFlags) (bool, types.ErrorCode) {
	return readTxn(ctx).HasObjectFlag(objID, flag)
}

func objectAllowsForRead(ctx *Execution, objID types.ObjID, flag dbstore.ObjectFlags) (bool, types.ErrorCode) {
	if ctx.IsWizard {
		return dbstore.ObjectAllows(types.ObjNothing, 0, ctx.Programmer, true, flag), types.E_NONE
	}
	owner, errCode := objectOwnerForRead(ctx, objID)
	if errCode != types.E_NONE {
		return false, errCode
	}
	if dbstore.ObjectAllows(owner, 0, ctx.Programmer, false, flag) {
		return true, types.E_NONE
	}
	flags, errCode := readTxn(ctx).ObjectFlags(objID)
	if errCode != types.E_NONE {
		return false, errCode
	}
	return dbstore.ObjectAllows(owner, flags, ctx.Programmer, ctx.IsWizard, flag), types.E_NONE
}

func parentForRead(ctx *Execution, objID types.ObjID) (types.ObjID, types.ErrorCode) {
	return readTxn(ctx).Parent(objID)
}

func parentsForRead(ctx *Execution, objID types.ObjID) ([]types.ObjID, types.ErrorCode) {
	return readTxn(ctx).Parents(objID)
}

func childrenForRead(ctx *Execution, objID types.ObjID) ([]types.ObjID, types.ErrorCode) {
	return readTxn(ctx).Children(objID)
}

func locationForRead(ctx *Execution, objID types.ObjID) (types.ObjID, types.ErrorCode) {
	return readTxn(ctx).Location(objID)
}

func definedPropertyNamesForRead(ctx *Execution, objID types.ObjID) ([]string, types.ErrorCode) {
	return readTxn(ctx).DefinedPropertyNames(objID)
}

func findPropertyForRead(ctx *Execution, objID types.ObjID, name string) (dbstore.PropertyView, types.ErrorCode) {
	return readTxn(ctx).FindProperty(objID, name)
}

func localPropertyForRead(ctx *Execution, objID types.ObjID, name string) (dbstore.PropertyView, bool, types.ErrorCode) {
	return readTxn(ctx).LocalProperty(objID, name)
}

func propertyClearStateForRead(ctx *Execution, objID types.ObjID, name string) (bool, types.ErrorCode) {
	return readTxn(ctx).PropertyClearState(objID, name)
}

func verbNamesForRead(ctx *Execution, objID types.ObjID) ([]string, types.ErrorCode) {
	return readTxn(ctx).VerbNames(objID)
}

func findVerbForRead(ctx *Execution, objID types.ObjID, name string) (dbstore.VerbView, types.ObjID, error) {
	return readTxn(ctx).FindVerb(objID, name)
}

// findCallableVerbForRead resolves the verb that would actually answer
// obj:verb() — a same-named verb without execute permission does not shadow an
// executable one defined further up the ancestry chain. It mirrors
// findVerbForRead but uses the call-dispatch walk, reading through the task's
// snapshot transaction when one is present.
func findCallableVerbForRead(ctx *Execution, objID types.ObjID, name string) (dbstore.VerbView, types.ObjID, error) {
	return readTxn(ctx).FindCallableVerb(objID, name)
}

func findVerbOnObjectForRead(ctx *Execution, objID types.ObjID, name string) (dbstore.VerbView, error) {
	return readTxn(ctx).FindVerbOnObject(objID, name)
}

func resolveVerbOnObjectForRead(ctx *Execution, objID types.ObjID, name string) (dbstore.ResolvedVerb, error) {
	return readTxn(ctx).ResolveVerbOnObject(objID, name)
}

func verbByIndexForRead(ctx *Execution, objID types.ObjID, index int) (dbstore.VerbView, types.ErrorCode) {
	return readTxn(ctx).VerbByIndex(objID, index)
}

func resolveVerbByIndexForRead(ctx *Execution, objID types.ObjID, index int) (dbstore.ResolvedVerb, types.ErrorCode) {
	return readTxn(ctx).ResolveVerbByIndex(objID, index)
}
