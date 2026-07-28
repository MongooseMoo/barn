// Package compiler owns complete MOO source compilation.
package compiler

import (
	"errors"
	"fmt"
	"strings"

	"barn/bytecode"
	"barn/parser"
	"barn/sourcekey"
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
// It hashes the full source to find the cache entry; callers on the verb-call
// hot path should hold a precomputed key and use CompileMOOWithKey instead.
func CompileMOO(sourceLines []string, registry bytecode.Registry) (*bytecode.Program, []Diagnostic) {
	return CompileMOOWithKey(sourceLines, sourcekey.Of(sourceLines), registry)
}

// CompileMOOWithKey is CompileMOO for callers that already hold the content key
// of these exact source lines (db/store carries one on every verb, refreshed by
// every code write). An unset key is computed here, so callers with source that
// has no stored key — eval strings, sources built from MOO values — may pass the
// zero Key.
//
// The key MUST be the key of sourceLines: it is the sole cache identity, so a
// key that describes different source serves that other source's program.
func CompileMOOWithKey(sourceLines []string, key sourcekey.Key, registry bytecode.Registry) (*bytecode.Program, []Diagnostic) {
	if !key.IsSet() {
		key = sourcekey.Of(sourceLines)
	}
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
