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

func findVerbOnObjectForRead(ctx *kernel.TaskContext, objID types.ObjID, name string) (dbstore.VerbView, error) {
	if tx := readTxn(ctx); tx != nil {
		return tx.FindVerbOnObject(objID, name)
	}
	return ctx.Store.FindVerbOnObject(objID, name)
}

func verbByIndexForRead(ctx *kernel.TaskContext, objID types.ObjID, index int) (dbstore.VerbView, types.ErrorCode) {
	if tx := readTxn(ctx); tx != nil {
		return tx.VerbByIndex(objID, index)
	}
	return ctx.Store.VerbByIndex(objID, index)
}
