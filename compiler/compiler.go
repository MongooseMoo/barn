// Package compiler owns complete MOO source compilation.
package compiler

import (
	"errors"
	"fmt"
	"strings"

	"barn/bytecode"
	"barn/parser"
	"barn/verb"
)

// Diagnostic is a source-located syntax or bytecode compilation failure.
type Diagnostic struct {
	Stage    DiagnosticStage
	Position verb.Position
	Message  string
	Detail   error
}

type DiagnosticStage uint8

const (
	SyntaxStage DiagnosticStage = iota
	BytecodeStage
)

func (d Diagnostic) Error() string {
	line := d.Position.Line
	if line < 1 {
		line = 1
	}
	return fmt.Sprintf("Line %d:  %s", line, d.Message)
}

// CompileMOO parses, lowers, source-attaches, and caches one MOO verb body.
func CompileMOO(sourceLines []string, registry bytecode.Registry) (*bytecode.Program, []Diagnostic) {
	key := sourceKey(sourceLines)
	if program, ok := mooProgramCache.get(key); ok {
		return program, nil
	}

	program, err := parser.NewParser(strings.Join(sourceLines, "\n")).ParseProgram()
	if err != nil {
		return nil, []Diagnostic{syntaxDiagnostic(err)}
	}

	compiled, err := bytecode.NewCompilerWithRegistry(registry).CompileProgram(program)
	if err != nil {
		return nil, []Diagnostic{compileDiagnostic(err)}
	}
	compiled.Source = append([]string(nil), sourceLines...)
	mooProgramCache.put(key, compiled)
	return compiled, nil
}

func syntaxDiagnostic(err error) Diagnostic {
	var parseError *parser.ParseError
	if errors.As(err, &parseError) {
		return Diagnostic{
			Stage:    SyntaxStage,
			Position: verb.Position{Line: parseError.Line},
			Message:  parseError.Msg,
			Detail:   parseError.Detail,
		}
	}
	return Diagnostic{Stage: SyntaxStage, Position: verb.Position{Line: 1}, Message: err.Error(), Detail: err}
}

func compileDiagnostic(err error) Diagnostic {
	var unknownBuiltin *bytecode.UnknownBuiltinError
	if errors.As(err, &unknownBuiltin) {
		return Diagnostic{
			Stage:    BytecodeStage,
			Position: verb.Position{Line: unknownBuiltin.Line},
			Message:  "Unknown built-in function: " + unknownBuiltin.Name,
			Detail:   err,
		}
	}
	return Diagnostic{Stage: BytecodeStage, Position: verb.Position{Line: 1}, Message: err.Error(), Detail: err}
}
