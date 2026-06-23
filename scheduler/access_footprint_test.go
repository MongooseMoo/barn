package scheduler

import (
	"testing"
	"time"

	"barn/task"
	"barn/types"
)

func TestAccessFootprintTracksStaticPropertyReadsAndWrites(t *testing.T) {
	footprint := analyzeAccessFootprint(parseTestStatements(t, `
x = #1.foo;
#2.bar = x;
return #3.Baz;
`), nil)

	assertPropertyRead(t, footprint, 1, "foo")
	assertPropertyRead(t, footprint, 3, "baz")
	assertPropertyWrite(t, footprint, 2, "bar")
	if footprint.unknown {
		t.Fatal("unknown = true, want false")
	}
}

func TestAccessFootprintTracksKnownTaskObjects(t *testing.T) {
	footprint := analyzeAccessFootprint(parseTestStatements(t, `
this.score = player.score;
`), map[string]types.ObjID{
		"this":   10,
		"player": 20,
	})

	assertPropertyWrite(t, footprint, 10, "score")
	assertPropertyRead(t, footprint, 20, "score")
	if footprint.unknown {
		t.Fatal("unknown = true, want false")
	}
}

func TestAccessFootprintMarksDynamicPropertyUnsafe(t *testing.T) {
	footprint := analyzeAccessFootprint(parseTestStatements(t, `
#1.(name) = 2;
return obj.foo;
`), nil)

	if !footprint.unknown {
		t.Fatal("unknown = false, want true")
	}
}

func TestAccessFootprintMarksIndexedPropertyMutationReadWrite(t *testing.T) {
	footprint := analyzeAccessFootprint(parseTestStatements(t, `
#1.items[2] = "x";
`), nil)

	assertPropertyRead(t, footprint, 1, "items")
	assertPropertyWrite(t, footprint, 1, "items")
	if footprint.unknown {
		t.Fatal("unknown = true, want false")
	}
}

func TestAccessFootprintIgnoresForkBodyForParentTask(t *testing.T) {
	footprint := analyzeAccessFootprint(parseTestStatements(t, `
fork (0)
  #1.later = 1;
endfork
return 0;
`), nil)

	assertNoPropertyWrite(t, footprint, 1, "later")
	if footprint.unknown {
		t.Fatal("unknown = true, want false")
	}
}

func TestAccessFootprintTracksPropertyBuiltins(t *testing.T) {
	footprint := analyzeAccessFootprint(parseTestStatements(t, `
set_property_info(#1, "foo", {#0, "rw"});
return property_info(#2, "bar");
`), nil)

	assertPropertyWrite(t, footprint, 1, "foo")
	assertPropertyRead(t, footprint, 2, "bar")
	if footprint.unknown {
		t.Fatal("unknown = true, want false")
	}
}

func TestAnalyzeTaskAccessFootprintResolvesCommandObjects(t *testing.T) {
	ticks, seconds := foregroundTaskLimits()
	queued := task.NewTaskFull(1201, 10, parseTestStatements(t, `
this.audit = player.source;
dobj.target = iobj.source;
`), ticks, seconds)
	queued.StartTime = time.Now()
	queued.Programmer = 11
	queued.This = 20
	queued.Caller = 21
	queued.Dobj = 22
	queued.Iobj = 23

	footprint := analyzeTaskAccessFootprint(queued)

	assertPropertyWrite(t, footprint, 20, "audit")
	assertPropertyRead(t, footprint, 10, "source")
	assertPropertyWrite(t, footprint, 22, "target")
	assertPropertyRead(t, footprint, 23, "source")
	if footprint.unknown {
		t.Fatal("unknown = true, want false")
	}
}

func TestAnalyzeTaskAccessFootprintMarksSavedVMUnknown(t *testing.T) {
	footprint := analyzeTaskAccessFootprint(&task.Task{BytecodeVM: struct{}{}})

	if !footprint.unknown {
		t.Fatal("unknown = false, want true")
	}
}

func TestAccessFootprintsCommuteForDisjointWrites(t *testing.T) {
	left := analyzeAccessFootprint(parseTestStatements(t, `#1.a = 2;`), nil)
	right := analyzeAccessFootprint(parseTestStatements(t, `#1.b = 3;`), nil)

	if !accessFootprintsCommute(left, right) {
		t.Fatal("disjoint property writes did not commute")
	}
}

func TestAccessFootprintsDoNotCommuteForSamePropertyWrites(t *testing.T) {
	left := analyzeAccessFootprint(parseTestStatements(t, `#1.a = 2;`), nil)
	right := analyzeAccessFootprint(parseTestStatements(t, `#1.A = 3;`), nil)

	if accessFootprintsCommute(left, right) {
		t.Fatal("same property writes commuted")
	}
}

func TestAccessFootprintsDoNotCommuteForReadWrite(t *testing.T) {
	left := analyzeAccessFootprint(parseTestStatements(t, `return #1.a;`), nil)
	right := analyzeAccessFootprint(parseTestStatements(t, `#1.a = 3;`), nil)

	if accessFootprintsCommute(left, right) {
		t.Fatal("read/write conflict commuted")
	}
}

func TestAccessFootprintsDoNotCommuteWhenUnknown(t *testing.T) {
	left := analyzeAccessFootprint(parseTestStatements(t, `return obj.a;`), nil)
	right := analyzeAccessFootprint(parseTestStatements(t, `#1.b = 3;`), nil)

	if accessFootprintsCommute(left, right) {
		t.Fatal("unknown footprint commuted")
	}
}

func assertPropertyRead(t *testing.T, footprint accessFootprint, obj types.ObjID, name string) {
	t.Helper()
	access := propertyAccess{obj: obj, name: canonicalPropertyName(name)}
	if _, ok := footprint.propertyReads[access]; !ok {
		t.Fatalf("missing property read #%d.%s in %#v", obj, name, footprint.propertyReads)
	}
}

func assertPropertyWrite(t *testing.T, footprint accessFootprint, obj types.ObjID, name string) {
	t.Helper()
	access := propertyAccess{obj: obj, name: canonicalPropertyName(name)}
	if _, ok := footprint.propertyWrites[access]; !ok {
		t.Fatalf("missing property write #%d.%s in %#v", obj, name, footprint.propertyWrites)
	}
}

func assertNoPropertyWrite(t *testing.T, footprint accessFootprint, obj types.ObjID, name string) {
	t.Helper()
	access := propertyAccess{obj: obj, name: canonicalPropertyName(name)}
	if _, ok := footprint.propertyWrites[access]; ok {
		t.Fatalf("unexpected property write #%d.%s in %#v", obj, name, footprint.propertyWrites)
	}
}
