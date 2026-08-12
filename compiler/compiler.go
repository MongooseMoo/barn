// Package compiler owns complete MOO source compilation.
package compiler

import (
	"errors"
	"fmt"
	"strings"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/parser"
	"github.com/MongooseMoo/barn/sourcekey"
	"github.com/MongooseMoo/barn/verb"
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

// Compiler owns one immutable builtin-ID view and its bounded source cache.
type Compiler struct {
	builtinIDs map[string]int
	cache      *programCache
}

// New constructs a compiler for one builtin registry layout.
func New(builtinIDs map[string]int) *Compiler {
	snapshot := make(map[string]int, len(builtinIDs))
	for name, id := range builtinIDs {
		snapshot[name] = id
	}
	return &Compiler{
		builtinIDs: snapshot,
		cache:      newProgramCache(mooCacheCapacity),
	}
}

// CompileMOO parses, lowers, source-attaches, and caches one MOO verb body.
// It hashes the full source to find the cache entry; callers on the verb-call
// hot path should hold a precomputed key and use CompileMOOWithKey instead.
func (c *Compiler) CompileMOO(sourceLines []string) (*bytecode.Program, []Diagnostic) {
	return c.CompileMOOWithKey(sourceLines, sourcekey.Of(sourceLines))
}

// CompileMOOWithKey is CompileMOO for callers that already hold the content key
// of these exact source lines (db/store carries one on every verb, refreshed by
// every code write). An unset key is computed here, so callers with source that
// has no stored key — eval strings, sources built from MOO values — may pass the
// zero Key.
//
// The key MUST be the key of sourceLines: it is the sole cache identity, so a
// key that describes different source serves that other source's program.
func (c *Compiler) CompileMOOWithKey(sourceLines []string, key sourcekey.Key) (*bytecode.Program, []Diagnostic) {
	if !key.IsSet() {
		key = sourcekey.Of(sourceLines)
	}
	if program, ok := c.cache.get(key); ok {
		return program, nil
	}

	program, err := parser.NewParser(strings.Join(sourceLines, "\n")).ParseProgram()
	if err != nil {
		return nil, []Diagnostic{syntaxDiagnostic(err)}
	}

	compiled, err := newLowerer(c.builtinIDs).compileProgram(program)
	if err != nil {
		return nil, []Diagnostic{compileDiagnostic(err)}
	}
	compiled.Source = append([]string(nil), sourceLines...)
	c.cache.put(key, compiled)
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
	var unknownBuiltin *UnknownBuiltinError
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
