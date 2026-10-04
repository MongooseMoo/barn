package parser

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/MongooseMoo/barn/verb"
)

// ParseError retains the complete offending source position. Msg remains the
// line-only MOO-facing diagnostic text; Detail preserves its specific cause.
type ParseError struct {
	Position verb.Position
	Msg      string
	Detail   error
}

func (e *ParseError) Error() string { return e.Msg }
func (e *ParseError) Unwrap() error { return e.Detail }

// ParseProgram returns no program if any diagnostic exists. errors.As/Is can
// inspect individual causes; ParseProgramWithDiagnostics exposes the full list.
func (p *Parser) ParseProgram() (*verb.Program, error) {
	program, diagnostics := p.ParseProgramWithDiagnostics(nil)
	if len(diagnostics) == 0 {
		return program, nil
	}
	if len(diagnostics) == 1 {
		return nil, diagnostics[0]
	}
	errs := make([]error, len(diagnostics))
	for i, diagnostic := range diagnostics {
		errs[i] = diagnostic
	}
	return nil, errors.Join(errs...)
}

// ParseProgramWithDiagnostics continues only through syntactically complete
// constructs with independent validation errors. It never skips tokens to find
// another statement after a syntax error. checkBuiltin is compiler-owned name
// admission, called at the closing token of a complete builtin call; the
// parser neither owns registry IDs nor implements builtin behavior.
func (p *Parser) ParseProgramWithDiagnostics(checkBuiltin func(string, verb.Position) error) (*verb.Program, []*ParseError) {
	p.collecting = true
	p.checkBuiltin = checkBuiltin
	defer func() { p.collecting = false; p.checkBuiltin = nil }()
	var statements []verb.Stmt
	for p.current.Type != TOKEN_EOF {
		stmt, err := p.parseStatement()
		if p.lexer.lexicalError != nil {
			p.diagnostics = append(p.diagnostics, p.lexer.lexicalError)
			return nil, p.orderedDiagnostics()
		}
		if err != nil {
			var parseError *ParseError
			if errors.As(err, &parseError) {
				p.diagnostics = append(p.diagnostics, parseError)
			} else {
				p.diagnostics = append(p.diagnostics, &ParseError{Position: p.current.Position, Msg: "syntax error", Detail: err})
			}
			return nil, p.orderedDiagnostics()
		}
		statements = append(statements, stmt)
	}
	if p.lexer.lexicalError != nil {
		p.diagnostics = append(p.diagnostics, p.lexer.lexicalError)
	}
	if len(p.diagnostics) != 0 {
		return nil, p.orderedDiagnostics()
	}
	program := &verb.Program{Statements: statements}
	if err := verb.ValidateNesting(program); err != nil {
		return nil, []*ParseError{{Position: p.current.Position, Msg: "syntax error", Detail: err}}
	}
	return program, nil
}

func (p *Parser) orderedDiagnostics() []*ParseError {
	sort.SliceStable(p.diagnostics, func(i, j int) bool { return p.diagnostics[i].Position.Offset < p.diagnostics[j].Position.Offset })
	return p.diagnostics
}

func (p *Parser) report(pos verb.Position, message string, cause error) error {
	diagnostic := &ParseError{Position: pos, Msg: message, Detail: cause}
	if !p.collecting {
		return diagnostic
	}
	p.diagnostics = append(p.diagnostics, diagnostic)
	return nil
}

type sourceLoop struct{ names []string }

// Convert the entire expression prefix before checking counts. An invalid
// variable discards the prefix, so it cannot produce secondary count errors.
func (p *Parser) scatterBindings(elements []verb.Expr) ([]verb.Binding, error) {
	bindings := make([]verb.Binding, 0, len(elements))
	for _, element := range elements {
		value := element
		splice, rest := element.(*verb.SpliceExpr)
		if rest {
			value = splice.Expr
		}
		identifier, ok := value.(*verb.IdentifierExpr)
		if !ok {
			return nil, p.report(p.previous.Position, "Scattering assignment targets must be simple variables.", fmt.Errorf("invalid assignment target: %T", element))
		}
		if rest {
			bindings = append(bindings, &verb.RestBinding{Pos: element.Position(), Name: identifier.Name})
		} else {
			bindings = append(bindings, &verb.RequiredBinding{Pos: identifier.Pos, Name: identifier.Name})
		}
	}
	return bindings, nil
}

func (p *Parser) validateScatterBindings(bindings []verb.Binding) error {
	restSeen := false
	for _, binding := range bindings {
		if _, rest := binding.(*verb.RestBinding); rest {
			if restSeen {
				if err := p.report(p.previous.Position, "More than one `@' target in scattering assignment.", nil); err != nil {
					return err
				}
			}
			restSeen = true
		}
	}
	if len(bindings) > 255 {
		return p.report(p.previous.Position, "Too many targets in scattering assignment.", nil)
	}
	return nil
}

func (p *Parser) checkLoopExit(pos verb.Position, name, kind string) error {
	if name == "" && len(p.sourceLoops) != 0 {
		return nil
	}
	for i := len(p.sourceLoops) - 1; i >= 0; i-- {
		for _, candidate := range p.sourceLoops[i].names {
			if candidate != "" && strings.EqualFold(candidate, name) {
				return nil
			}
		}
	}
	if name == "" {
		return p.report(pos, fmt.Sprintf("No enclosing loop for `%s' statement", kind), nil)
	}
	return p.report(pos, fmt.Sprintf("Invalid loop name in `%s' statement: %s", kind, name), nil)
}
