package db

import (
	"barn/parser"
	"fmt"
	"strings"
)

// CompileVerb compiles verb source code into an AST
// Returns compiled program or compile errors
func CompileVerb(code []string) (*VerbProgram, []string) {
	if len(code) == 0 {
		return &VerbProgram{Statements: []parser.Stmt{}}, nil
	}

	// Join lines into a single string for parsing
	source := strings.Join(code, "\n")

	// Parse the code
	p := parser.NewParser(source)
	statements, err := p.ParseProgram()
	if err != nil {
		return nil, []string{fmt.Sprintf("parse error: %v", err)}
	}

	return &VerbProgram{Statements: statements}, nil
}
