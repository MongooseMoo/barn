package server

import (
	"reflect"
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/engine"
)

func TestInteractiveProgrammingKeepsOrderedDiagnosticsAndOldBody(t *testing.T) {
	store := dbstore.NewStore()
	object := addTestObject(t, store, 3, 0)
	addTestVerb(store, object, "probe", "return 99;")
	runtime := engine.NewRuntime(store)
	t.Cleanup(runtime.Stop)
	processor := NewInputProcessor(store, runtime)
	transport := newRecordingTransport("programming diagnostics")
	connection := NewConnection(1, transport)
	t.Cleanup(connection.cancel)
	connection.programming = &programmingMode{Target: object, Verb: "probe"}
	for _, line := range []string{"break;", "continue;", "."} {
		if !processor.processProgrammingInput(connection, line) {
			t.Fatal("programming input was not consumed")
		}
	}
	want := []string{"Line 1:  No enclosing loop for `break' statement", "Line 2:  No enclosing loop for `continue' statement"}
	if got := transport.writtenLines(); !reflect.DeepEqual(got, want) {
		t.Fatalf("output = %q, want %q", got, want)
	}
	view, _, err := store.DirectTxn().FindVerb(object, "probe")
	if err != nil || !reflect.DeepEqual(view.Code, []string{"return 99;"}) || connection.programming != nil {
		t.Fatalf("failed programming changed body or retained mode: %+v, %v", view, err)
	}
	connection.programming = &programmingMode{Target: object, Verb: "probe", Lines: []string{"return 7;"}}
	processor.processProgrammingInput(connection, ".")
	view, _, err = store.DirectTxn().FindVerb(object, "probe")
	if err != nil || !reflect.DeepEqual(view.Code, []string{"return 7;"}) {
		t.Fatalf("valid retry was not stored: %+v, %v", view, err)
	}
}
