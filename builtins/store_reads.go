package builtins

import (
	dbstore "barn/db/store"
	"barn/kernel"
	"barn/types"
)

func readTxn(ctx *kernel.TaskContext) *dbstore.StoreTxn {
	if ctx == nil {
		return nil
	}
	return ctx.StoreTxn
}

// markLiveStoreMutated records that this builtin mutated the live Store directly,
// outside the task's transaction. It flags both the context (so the scheduler will
// not retry the task — a retry would re-apply the un-rollback-able mutation) and the
// transaction (so commit uses the coarse path). Each mutating builtin separately
// adopts only the object facets changed by its own mutation.
func markLiveStoreMutated(ctx *kernel.TaskContext) {
	if ctx == nil {
		return
	}
	ctx.LiveStoreMutated = true
	if tx := readTxn(ctx); tx != nil {
		tx.MarkLiveMutated()
	}
}

// flushStagedBeforeCoarse ensures the live store reflects this task's staged
// decentralized writes (a prior create/move/recycle) before a COARSE builtin
// (renumber/chparent/add_verb/...) reads or mutates the live store mid-task, so the
// coarse builtin never observes stale live state. After it the task is treated as
// having mutated the live store directly (non-retryable, coarse commit). No-op when
// nothing is staged.
func flushStagedBeforeCoarse(ctx *kernel.TaskContext) {
	tx := readTxn(ctx)
	if tx == nil || !tx.HasStagedTopology() {
		return
	}
	tx.FlushStagedToLive()
	markLiveStoreMutated(ctx)
}

func objectExistsForRead(ctx *kernel.TaskContext, objID types.ObjID) types.ErrorCode {
	if tx := readTxn(ctx); tx != nil {
		return tx.ObjectExists(objID)
	}
	return ctx.Store.ObjectExists(objID)
}

func validForRead(ctx *kernel.TaskContext, objID types.ObjID) bool {
	if tx := readTxn(ctx); tx != nil {
		return tx.Valid(objID)
	}
	return ctx.Store.Valid(objID)
}

func isRecycledForRead(ctx *kernel.TaskContext, objID types.ObjID) bool {
	if tx := readTxn(ctx); tx != nil {
		return tx.IsRecycled(objID)
	}
	return ctx.Store.IsRecycled(objID)
}

func objectOwnerForRead(ctx *kernel.TaskContext, objID types.ObjID) (types.ObjID, types.ErrorCode) {
	if tx := readTxn(ctx); tx != nil {
		return tx.ObjectOwner(objID)
	}
	return ctx.Store.ObjectOwner(objID)
}

func objectIsAnonymousForRead(ctx *kernel.TaskContext, objID types.ObjID) (bool, types.ErrorCode) {
	if tx := readTxn(ctx); tx != nil {
		return tx.ObjectIsAnonymous(objID)
	}
	return ctx.Store.ObjectIsAnonymous(objID)
}

func hasObjectFlagForRead(ctx *kernel.TaskContext, objID types.ObjID, flag dbstore.ObjectFlags) (bool, types.ErrorCode) {
	if tx := readTxn(ctx); tx != nil {
		return tx.HasObjectFlag(objID, flag)
	}
	return ctx.Store.HasObjectFlag(objID, flag)
}

func objectAllowsForRead(ctx *kernel.TaskContext, objID types.ObjID, flag dbstore.ObjectFlags) (bool, types.ErrorCode) {
	if ctx.IsWizard {
		return true, types.E_NONE
	}
	owner, errCode := objectOwnerForRead(ctx, objID)
	if errCode != types.E_NONE {
		return false, errCode
	}
	if owner == ctx.Programmer {
		return true, types.E_NONE
	}
	return hasObjectFlagForRead(ctx, objID, flag)
}

func parentForRead(ctx *kernel.TaskContext, objID types.ObjID) (types.ObjID, types.ErrorCode) {
	if tx := readTxn(ctx); tx != nil {
		return tx.Parent(objID)
	}
	return ctx.Store.Parent(objID)
}

func parentsForRead(ctx *kernel.TaskContext, objID types.ObjID) ([]types.ObjID, types.ErrorCode) {
	if tx := readTxn(ctx); tx != nil {
		return tx.Parents(objID)
	}
	return ctx.Store.Parents(objID)
}

func childrenForRead(ctx *kernel.TaskContext, objID types.ObjID) ([]types.ObjID, types.ErrorCode) {
	if tx := readTxn(ctx); tx != nil {
		return tx.Children(objID)
	}
	return ctx.Store.Children(objID)
}

func locationForRead(ctx *kernel.TaskContext, objID types.ObjID) (types.ObjID, types.ErrorCode) {
	if tx := readTxn(ctx); tx != nil {
		return tx.Location(objID)
	}
	return ctx.Store.Location(objID)
}

func definedPropertyNamesForRead(ctx *kernel.TaskContext, objID types.ObjID) ([]string, types.ErrorCode) {
	if tx := readTxn(ctx); tx != nil {
		return tx.DefinedPropertyNames(objID)
	}
	return ctx.Store.DefinedPropertyNames(objID)
}

func findPropertyForRead(ctx *kernel.TaskContext, objID types.ObjID, name string) (dbstore.PropertyView, types.ErrorCode) {
	if tx := readTxn(ctx); tx != nil {
		return tx.FindProperty(objID, name)
	}
	return ctx.Store.FindProperty(objID, name)
}

func localPropertyForRead(ctx *kernel.TaskContext, objID types.ObjID, name string) (dbstore.PropertyView, bool, types.ErrorCode) {
	if tx := readTxn(ctx); tx != nil {
		return tx.LocalProperty(objID, name)
	}
	return ctx.Store.LocalProperty(objID, name)
}

func propertyClearStateForRead(ctx *kernel.TaskContext, objID types.ObjID, name string) (bool, types.ErrorCode) {
	if tx := readTxn(ctx); tx != nil {
		return tx.PropertyClearState(objID, name)
	}
	return ctx.Store.PropertyClearState(objID, name)
}

func verbNamesForRead(ctx *kernel.TaskContext, objID types.ObjID) ([]string, types.ErrorCode) {
	if tx := readTxn(ctx); tx != nil {
		return tx.VerbNames(objID)
	}
	return ctx.Store.VerbNames(objID)
}

func findVerbForRead(ctx *kernel.TaskContext, objID types.ObjID, name string) (dbstore.VerbView, types.ObjID, error) {
	if tx := readTxn(ctx); tx != nil {
		return tx.FindVerb(objID, name)
	}
	return ctx.Store.FindVerb(objID, name)
}

// findCallableVerbForRead resolves the verb that would actually answer
// obj:verb() — a same-named verb without execute permission does not shadow an
// executable one defined further up the ancestry chain. It mirrors
// findVerbForRead but uses the call-dispatch walk, reading through the task's
// snapshot transaction when one is present.
func findCallableVerbForRead(ctx *kernel.TaskContext, objID types.ObjID, name string) (dbstore.VerbView, types.ObjID, error) {
	if tx := readTxn(ctx); tx != nil {
		return tx.FindCallableVerb(objID, name)
	}
	return ctx.Store.FindCallableVerb(objID, name)
}

func findVerbOnObjectForRead(ctx *kernel.TaskContext, objID types.ObjID, name string) (dbstore.VerbView, error) {
	if tx := readTxn(ctx); tx != nil {
		return tx.FindVerbOnObject(objID, name)
	}
	return ctx.Store.FindVerbOnObject(objID, name)
}

func resolveVerbOnObjectForRead(ctx *kernel.TaskContext, objID types.ObjID, name string) (dbstore.ResolvedVerb, error) {
	if tx := readTxn(ctx); tx != nil {
		return tx.ResolveVerbOnObject(objID, name)
	}
	return ctx.Store.ResolveVerbOnObject(objID, name)
}

func verbByIndexForRead(ctx *kernel.TaskContext, objID types.ObjID, index int) (dbstore.VerbView, types.ErrorCode) {
	if tx := readTxn(ctx); tx != nil {
		return tx.VerbByIndex(objID, index)
	}
	return ctx.Store.VerbByIndex(objID, index)
}

func resolveVerbByIndexForRead(ctx *kernel.TaskContext, objID types.ObjID, index int) (dbstore.ResolvedVerb, types.ErrorCode) {
	if tx := readTxn(ctx); tx != nil {
		return tx.ResolveVerbByIndex(objID, index)
	}
	return ctx.Store.ResolveVerbByIndex(objID, index)
}
