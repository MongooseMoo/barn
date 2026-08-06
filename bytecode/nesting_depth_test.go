package bytecode_test

import (
	"errors"
	"strconv"
	"testing"

	"barn/bytecode"
	"barn/verb"
)

func nestedUnaryExpr(depth int) verb.Expr {
	return nestedUnaryExprAt(depth, 7)
}

func nestedUnaryExprAt(depth, line int) verb.Expr {
	var expr verb.Expr = &verb.LiteralExpr{Pos: verb.Position{Line: line}, Kind: verb.LiteralInt, IntValue: 1}
	for i := 1; i < depth; i++ {
		expr = &verb.UnaryExpr{Pos: verb.Position{Line: line}, Operator: verb.UnaryNegate, Operand: expr}
	}
	return expr
}

func TestCompileDirectIRNestingBoundary(t *testing.T) {
	for _, depth := range []int{256, 257, 2560} {
		t.Run(strconv.Itoa(depth), func(t *testing.T) {
			program, err := compileNodeWithoutPanic(t, nestedUnaryExpr(depth))
			if depth == 256 {
				if err != nil || program == nil {
					t.Fatalf("Compile(depth 256) = (%v, %v), want program", program, err)
				}
				return
			}
			if program != nil {
				t.Fatalf("Compile(depth %d) returned a partial program", depth)
			}
			var depthErr *verb.NestingDepthError
			if !errors.As(err, &depthErr) {
				t.Fatalf("Compile(depth %d) error = %T %v, want *verb.NestingDepthError", depth, err, err)
			}
		})
	}
}

func TestCompileProgramDirectIRNestingBoundary(t *testing.T) {
	for _, depth := range []int{255, 256, 257, 2560} {
		t.Run(strconv.Itoa(depth), func(t *testing.T) {
			semantic := &verb.Program{Statements: []verb.Stmt{&verb.ReturnStmt{
				Pos:   verb.Position{Line: 11},
				Value: nestedUnaryExprAt(depth-1, 11),
			}}}
			program, err := compileProgramWithoutPanic(t, semantic)
			if depth <= 256 {
				if err != nil || program == nil {
					t.Fatalf("CompileProgram(depth %d) = (%v, %v), want program", depth, program, err)
				}
				return
			}
			if program != nil {
				t.Fatalf("CompileProgram(depth %d) returned a partial program", depth)
			}
			var depthErr *verb.NestingDepthError
			if !errors.As(err, &depthErr) {
				t.Fatalf("CompileProgram(depth %d) error = %T %v, want *verb.NestingDepthError", depth, err, err)
			}
			if depthErr.Position.Line != 11 {
				t.Fatalf("CompileProgram(depth %d) error line = %d, want 11", depth, depthErr.Position.Line)
			}
		})
	}
}

func compileNodeWithoutPanic(t *testing.T, node verb.Node) (program *bytecode.Program, err error) {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("Compile() panicked: %v", recovered)
		}
	}()
	return bytecode.NewCompiler().Compile(node)
}

func compileProgramWithoutPanic(t *testing.T, semantic *verb.Program) (program *bytecode.Program, err error) {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("CompileProgram() panicked: %v", recovered)
		}
	}()
	return bytecode.NewCompiler().CompileProgram(semantic)
}
