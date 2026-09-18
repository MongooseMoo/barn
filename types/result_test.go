package types

import (
	"reflect"
	"testing"
)

func TestResultCallStackHasStaticActivationFrameType(t *testing.T) {
	field, ok := reflect.TypeOf(Result{}).FieldByName("CallStack")
	if !ok {
		t.Fatal("Result.CallStack field is missing")
	}
	if field.Type.Kind() != reflect.Slice {
		t.Fatalf("Result.CallStack type = %v, want []types.ActivationFrame", field.Type)
	}
	if got := field.Type.Elem(); got.PkgPath() != "github.com/MongooseMoo/barn/types" || got.Name() != "ActivationFrame" {
		t.Fatalf("Result.CallStack element type = %v, want types.ActivationFrame", got)
	}
}

func TestResultConstructors(t *testing.T) {
	t.Run("Ok", func(t *testing.T) {
		r := Ok(NewInt(42))
		if !r.IsNormal() {
			t.Error("Ok() should create normal result")
		}
		if !r.Val.Equal(NewInt(42)) {
			t.Errorf("Expected value 42, got %v", r.Val)
		}
	})

	t.Run("Err", func(t *testing.T) {
		r := Err(E_TYPE)
		if !r.IsError() {
			t.Error("Err() should create error result")
		}
		if r.Error != E_TYPE {
			t.Errorf("Expected E_TYPE, got %v", r.Error)
		}
	})

	t.Run("Return", func(t *testing.T) {
		r := Return(NewInt(42))
		if !r.IsReturn() {
			t.Error("Return() should create return result")
		}
		if !r.Val.Equal(NewInt(42)) {
			t.Errorf("Expected value 42, got %v", r.Val)
		}
	})

	t.Run("Break", func(t *testing.T) {
		r := Break("", None)
		if !r.IsBreak() {
			t.Error("Break() should create break result")
		}
	})

	t.Run("Continue", func(t *testing.T) {
		r := Continue("")
		if !r.IsContinue() {
			t.Error("Continue() should create continue result")
		}
	})
}

func TestResultPredicates(t *testing.T) {
	tests := []struct {
		name       string
		result     Result
		isNormal   bool
		isError    bool
		isReturn   bool
		isBreak    bool
		isContinue bool
	}{
		{"normal", Ok(NewInt(42)), true, false, false, false, false},
		{"error", Err(E_TYPE), false, true, false, false, false},
		{"return", Return(NewInt(42)), false, false, true, false, false},
		{"break", Break("", None), false, false, false, true, false},
		{"continue", Continue(""), false, false, false, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.result.IsNormal() != tt.isNormal {
				t.Errorf("IsNormal() = %v, want %v", tt.result.IsNormal(), tt.isNormal)
			}
			if tt.result.IsError() != tt.isError {
				t.Errorf("IsError() = %v, want %v", tt.result.IsError(), tt.isError)
			}
			if tt.result.IsReturn() != tt.isReturn {
				t.Errorf("IsReturn() = %v, want %v", tt.result.IsReturn(), tt.isReturn)
			}
			if tt.result.IsBreak() != tt.isBreak {
				t.Errorf("IsBreak() = %v, want %v", tt.result.IsBreak(), tt.isBreak)
			}
			if tt.result.IsContinue() != tt.isContinue {
				t.Errorf("IsContinue() = %v, want %v", tt.result.IsContinue(), tt.isContinue)
			}
		})
	}
}
