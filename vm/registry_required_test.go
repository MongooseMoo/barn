package vm

import (
	"testing"

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
