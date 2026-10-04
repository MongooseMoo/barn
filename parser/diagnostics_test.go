package parser

import (
	"errors"
	"testing"

	"github.com/MongooseMoo/barn/verb"
)

func TestProgramDiagnosticsNeverExposePartialProgram(t *testing.T) {
	source := "break;\ncontinue;\na = ;\nreturn 1;"
	program, diagnostics := NewParser(source).ParseProgramWithDiagnostics(nil)
	if program != nil || len(diagnostics) != 3 {
		t.Fatalf("partial program %v / diagnostics %v", program, diagnostics)
	}
	for i, diagnostic := range diagnostics {
		if diagnostic.Position.Line != i+1 || diagnostic.Position.Column == 0 || diagnostic.Position.Offset == 0 {
			t.Errorf("missing full diagnostic position: %+v", diagnostic)
		}
	}
	program, err := NewParser(source).ParseProgram()
	var first *ParseError
	if program != nil || !errors.As(err, &first) || first.Position != diagnostics[0].Position {
		t.Fatalf("convenience API lost first cause: %v / %v", program, err)
	}
}

func TestSemanticDiagnosticAtFinalTokenHasNoPhantomEOFLine(t *testing.T) {
	_, diagnostics := NewParser("1 = 2;\n").ParseProgramWithDiagnostics(nil)
	if len(diagnostics) != 1 || diagnostics[0].Position != (verb.Position{Line: 1, Column: 5, Offset: 4}) {
		t.Fatalf("diagnostic must point at completed RHS: %+v", diagnostics)
	}
}
