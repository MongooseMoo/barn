package store

import (
	"reflect"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func TestDirectTxnWritesThroughWithoutStaging(t *testing.T) {
	s := NewStore()
	obj := NewObject(0, types.ObjNothing)
	if err := s.Add(obj); err != nil {
		t.Fatalf("add object: %v", err)
	}

	tx := s.DirectTxn()
	if tx == nil {
		t.Fatal("DirectTxn returned nil")
	}
	if tx != s.DirectTxn() {
		t.Fatal("DirectTxn allocated a new transaction")
	}
	if errCode := tx.SetObjectName(0, "direct"); errCode != types.E_NONE {
		t.Fatalf("SetObjectName: %s", errCode)
	}
	if tx.HasWrites() {
		t.Fatal("direct transaction staged a write")
	}

	view, ok := s.Get(0)
	if !ok {
		t.Fatal("object disappeared after direct write")
	}
	if got := view.Name; got != "direct" {
		t.Fatalf("live object name = %q, want direct", got)
	}
	if errCode := tx.Commit(); errCode != types.E_NONE {
		t.Fatalf("direct Commit: %s", errCode)
	}
}

func TestStoreDoesNotMirrorTransactionRuntimeSurface(t *testing.T) {
	storeType := reflect.TypeOf((*Store)(nil))
	txnType := reflect.TypeOf((*StoreTxn)(nil))

	mirrored := []string{
		"Ancestors", "Children", "ClearPropertyOverride", "Contents", "CreateObject",
		"DefineProperty", "DefinedPropertyNames", "DefinedPropertyNamesInAncestry",
		"DeleteDefinedProperty", "DeleteResolvedVerb", "Descendants",
		"ExpandAnonymousReachability", "FindCallableVerb", "FindParentVerb", "FindProperty",
		"FindVerb", "FindVerbOnObject", "HasAncestor", "HasChparentDescendantPropertyConflict",
		"HasContentDescendant", "HasDefinedPropertyConflictWithAncestry",
		"HasDefinedPropertyInDescendants", "HasDuplicateDefinedPropertyAmong", "HasObjectFlag",
		"IsRecycled", "LastMove", "LocalProperty", "Location", "MaxObject", "MoveObject",
		"ObjectByteEstimate", "ObjectExists", "ObjectFlags", "ObjectIsAnonymous", "ObjectName",
		"ObjectOwner", "Parent", "Parents", "PropertyClearState", "PropertyValue",
		"PropertyValues", "ReadTimestamp", "ResolveVerbByIndex", "ResolveVerbOnObject",
		"SetObjectFlag", "SetObjectLocationRaw", "SetObjectName", "SetObjectOwner",
		"SetPropertyInfo", "SetPropertyValue", "SetVerbCode", "SetVerbCodeByIndex",
		"TruthyPropertiesWithPrefixInAncestry", "Valid", "VerbByIndex", "VerbNames",
	}

	for _, name := range mirrored {
		if _, ok := txnType.MethodByName(name); !ok {
			t.Errorf("StoreTxn is missing runtime method %s", name)
		}
		if _, ok := storeType.MethodByName(name); ok {
			t.Errorf("Store still mirrors StoreTxn.%s", name)
		}
	}
}
