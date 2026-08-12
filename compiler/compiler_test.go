package compiler

import "testing"

func TestCompileMOOOwnsCompleteSourceCompilation(t *testing.T) {
	source := []string{"x = 1;", "return x;"}
	program, diagnostics := New(nil).CompileMOO(source)
	if len(diagnostics) != 0 {
		t.Fatalf("CompileMOO() diagnostics = %v", diagnostics)
	}
	if len(program.Code) == 0 {
		t.Fatal("CompileMOO() returned empty bytecode")
	}
	if len(program.Source) != len(source) || program.Source[0] != source[0] {
		t.Fatalf("program source = %v, want %v", program.Source, source)
	}

	source[0] = "changed after compilation"
	if program.Source[0] != "x = 1;" {
		t.Fatalf("compiled source aliases caller input: %q", program.Source[0])
	}
}

func TestCompileMOOCachesBySourceContent(t *testing.T) {
	source := []string{"return 987654321;"}
	compiler := New(nil)
	first, diagnostics := compiler.CompileMOO(source)
	if len(diagnostics) != 0 {
		t.Fatalf("first CompileMOO() diagnostics = %v", diagnostics)
	}
	second, diagnostics := compiler.CompileMOO(append([]string(nil), source...))
	if len(diagnostics) != 0 {
		t.Fatalf("second CompileMOO() diagnostics = %v", diagnostics)
	}
	if first != second {
		t.Fatal("identical source did not return the cached program")
	}
}

func TestCompileMOOReturnsStructuredDiagnostics(t *testing.T) {
	compiler := New(nil)
	_, diagnostics := compiler.CompileMOO([]string{"return 1;", "if ("})
	if len(diagnostics) != 1 {
		t.Fatalf("syntax diagnostic count = %d, want 1", len(diagnostics))
	}
	if diagnostics[0].Position.Line != 3 || diagnostics[0].Message != "syntax error" || diagnostics[0].Detail == nil {
		t.Fatalf("syntax diagnostic = %+v", diagnostics[0])
	}

	_, diagnostics = compiler.CompileMOO([]string{"return missing_builtin();"})
	if len(diagnostics) != 1 {
		t.Fatalf("compile diagnostic count = %d, want 1", len(diagnostics))
	}
	if diagnostics[0].Position.Line != 1 || diagnostics[0].Message != "Unknown built-in function: missing_builtin" {
		t.Fatalf("compile diagnostic = %+v", diagnostics[0])
	}
}
