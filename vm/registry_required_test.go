package vm

import (
	"strings"
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

func TestLimitOperationsRequireRegistry(t *testing.T) {
	tests := []struct {
		name string
		run  func()
	}{
		{
			name: "VM opcode",
			run: func() {
				machine := NewVM(nil, nil)
				machine.Push(types.NewStr("left"))
				machine.Push(types.NewStr("right"))
				_ = machine.executeAdd()
			},
		},
		{
			name: "indexed assignment helper",
			run: func() {
				_, _ = setAtIndex(
					nil,
					nil,
					types.NewList([]types.Value{types.NewInt(1)}),
					types.NewInt(1),
					types.NewInt(2),
				)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("limit operation accepted a nil registry")
				}
			}()
			test.run()
		})
	}
}

func TestLimitOperationsUseProvidedRegistry(t *testing.T) {
	store := dbstore.NewStore()
	for _, object := range []struct {
		id    types.ObjID
		value *dbstore.Object
	}{
		{id: 0, value: dbstore.NewObject(0, 0)},
		{id: 1, value: dbstore.NewObject(1, 0)},
	} {
		if err := store.Add(object.value); err != nil {
			t.Fatalf("store.Add(%d): %v", object.id, err)
		}
	}
	if err := store.DirectTxn().DefineProperty(0, "server_options", dbstore.NewProperty(types.NewObj(1), 0, dbstore.PropRead, false, true)); err != types.E_NONE {
		t.Fatalf("DefineProperty(server_options): %v", err)
	}
	for _, name := range []string{"max_string_concat", "max_list_value_bytes", "max_map_value_bytes"} {
		if err := store.DirectTxn().DefineProperty(1, name, dbstore.NewProperty(types.NewInt(1021), 0, dbstore.PropRead, false, true)); err != types.E_NONE {
			t.Fatalf("DefineProperty(%s): %v", name, err)
		}
	}

	registry := BuildVMRegistry()
	session := newTestSession(registry)
	if loaded := session.LoadServerOptionsFromStore(store); loaded != 3 {
		t.Fatalf("LoadServerOptionsFromStore loaded %d options, want 3", loaded)
	}

	longValue := strings.Repeat("x", 1500)
	tests := []struct {
		name string
		code string
	}{
		{name: "string concat", code: `return "` + longValue[:750] + `" + "` + longValue[750:] + `";`},
		{name: "list construction", code: `return {"` + longValue + `"};`},
		{name: "map construction", code: `return ["key" -> "` + longValue + `"];`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			program, diagnostics := registry.Compiler().CompileMOO([]string{test.code})
			if len(diagnostics) > 0 {
				t.Fatalf("CompileMOO: %v", diagnostics)
			}
			result := NewVM(store, session).Run(program)
			if result.Flow != types.FlowException || result.Error != types.E_QUOTA {
				t.Fatalf("result = flow %v error %v value %v, want E_QUOTA", result.Flow, result.Error, result.Val)
			}
		})
	}
}
