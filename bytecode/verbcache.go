package bytecode

import (
	"errors"
	"fmt"
	"strings"

	"barn/parser"
)

// VerbProgram holds a verb's compiled AST. Relocated out of db/store so the
// world model (db/store) no longer imports barn/parser.
type VerbProgram struct {
	Statements []parser.Stmt // Compiled AST statements
}

// CompileVerb parses verb source lines into an AST (VerbProgram).
// Returns the program or a list of parse-error strings. This is the parser
// bridge that used to live in db/store; relocating it here is what lets
// db/store drop its barn/parser dependency.
func CompileVerb(code []string) (*VerbProgram, []string) {
	if len(code) == 0 {
		return &VerbProgram{Statements: []parser.Stmt{}}, nil
	}

	source := strings.Join(code, "\n")

	p := parser.NewParser(source)
	statements, err := p.ParseProgram()
	if err != nil {
		return nil, []string{formatParseError(err)}
	}

	return &VerbProgram{Statements: statements}, nil
}

// formatParseError renders a parser error in ToastStunt's "Line N:  <msg>" form
// (the word "Line", a space, the line number, a colon, TWO spaces, the message).
// Toast surfaces parse errors as the generic "Line N:  syntax error"; we follow
// suit using the line carried by *parser.ParseError.
func formatParseError(err error) string {
	var pe *parser.ParseError
	if errors.As(err, &pe) {
		return fmt.Sprintf("Line %d:  %s", pe.Line, pe.Msg)
	}
	// Fallback for non-positional parser errors: still use Toast's generic text.
	return fmt.Sprintf("Line 1:  %s", err)
}
