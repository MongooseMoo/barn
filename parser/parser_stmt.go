package parser

import (
	"fmt"

	"github.com/MongooseMoo/barn/verb"
)

// ParseError is a syntax error carrying the source line of the offending token.
// ToastStunt collapses essentially all parse errors to the generic message
// "syntax error" and reports them as "Line N:  syntax error" (the inner
// fmt.Errorf detail is for Barn-internal diagnostics only and is not surfaced to
// MOO callers). Line is the line of p.current at the point parsing failed; for
// unexpected-EOF the lexer reports a phantom final line (numLines+1), matching
// Toast.
type ParseError struct {
	Line int    // 1-based source line of the offending token
	Msg  string // generic message surfaced to MOO callers ("syntax error")
	// Detail preserves Barn's specific inner message (e.g. "expected ';'") for
	// internal diagnostics; it is NOT part of the MOO-facing format.
	Detail error
}

func (e *ParseError) Error() string { return e.Msg }

func (e *ParseError) Unwrap() error { return e.Detail }

// ParseProgram parses a complete MOO program (sequence of statements)
func (p *Parser) ParseProgram() (*verb.Program, error) {
	var statements []verb.Stmt

	for p.current.Type != TOKEN_EOF {
		stmt, err := p.parseStatement()
		if err != nil {
			// Capture the line of the offending token and present Toast's
			// generic "syntax error". p.current is the token parsing choked on.
			return nil, &ParseError{
				Line:   p.current.Position.Line,
				Msg:    "syntax error",
				Detail: err,
			}
		}
		statements = append(statements, stmt)
	}

	program := &verb.Program{Statements: statements}
	if err := verb.ValidateNesting(program); err != nil {
		return nil, &ParseError{Line: p.current.Position.Line, Msg: "syntax error", Detail: err}
	}
	return program, nil
}

// parseStatement parses a single statement
func (p *Parser) parseStatement() (verb.Stmt, error) {
	p.statementCalls++
	defer func() { p.statementCalls-- }()
	if p.statementCalls > MaxNestingDepth+1 {
		return nil, p.limitError()
	}
	switch p.current.Type {
	case TOKEN_IF:
		return p.parseIfStatement()
	case TOKEN_WHILE:
		return p.parseWhileStatement()
	case TOKEN_FOR:
		return p.parseForStatement()
	case TOKEN_FORK:
		return p.parseForkStatement()
	case TOKEN_TRY:
		return p.parseTryStatement()
	case TOKEN_RETURN:
		return p.parseReturnStatement()
	case TOKEN_BREAK:
		return p.parseBreakStatement()
	case TOKEN_CONTINUE:
		return p.parseContinueStatement()
	case TOKEN_LBRACE:
		// Could be scatter assignment or list expression
		return p.parseScatterOrExprStatement()
	case TOKEN_SEMICOLON:
		// Empty statement
		pos := p.current.Position
		p.nextToken()
		return &verb.ExprStmt{Pos: pos, Expr: nil}, nil
	default:
		// Expression statement
		return p.parseExpressionStatement()
	}
}

// parseIfStatement parses if/elseif/else/endif
func (p *Parser) parseIfStatement() (verb.Stmt, error) {
	pos := p.current.Position
	p.nextToken() // consume 'if'

	// Parse condition
	if p.current.Type != TOKEN_LPAREN {
		return nil, fmt.Errorf("expected '(' after 'if'")
	}
	p.nextToken() // consume '('

	condition, err := p.ParseExpression(PREC_LOWEST)
	if err != nil {
		return nil, err
	}

	if p.current.Type != TOKEN_RPAREN {
		return nil, fmt.Errorf("expected ')' after if condition")
	}
	p.nextToken() // consume ')'

	// Parse body
	body, err := p.parseBody(TOKEN_ELSEIF, TOKEN_ELSE, TOKEN_ENDIF)
	if err != nil {
		return nil, err
	}

	conditions := []verb.Expr{condition}
	bodies := [][]verb.Stmt{body}
	positions := []verb.Position{pos}

	// Parse concrete elseif clauses for direct lowering to nested conditionals.
	for p.current.Type == TOKEN_ELSEIF {
		elseIfPos := p.current.Position
		p.nextToken() // consume 'elseif'

		if p.current.Type != TOKEN_LPAREN {
			return nil, fmt.Errorf("expected '(' after 'elseif'")
		}
		p.nextToken()

		elseIfCond, err := p.ParseExpression(PREC_LOWEST)
		if err != nil {
			return nil, err
		}

		if p.current.Type != TOKEN_RPAREN {
			return nil, fmt.Errorf("expected ')' after elseif condition")
		}
		p.nextToken()

		elseIfBody, err := p.parseBody(TOKEN_ELSEIF, TOKEN_ELSE, TOKEN_ENDIF)
		if err != nil {
			return nil, err
		}

		conditions = append(conditions, elseIfCond)
		bodies = append(bodies, elseIfBody)
		positions = append(positions, elseIfPos)
	}

	// Parse else clause (optional)
	var elseBody []verb.Stmt
	if p.current.Type == TOKEN_ELSE {
		p.nextToken() // consume 'else'
		elseBody, err = p.parseBody(TOKEN_ENDIF)
		if err != nil {
			return nil, err
		}
	}

	// Expect endif
	if p.current.Type != TOKEN_ENDIF {
		return nil, fmt.Errorf("expected 'endif'")
	}
	p.nextToken() // consume 'endif'

	for i := len(conditions) - 1; i > 0; i-- {
		elseBody = []verb.Stmt{&verb.IfStmt{
			Pos:       positions[i],
			Condition: conditions[i],
			Body:      bodies[i],
			Else:      elseBody,
		}}
	}

	return &verb.IfStmt{Pos: pos, Condition: condition, Body: body, Else: elseBody}, nil
}

// parseWhileStatement parses while loops
func (p *Parser) parseWhileStatement() (verb.Stmt, error) {
	pos := p.current.Position
	p.nextToken() // consume 'while'

	// Check for optional label
	var label string
	if p.current.Type == TOKEN_IDENTIFIER && p.peek.Type == TOKEN_LPAREN {
		label = p.current.Value
		p.nextToken() // consume label
	}

	// Parse condition
	if p.current.Type != TOKEN_LPAREN {
		return nil, fmt.Errorf("expected '(' in while statement")
	}
	p.nextToken() // consume '('

	condition, err := p.ParseExpression(PREC_LOWEST)
	if err != nil {
		return nil, err
	}

	if p.current.Type != TOKEN_RPAREN {
		return nil, fmt.Errorf("expected ')' after while condition")
	}
	p.nextToken() // consume ')'

	// Parse body
	body, err := p.parseBody(TOKEN_ENDWHILE)
	if err != nil {
		return nil, err
	}

	// Expect endwhile
	if p.current.Type != TOKEN_ENDWHILE {
		return nil, fmt.Errorf("expected 'endwhile'")
	}
	p.nextToken() // consume 'endwhile'

	return &verb.WhileStmt{
		Pos:       pos,
		Label:     label,
		Condition: condition,
		Body:      body,
	}, nil
}

// parseForStatement parses for loops (list, range, or map iteration)
func (p *Parser) parseForStatement() (verb.Stmt, error) {
	startPos := p.current.Position
	p.nextToken() // consume 'for'

	// Check for optional label
	var label string
	if p.current.Type == TOKEN_IDENTIFIER && p.peek.Type == TOKEN_IDENTIFIER {
		// Might be a label - need to distinguish from "for x in (...)"
		// Look ahead further
		label = p.current.Value
		p.nextToken() // consume label
	}

	// Parse variable name(s)
	if p.current.Type != TOKEN_IDENTIFIER {
		return nil, fmt.Errorf("expected identifier in for loop")
	}
	value := p.current.Value
	p.nextToken()

	var index string
	if p.current.Type == TOKEN_COMMA {
		p.nextToken() // consume comma
		if p.current.Type != TOKEN_IDENTIFIER {
			return nil, fmt.Errorf("expected identifier after comma in for loop")
		}
		index = p.current.Value
		p.nextToken()
	}

	// Expect 'in'
	if p.current.Type != TOKEN_IN {
		return nil, fmt.Errorf("expected 'in' in for loop")
	}
	p.nextToken() // consume 'in'

	// Check for range [start..end] or container (expr)
	var container verb.Expr
	var rangeStart, rangeEnd verb.Expr
	var err error

	if p.current.Type == TOKEN_LBRACKET {
		// Range iteration: for x in [start..end]
		if index != "" {
			return nil, fmt.Errorf("range loop cannot bind an index variable")
		}
		p.nextToken() // consume '['

		rangeStart, err = p.ParseExpression(PREC_LOWEST)
		if err != nil {
			return nil, err
		}

		if p.current.Type != TOKEN_RANGE {
			return nil, fmt.Errorf("expected '..' in range expression")
		}
		p.nextToken() // consume '..'

		rangeEnd, err = p.ParseExpression(PREC_LOWEST)
		if err != nil {
			return nil, err
		}

		if p.current.Type != TOKEN_RBRACKET {
			return nil, fmt.Errorf("expected ']' after range expression")
		}
		p.nextToken() // consume ']'

	} else if p.current.Type == TOKEN_LPAREN {
		// List/map iteration: for x in (expr)
		p.nextToken() // consume '('

		container, err = p.ParseExpression(PREC_LOWEST)
		if err != nil {
			return nil, err
		}

		if p.current.Type != TOKEN_RPAREN {
			return nil, fmt.Errorf("expected ')' after for loop expression")
		}
		p.nextToken() // consume ')'
	} else if p.current.Type == TOKEN_LBRACE {
		// List literal iteration: for x in {expr, ...} or {start..end}
		// Parse the list/map expression directly
		container, err = p.ParseExpression(PREC_LOWEST)
		if err != nil {
			return nil, err
		}
	} else {
		return nil, fmt.Errorf("expected '[' or '(' after 'in' in for loop")
	}

	// Parse body
	body, err := p.parseBody(TOKEN_ENDFOR)
	if err != nil {
		return nil, err
	}

	// Expect endfor
	if p.current.Type != TOKEN_ENDFOR {
		return nil, fmt.Errorf("expected 'endfor'")
	}
	p.nextToken() // consume 'endfor'

	if container != nil {
		return &verb.CollectionLoopStmt{
			Pos:        startPos,
			Label:      label,
			Value:      value,
			Index:      index,
			Collection: container,
			Body:       body,
		}, nil
	}

	return &verb.RangeLoopStmt{
		Pos:   startPos,
		Label: label,
		Value: value,
		Start: rangeStart,
		End:   rangeEnd,
		Body:  body,
	}, nil
}

// parseForkStatement parses fork statements
// Syntax: fork [varname] (delay) body endfork
func (p *Parser) parseForkStatement() (verb.Stmt, error) {
	pos := p.current.Position
	p.nextToken() // consume 'fork'

	// Check for optional variable name
	var varName string
	if p.current.Type == TOKEN_IDENTIFIER && p.peek.Type == TOKEN_LPAREN {
		varName = p.current.Value
		p.nextToken() // consume variable name
	}

	// Parse delay expression in parentheses
	if p.current.Type != TOKEN_LPAREN {
		return nil, fmt.Errorf("expected '(' after 'fork'")
	}
	p.nextToken() // consume '('

	delay, err := p.ParseExpression(PREC_LOWEST)
	if err != nil {
		return nil, err
	}

	if p.current.Type != TOKEN_RPAREN {
		return nil, fmt.Errorf("expected ')' after fork delay")
	}
	p.nextToken() // consume ')'

	// Parse fork body
	body, err := p.parseBody(TOKEN_ENDFORK)
	if err != nil {
		return nil, err
	}

	// Expect endfork
	if p.current.Type != TOKEN_ENDFORK {
		return nil, fmt.Errorf("expected 'endfork'")
	}
	p.nextToken() // consume 'endfork'

	return &verb.ForkStmt{
		Pos:     pos,
		Delay:   delay,
		VarName: varName,
		Body:    body,
	}, nil
}

// parseReturnStatement parses return statements
func (p *Parser) parseReturnStatement() (verb.Stmt, error) {
	pos := p.current.Position
	p.nextToken() // consume 'return'

	var value verb.Expr
	var err error

	// Check if there's an expression to return
	if p.current.Type != TOKEN_SEMICOLON && p.current.Type != TOKEN_EOF {
		value, err = p.ParseExpression(PREC_LOWEST)
		if err != nil {
			return nil, err
		}
	}

	// Expect semicolon
	if p.current.Type != TOKEN_SEMICOLON {
		return nil, fmt.Errorf("expected ';' after return statement")
	}
	p.nextToken() // consume ';'

	return &verb.ReturnStmt{
		Pos:   pos,
		Value: value,
	}, nil
}

// parseBreakStatement parses break statements.
// Syntax: break; OR break ID; where ID names an enclosing loop.
// Mirrors parseContinueStatement (ToastStunt parser.y:241-252).
func (p *Parser) parseBreakStatement() (verb.Stmt, error) {
	pos := p.current.Position
	p.nextToken() // consume 'break'

	var label string
	if p.current.Type == TOKEN_IDENTIFIER {
		label = p.current.Value
		p.nextToken()
	}

	// Expect semicolon
	if p.current.Type != TOKEN_SEMICOLON {
		return nil, fmt.Errorf("expected ';' after break statement")
	}
	p.nextToken() // consume ';'

	return &verb.BreakStmt{
		Pos:   pos,
		Label: label,
	}, nil
}

// parseContinueStatement parses continue statements
func (p *Parser) parseContinueStatement() (verb.Stmt, error) {
	pos := p.current.Position
	p.nextToken() // consume 'continue'

	var label string
	if p.current.Type == TOKEN_IDENTIFIER {
		label = p.current.Value
		p.nextToken()
	}

	// Expect semicolon
	if p.current.Type != TOKEN_SEMICOLON {
		return nil, fmt.Errorf("expected ';' after continue statement")
	}
	p.nextToken() // consume ';'

	return &verb.ContinueStmt{
		Pos:   pos,
		Label: label,
	}, nil
}

// parseExpressionStatement parses an expression statement
func (p *Parser) parseExpressionStatement() (verb.Stmt, error) {
	pos := p.current.Position

	expr, err := p.ParseExpression(PREC_LOWEST)
	if err != nil {
		return nil, err
	}

	// Expect semicolon
	if p.current.Type != TOKEN_SEMICOLON {
		return nil, fmt.Errorf("expected ';' after expression statement")
	}
	p.nextToken() // consume ';'

	return &verb.ExprStmt{
		Pos:  pos,
		Expr: expr,
	}, nil
}

// parseBody parses a sequence of statements until one of the terminators is reached
func (p *Parser) parseBody(terminators ...TokenType) ([]verb.Stmt, error) {
	var body []verb.Stmt

	for {
		// Check if we've reached a terminator
		isTerminator := false
		for _, term := range terminators {
			if p.current.Type == term {
				isTerminator = true
				break
			}
		}
		if isTerminator || p.current.Type == TOKEN_EOF {
			break
		}

		stmt, err := p.parseStatement()
		if err != nil {
			return nil, err
		}
		body = append(body, stmt)
	}

	return body, nil
}

// parseTryStatement parses try/except/finally/endtry statements
// Handles three forms:
// - try ... except ... endtry
// - try ... finally ... endtry
// - try ... except ... finally ... endtry
func (p *Parser) parseTryStatement() (verb.Stmt, error) {
	pos := p.current.Position
	p.nextToken() // consume 'try'

	// Parse try body
	body, err := p.parseBody(TOKEN_EXCEPT, TOKEN_FINALLY, TOKEN_ENDTRY)
	if err != nil {
		return nil, err
	}

	var handlers []verb.ExceptionHandler
	var finalizer *verb.Finalizer

	// Parse except clauses (zero or more)
	for p.current.Type == TOKEN_EXCEPT {
		exceptPos := p.current.Position
		p.nextToken() // consume 'except'

		// Optional variable to bind the error
		var variable string
		if p.current.Type == TOKEN_IDENTIFIER {
			variable = p.current.Value
			p.nextToken()
		}

		// Parse error codes in parentheses
		if p.current.Type != TOKEN_LPAREN {
			return nil, fmt.Errorf("expected '(' after 'except'")
		}
		p.nextToken() // consume '('

		// Parse exception names
		var codes []string
		isAny := false

		if p.current.Type == TOKEN_ANY || (p.current.Type == TOKEN_IDENTIFIER && p.current.Value == "ANY") {
			isAny = true
			p.nextToken()
		} else {
			// Parse list of error codes
			for {
				if p.current.Type != TOKEN_ERROR_LIT {
					return nil, fmt.Errorf("expected error code, got %v", p.current.Type)
				}
				if !isErrorName(p.current.Value) {
					return nil, fmt.Errorf("unknown error code: %s", p.current.Value)
				}
				codes = append(codes, p.current.Value)
				p.nextToken()

				if p.current.Type == TOKEN_COMMA {
					p.nextToken() // consume ','
				} else {
					break
				}
			}
		}

		if p.current.Type != TOKEN_RPAREN {
			return nil, fmt.Errorf("expected ')' after error codes")
		}
		p.nextToken() // consume ')'

		// Parse except body
		exceptBody, err := p.parseBody(TOKEN_EXCEPT, TOKEN_FINALLY, TOKEN_ENDTRY)
		if err != nil {
			return nil, err
		}

		handlers = append(handlers, verb.ExceptionHandler{
			Pos:      exceptPos,
			Variable: variable,
			Codes:    codes,
			IsAny:    isAny,
			Body:     exceptBody,
		})
	}

	// Parse finally clause (optional)
	if p.current.Type == TOKEN_FINALLY {
		finalizerPos := p.current.Position
		p.nextToken() // consume 'finally'

		finalizerBody, err := p.parseBody(TOKEN_ENDTRY)
		if err != nil {
			return nil, err
		}
		finalizer = &verb.Finalizer{Pos: finalizerPos, Body: finalizerBody}
	}

	// Consume 'endtry'
	if p.current.Type != TOKEN_ENDTRY {
		return nil, fmt.Errorf("expected 'endtry'")
	}
	p.nextToken() // consume 'endtry'

	if len(handlers) == 0 && finalizer == nil {
		return nil, fmt.Errorf("try statement must have except or finally clause")
	}

	return &verb.TryStmt{Pos: pos, Body: body, Handlers: handlers, Finalizer: finalizer}, nil
}

// parseScatterOrExprStatement decides if {... is scatter assignment or expression
func (p *Parser) parseScatterOrExprStatement() (verb.Stmt, error) {
	// Simple heuristic: if we see { followed by identifier/? /@, likely scatter
	// Otherwise, parse as expression
	if p.looksLikeScatter() {
		return p.parseScatterStatement()
	}
	return p.parseExpressionStatement()
}

// looksLikeScatter reports whether the leading '{' begins a scatter-assignment
// target rather than a list-literal expression.
//
// A '{...}' is fundamentally a list literal; it is a scatter target ONLY when a
// top-level '=' immediately follows the matching '}'. This mirrors ToastStunt's
// grammar (src/parser.y): a brace-group reduces to a list expr via
//
//	expr: '{' arglist '}'                          (parser.y:630)
//
// and only becomes a scatter assignment when an '=' follows, either by
// reinterpreting a list LHS
//
//	expr: expr '=' expr   // if LHS is EXPR_LIST -> EXPR_SCATTER   (parser.y:466-487)
//
// or via the dedicated optional/rest production
//
//	expr: '{' scatter '}' '=' expr                 (parser.y:488-495)
//
// The LALR parser distinguishes the two with one token of lookahead on '='.
// We reproduce that by scanning a cloned lexer over the brace group (tracking
// (), [] and {} nesting) and checking the token after the matching '}'.
//
// p.current is the opening '{'; p.peek is the first token inside it.
func (p *Parser) looksLikeScatter() bool {
	// Lexer is a pure value struct (no shared mutable state), so a copy gives an
	// independent cursor we can advance without disturbing the real parser.
	lex := *p.lexer
	depth := 1 // the opening '{' (p.current) is already consumed
	tok := p.peek
	for {
		switch tok.Type {
		case TOKEN_LBRACE, TOKEN_LBRACKET, TOKEN_LPAREN:
			depth++
		case TOKEN_RBRACE, TOKEN_RBRACKET, TOKEN_RPAREN:
			depth--
			if depth == 0 {
				// tok is the matching '}'; scatter iff a top-level '=' follows.
				return lex.NextToken().Type == TOKEN_ASSIGN
			}
		case TOKEN_EOF:
			return false
		}
		tok = lex.NextToken()
	}
}

// parseScatterStatement parses a scatter assignment
func (p *Parser) parseScatterStatement() (verb.Stmt, error) {
	pos := p.current.Position
	p.nextToken() // consume '{'

	var bindings []verb.Binding
	hasRest := false

	for p.current.Type != TOKEN_RBRACE && p.current.Type != TOKEN_EOF {
		binding, err := p.parseScatterBinding()
		if err != nil {
			return nil, err
		}
		if _, rest := binding.(*verb.RestBinding); rest {
			if hasRest {
				return nil, fmt.Errorf("more than one '@' target in scattering assignment")
			}
			hasRest = true
		}
		bindings = append(bindings, binding)

		if p.current.Type == TOKEN_COMMA {
			p.nextToken() // consume ','
		} else if p.current.Type != TOKEN_RBRACE {
			return nil, fmt.Errorf("expected ',' or '}' in scatter")
		}
	}

	if p.current.Type != TOKEN_RBRACE {
		return nil, fmt.Errorf("expected '}' after scatter targets")
	}
	p.nextToken() // consume '}'

	// Must be followed by =
	if p.current.Type != TOKEN_ASSIGN {
		return nil, fmt.Errorf("scatter must be followed by '='")
	}
	p.nextToken() // consume '='

	// Parse value expression
	value, err := p.ParseExpression(PREC_LOWEST)
	if err != nil {
		return nil, err
	}

	// Consume semicolon
	if p.current.Type != TOKEN_SEMICOLON {
		return nil, fmt.Errorf("expected ';' after scatter assignment")
	}
	p.nextToken() // consume ';'

	return &verb.ExprStmt{
		Pos: pos,
		Expr: &verb.AssignExpr{
			Pos:    pos,
			Target: &verb.DestructuringTarget{Pos: pos, Bindings: bindings},
			Value:  value,
		},
	}, nil
}

// parseScatterBinding parses a single binding: var, ?var, ?var = default, @var.
func (p *Parser) parseScatterBinding() (verb.Binding, error) {
	pos := p.current.Position

	optional := false
	rest := false
	if p.current.Type == TOKEN_QUESTION {
		optional = true
		p.nextToken() // consume '?'
	} else if p.current.Type == TOKEN_AT {
		rest = true
		p.nextToken() // consume '@'
	}

	// Parse identifier
	if p.current.Type != TOKEN_IDENTIFIER {
		return nil, fmt.Errorf("expected identifier in scatter target")
	}
	name := p.current.Value
	p.nextToken()

	if optional {
		var defaultExpr verb.Expr
		if p.current.Type == TOKEN_ASSIGN {
			p.nextToken() // consume '='
			var err error
			defaultExpr, err = p.ParseExpression(PREC_LOWEST)
			if err != nil {
				return nil, err
			}
		}
		return &verb.OptionalBinding{Pos: pos, Name: name, Default: defaultExpr}, nil
	}
	if rest {
		return &verb.RestBinding{Pos: pos, Name: name}, nil
	}

	return &verb.RequiredBinding{Pos: pos, Name: name}, nil
}
