package compiler

import (
	"errors"
	"strings"
	"testing"

	"barn/bytecode"
	"barn/sourcekey"
	"barn/verb"
)

func compilerUnarySource(depth int) []string {
	return []string{"return 0;", "return " + strings.Repeat("-", depth-2) + "1;"}
}

func TestCompileMOONestingFailureIsOneSyntaxDiagnostic(t *testing.T) {
	for _, depth := range []int{257, 2560} {
		source := compilerUnarySource(depth)
		key := sourcekey.Of(source)
		if _, ok := mooProgramCache.get(key); ok {
			t.Fatalf("test source for depth %d unexpectedly already cached", depth)
		}

		program, diagnostics := compileMOOWithKeyWithoutPanic(t, source, key)
		if program != nil {
			t.Fatalf("CompileMOOWithKey(depth %d) returned partial program", depth)
		}
		if len(diagnostics) != 1 {
			t.Fatalf("CompileMOOWithKey(depth %d) diagnostic count = %d, want 1", depth, len(diagnostics))
		}
		diagnostic := diagnostics[0]
		if diagnostic.Stage != SyntaxStage || diagnostic.Message != "syntax error" || diagnostic.Position.Line != 2 {
			t.Fatalf("CompileMOOWithKey(depth %d) diagnostic = %+v, want line 2 SyntaxStage syntax error", depth, diagnostic)
		}
		var depthErr *verb.NestingDepthError
		if !errors.As(diagnostic.Detail, &depthErr) {
			t.Fatalf("CompileMOOWithKey(depth %d) detail = %T %v, want *verb.NestingDepthError", depth, diagnostic.Detail, diagnostic.Detail)
		}
		if got, want := depthErr.Error(), "maximum nesting depth exceeded (max 256)"; got != want {
			t.Fatalf("CompileMOOWithKey(depth %d) detail = %q, want %q", depth, got, want)
		}
		if _, ok := mooProgramCache.get(key); ok {
			t.Fatalf("CompileMOOWithKey(depth %d) inserted rejected program into cache", depth)
		}
	}
}

func compileMOOWithKeyWithoutPanic(t *testing.T, source []string, key sourcekey.Key) (program *bytecode.Program, diagnostics []Diagnostic) {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("CompileMOOWithKey() panicked: %v", recovered)
		}
	}()
	return CompileMOOWithKey(source, key, testRegistry{})
}

func TestCompileMOONestingBoundarySucceeds(t *testing.T) {
	for _, depth := range []int{255, 256} {
		program, diagnostics := CompileMOO(compilerUnarySource(depth), testRegistry{})
		if len(diagnostics) != 0 || program == nil {
			t.Fatalf("CompileMOO(depth %d) = (%v, %v), want program", depth, program, diagnostics)
		}
	}
}

func TestCompileMOOCollectionNestingDetailIsCanonicalTypedError(t *testing.T) {
	tests := []struct {
		name string
		expr string
	}{
		{name: "list", expr: strings.Repeat("{", 255) + "1" + strings.Repeat("}", 255)},
		{name: "map", expr: strings.Repeat("[\"key\" -> ", 255) + "1" + strings.Repeat("]", 255)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := []string{"return 0;", "return " + test.expr + ";"}
			key := sourcekey.Of(source)
			if _, ok := mooProgramCache.get(key); ok {
				t.Fatal("test source unexpectedly already cached")
			}
			program, diagnostics := compileMOOWithKeyWithoutPanic(t, source, key)
			if program != nil {
				t.Fatal("CompileMOOWithKey() returned partial program")
			}
			if len(diagnostics) != 1 {
				t.Fatalf("diagnostic count = %d, want 1", len(diagnostics))
			}
			diagnostic := diagnostics[0]
			if diagnostic.Stage != SyntaxStage || diagnostic.Message != "syntax error" || diagnostic.Position.Line != 2 {
				t.Fatalf("diagnostic = %+v, want line 2 SyntaxStage syntax error", diagnostic)
			}
			depthErr, ok := diagnostic.Detail.(*verb.NestingDepthError)
			if !ok {
				t.Fatalf("Diagnostic.Detail = %T %v, want exactly *verb.NestingDepthError", diagnostic.Detail, diagnostic.Detail)
			}
			if got, want := depthErr.Error(), "maximum nesting depth exceeded (max 256)"; got != want {
				t.Fatalf("NestingDepthError.Error() = %q, want %q", got, want)
			}
			if depthErr.Position.Line != 2 {
				t.Fatalf("NestingDepthError.Position.Line = %d, want 2", depthErr.Position.Line)
			}
			if _, ok := mooProgramCache.get(key); ok {
				t.Fatal("rejected program inserted into cache")
			}
		})
	}
}
