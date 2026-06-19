package bytecode

import (
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
		return nil, []string{fmt.Sprintf("parse error: %v", err)}
	}

	return &VerbProgram{Statements: statements}, nil
}
