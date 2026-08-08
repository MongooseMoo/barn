package parser

import (
	"github.com/MongooseMoo/barn/verb"
	"testing"
)

func TestParseExpressionStatementRequiresSemicolonAtEOF(t *testing.T) {
	p := NewParser(`delete_property(#0, "recycle_log")`)
	if _, err := p.ParseProgram(); err == nil {
		t.Fatal("ParseProgram() succeeded, want error for missing trailing semicolon")
	}
}

func TestParseExpressionStatementRequiresSeparatorBetweenStatements(t *testing.T) {
	p := NewParser(`delete_property(#0, "a") delete_property(#0, "b");`)
	if _, err := p.ParseProgram(); err == nil {
		t.Fatal("ParseProgram() succeeded, want error for missing separator")
	}
}

func TestElseIfLowersToNestedSemanticConditionals(t *testing.T) {
	program, err := NewParser("if (a)\nreturn 1;\nelseif (b)\nreturn 2;\nelseif (c)\nreturn 3;\nelse\nreturn 4;\nendif").ParseProgram()
	if err != nil {
		t.Fatalf("ParseProgram() error = %v", err)
	}

	root, ok := program.Statements[0].(*verb.IfStmt)
	if !ok {
		t.Fatalf("root = %T, want *verb.IfStmt", program.Statements[0])
	}
	second, ok := root.Else[0].(*verb.IfStmt)
	if !ok {
		t.Fatalf("first else = %T, want nested *verb.IfStmt", root.Else[0])
	}
	third, ok := second.Else[0].(*verb.IfStmt)
	if !ok {
		t.Fatalf("second else = %T, want nested *verb.IfStmt", second.Else[0])
	}
	if len(third.Else) != 1 {
		t.Fatalf("final else length = %d, want 1", len(third.Else))
	}
	if root.Position().Line != 1 || second.Position().Line != 3 || third.Position().Line != 5 {
		t.Fatalf("conditional lines = %d, %d, %d; want 1, 3, 5", root.Position().Line, second.Position().Line, third.Position().Line)
	}
}

func TestTryFormsLowerToOneSemanticStatement(t *testing.T) {
	tests := []struct {
		name          string
		source        string
		wantHandlers  int
		wantFinalizer bool
	}{
		{
			name:         "handlers only",
			source:       "try\nreturn 1;\nexcept detail (E_DIV)\nreturn detail;\nendtry",
			wantHandlers: 1,
		},
		{
			name:          "finalizer only",
			source:        "try\nreturn 1;\nfinally\nreturn 2;\nendtry",
			wantFinalizer: true,
		},
		{
			name:          "handlers and finalizer",
			source:        "try\nreturn 1;\nexcept (E_DIV)\nreturn 2;\nexcept (ANY)\nreturn 3;\nfinally\nreturn 4;\nendtry",
			wantHandlers:  2,
			wantFinalizer: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			program, err := NewParser(test.source).ParseProgram()
			if err != nil {
				t.Fatalf("ParseProgram() error = %v", err)
			}

			tryStmt, ok := program.Statements[0].(*verb.TryStmt)
			if !ok {
				t.Fatalf("statement = %T, want *verb.TryStmt", program.Statements[0])
			}
			if len(tryStmt.Handlers) != test.wantHandlers {
				t.Fatalf("handler count = %d, want %d", len(tryStmt.Handlers), test.wantHandlers)
			}
			if (tryStmt.Finalizer != nil) != test.wantFinalizer {
				t.Fatalf("finalizer present = %t, want %t", tryStmt.Finalizer != nil, test.wantFinalizer)
			}
		})
	}
}

func TestForFormsLowerToDistinctSemanticStatements(t *testing.T) {
	collectionProgram, err := NewParser("for items value, key in (source)\nreturn value;\nendfor").ParseProgram()
	if err != nil {
		t.Fatalf("collection ParseProgram() error = %v", err)
	}
	collection, ok := collectionProgram.Statements[0].(*verb.CollectionLoopStmt)
	if !ok {
		t.Fatalf("collection statement = %T, want *verb.CollectionLoopStmt", collectionProgram.Statements[0])
	}
	if collection.Label != "items" || collection.Value != "value" || collection.Index != "key" {
		t.Fatalf("collection bindings = (%q, %q, %q), want (items, value, key)", collection.Label, collection.Value, collection.Index)
	}

	rangeProgram, err := NewParser("for numbers value in [start..finish]\nreturn value;\nendfor").ParseProgram()
	if err != nil {
		t.Fatalf("range ParseProgram() error = %v", err)
	}
	rangeLoop, ok := rangeProgram.Statements[0].(*verb.RangeLoopStmt)
	if !ok {
		t.Fatalf("range statement = %T, want *verb.RangeLoopStmt", rangeProgram.Statements[0])
	}
	if rangeLoop.Label != "numbers" || rangeLoop.Value != "value" {
		t.Fatalf("range bindings = (%q, %q), want (numbers, value)", rangeLoop.Label, rangeLoop.Value)
	}
}

func TestRangeLoopRejectsIndexBindingLikeToast(t *testing.T) {
	_, err := NewParser("for value, index in [1..2]\nreturn value;\nendfor").ParseProgram()
	if err == nil {
		t.Fatal("ParseProgram() succeeded, want syntax error for range index binding")
	}
}

func TestAssignmentsLowerToSealedSemanticTargets(t *testing.T) {
	tests := []struct {
		name   string
		source string
		check  func(*testing.T, verb.Target)
	}{
		{"variable", "x = 1;", func(t *testing.T, target verb.Target) {
			if _, ok := target.(*verb.VariableTarget); !ok {
				t.Fatalf("target = %T, want *verb.VariableTarget", target)
			}
		}},
		{"property", "object.name = 1;", func(t *testing.T, target verb.Target) {
			if _, ok := target.(*verb.PropertyTarget); !ok {
				t.Fatalf("target = %T, want *verb.PropertyTarget", target)
			}
		}},
		{"index", "items[1] = 1;", func(t *testing.T, target verb.Target) {
			if _, ok := target.(*verb.IndexTarget); !ok {
				t.Fatalf("target = %T, want *verb.IndexTarget", target)
			}
		}},
		{"range", "items[1..2] = {};", func(t *testing.T, target verb.Target) {
			if _, ok := target.(*verb.RangeTarget); !ok {
				t.Fatalf("target = %T, want *verb.RangeTarget", target)
			}
		}},
		{"destructuring", "{a, ?b = 2, @rest} = {1};", func(t *testing.T, target verb.Target) {
			destructuring, ok := target.(*verb.DestructuringTarget)
			if !ok {
				t.Fatalf("target = %T, want *verb.DestructuringTarget", target)
			}
			if len(destructuring.Bindings) != 3 {
				t.Fatalf("binding count = %d, want 3", len(destructuring.Bindings))
			}
			if _, ok := destructuring.Bindings[0].(*verb.RequiredBinding); !ok {
				t.Fatalf("binding 0 = %T, want *verb.RequiredBinding", destructuring.Bindings[0])
			}
			if _, ok := destructuring.Bindings[1].(*verb.OptionalBinding); !ok {
				t.Fatalf("binding 1 = %T, want *verb.OptionalBinding", destructuring.Bindings[1])
			}
			if _, ok := destructuring.Bindings[2].(*verb.RestBinding); !ok {
				t.Fatalf("binding 2 = %T, want *verb.RestBinding", destructuring.Bindings[2])
			}
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			program, err := NewParser(test.source).ParseProgram()
			if err != nil {
				t.Fatalf("ParseProgram() error = %v", err)
			}
			statement := program.Statements[0].(*verb.ExprStmt)
			assignment := statement.Expr.(*verb.AssignExpr)
			test.check(t, assignment.Target)
		})
	}
}

func TestAssignmentRejectsNonTargetExpressionDuringParsing(t *testing.T) {
	if _, err := NewParser("1 = 2;").ParseProgram(); err == nil {
		t.Fatal("ParseProgram() succeeded, want invalid assignment target error")
	}
}

func TestDestructuringRejectsMultipleRestBindingsLikeToast(t *testing.T) {
	if _, err := NewParser("{@first, @second} = {1, 2};").ParseProgram(); err == nil {
		t.Fatal("ParseProgram() succeeded, want multiple rest binding error")
	}
}

func TestIndexMarkersLowerToSemanticBoundaries(t *testing.T) {
	program, err := NewParser("return items[^..$];").ParseProgram()
	if err != nil {
		t.Fatalf("ParseProgram() error = %v", err)
	}
	returnStmt := program.Statements[0].(*verb.ReturnStmt)
	rangeExpr := returnStmt.Value.(*verb.RangeExpr)
	first := rangeExpr.Start.(*verb.IndexBoundaryExpr)
	last := rangeExpr.End.(*verb.IndexBoundaryExpr)
	if first.Boundary != verb.IndexFirst || last.Boundary != verb.IndexLast {
		t.Fatalf("boundaries = (%v, %v), want (%v, %v)", first.Boundary, last.Boundary, verb.IndexFirst, verb.IndexLast)
	}
}
