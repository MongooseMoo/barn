package parser

import (
	"barn/types"
	"fmt"
)

// ParseProgram parses a complete MOO program (sequence of statements)
func (p *Parser) ParseProgram() ([]Stmt, error) {
	var statements []Stmt

	for p.current.Type != TOKEN_EOF {
		stmt, err := p.parseStatement()
		if err != nil {
			return nil, err
		}
		statements = append(statements, stmt)
	}

	return statements, nil
}

// parseStatement parses a single statement
func (p *Parser) parseStatement() (Stmt, error) {
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
		return &ExprStmt{Pos: pos, Expr: nil}, nil
	default:
		// Expression statement
		return p.parseExpressionStatement()
	}
}

// parseIfStatement parses if/elseif/else/endif
func (p *Parser) parseIfStatement() (Stmt, error) {
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

	// Parse elseif clauses
	var elseIfs []*ElseIfClause
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

		elseIfs = append(elseIfs, &ElseIfClause{
			Pos:       elseIfPos,
			Condition: elseIfCond,
			Body:      elseIfBody,
		})
	}

	// Parse else clause (optional)
	var elseBody []Stmt
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

	return &IfStmt{
		Pos:       pos,
		Condition: condition,
		Body:      body,
		ElseIfs:   elseIfs,
		Else:      elseBody,
	}, nil
}

// parseWhileStatement parses while loops
func (p *Parser) parseWhileStatement() (Stmt, error) {
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

	return &WhileStmt{
		Pos:       pos,
		Label:     label,
		Condition: condition,
		Body:      body,
	}, nil
}

// parseForStatement parses for loops (list, range, or map iteration)
func (p *Parser) parseForStatement() (Stmt, error) {
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
	var container Expr
	var rangeStart, rangeEnd Expr
	var err error

	if p.current.Type == TOKEN_LBRACKET {
		// Range iteration: for x in [start..end]
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

	return &ForStmt{
		Pos:        startPos,
		Label:      label,
		Value:      value,
		Index:      index,
		Container:  container,
		RangeStart: rangeStart,
		RangeEnd:   rangeEnd,
		Body:       body,
	}, nil
}

// parseForkStatement parses fork statements
// Syntax: fork [varname] (delay) body endfork
func (p *Parser) parseForkStatement() (Stmt, error) {
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

	return &ForkStmt{
		Pos:     pos,
		Delay:   delay,
		VarName: varName,
		Body:    body,
	}, nil
}

// parseReturnStatement parses return statements
func (p *Parser) parseReturnStatement() (Stmt, error) {
	pos := p.current.Position
	p.nextToken() // consume 'return'

	var value Expr
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

	return &ReturnStmt{
		Pos:   pos,
		Value: value,
	}, nil
}

// parseBreakStatement parses break statements
// Syntax: break; OR break expr;
// The expr becomes the value of the enclosing loop
func (p *Parser) parseBreakStatement() (Stmt, error) {
	pos := p.current.Position
	p.nextToken() // consume 'break'

	var value Expr
	var label string

	// Check for optional expression or label
	if p.current.Type != TOKEN_SEMICOLON {
		// Parse expression
		expr, err := p.ParseExpression(0)
		if err != nil {
			return nil, fmt.Errorf("error in break value: %w", err)
		}
		value = expr
	}

	// Expect semicolon
	if p.current.Type != TOKEN_SEMICOLON {
		return nil, fmt.Errorf("expected ';' after break statement")
	}
	p.nextToken() // consume ';'

	return &BreakStmt{
		Pos:   pos,
		Label: label,
		Value: value,
	}, nil
}

// parseContinueStatement parses continue statements
func (p *Parser) parseContinueStatement() (Stmt, error) {
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

	return &ContinueStmt{
		Pos:   pos,
		Label: label,
	}, nil
}

// parseExpressionStatement parses an expression statement
func (p *Parser) parseExpressionStatement() (Stmt, error) {
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

	return &ExprStmt{
		Pos:  pos,
		Expr: expr,
	}, nil
}

// parseBody parses a sequence of statements until one of the terminators is reached
func (p *Parser) parseBody(terminators ...TokenType) ([]Stmt, error) {
	var body []Stmt

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
func (p *Parser) parseTryStatement() (Stmt, error) {
	pos := p.current.Position
	p.nextToken() // consume 'try'

	// Parse try body
	body, err := p.parseBody(TOKEN_EXCEPT, TOKEN_FINALLY, TOKEN_ENDTRY)
	if err != nil {
		return nil, err
	}

	// Check what follows: except, finally, or endtry
	var excepts []*ExceptClause
	var finally []Stmt
	hasExcept := false
	hasFinally := false

	// Parse except clauses (zero or more)
	for p.current.Type == TOKEN_EXCEPT {
		hasExcept = true
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

		// Parse exception codes
		var codes []types.ErrorCode
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
				// Convert error name to code
				code := p.errorNameToCode(p.current.Value)
				codes = append(codes, code)
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

		excepts = append(excepts, &ExceptClause{
			Pos:      exceptPos,
			Variable: variable,
			Codes:    codes,
			IsAny:    isAny,
			Body:     exceptBody,
		})
	}

	// Parse finally clause (optional)
	if p.current.Type == TOKEN_FINALLY {
		hasFinally = true
		p.nextToken() // consume 'finally'

		finally, err = p.parseBody(TOKEN_ENDTRY)
		if err != nil {
			return nil, err
		}
	}

	// Consume 'endtry'
	if p.current.Type != TOKEN_ENDTRY {
		return nil, fmt.Errorf("expected 'endtry'")
	}
	p.nextToken() // consume 'endtry'

	// Construct the appropriate statement based on what we found
	if hasExcept && hasFinally {
		return &TryExceptFinallyStmt{
			Pos:     pos,
			Body:    body,
			Excepts: excepts,
			Finally: finally,
		}, nil
	} else if hasExcept {
		return &TryExceptStmt{
			Pos:     pos,
			Body:    body,
			Excepts: excepts,
		}, nil
	} else if hasFinally {
		return &TryFinallyStmt{
			Pos:     pos,
			Body:    body,
			Finally: finally,
		}, nil
	} else {
		return nil, fmt.Errorf("try statement must have except or finally clause")
	}
}

// errorNameToCode converts an error name string to an ErrorCode
func (p *Parser) errorNameToCode(name string) types.ErrorCode {
	switch name {
	case "E_NONE":
		return types.E_NONE
	case "E_TYPE":
		return types.E_TYPE
	case "E_DIV":
		return types.E_DIV
	case "E_PERM":
		return types.E_PERM
	case "E_PROPNF":
		return types.E_PROPNF
	case "E_VERBNF":
		return types.E_VERBNF
	case "E_VARNF":
		return types.E_VARNF
	case "E_INVIND":
		return types.E_INVIND
	case "E_RECMOVE":
		return types.E_RECMOVE
	case "E_MAXREC":
		return types.E_MAXREC
	case "E_RANGE":
		return types.E_RANGE
	case "E_ARGS":
		return types.E_ARGS
	case "E_NACC":
		return types.E_NACC
	case "E_INVARG":
		return types.E_INVARG
	case "E_QUOTA":
		return types.E_QUOTA
	case "E_FLOAT":
		return types.E_FLOAT
	case "E_FILE":
		return types.E_FILE
	case "E_EXEC":
		return types.E_EXEC
	default:
		return types.E_NONE // Unknown error code
	}
}

// parseScatterOrExprStatement decides if {... is scatter assignment or expression
func (p *Parser) parseScatterOrExprStatement() (Stmt, error) {
	// Simple heuristic: if we see { followed by identifier/? /@, likely scatter
	// Otherwise, parse as expression
	if p.looksLikeScatter() {
		return p.parseScatterStatement()
	}
	return p.parseExpressionStatement()
}

// looksLikeScatter checks if the current position looks like scatter assignment
func (p *Parser) looksLikeScatter() bool {
	// { identifier or { ? or { @
	return p.peek.Type == TOKEN_IDENTIFIER || p.peek.Type == TOKEN_QUESTION || p.peek.Type == TOKEN_AT
}

// parseScatterStatement parses a scatter assignment
func (p *Parser) parseScatterStatement() (Stmt, error) {
	pos := p.current.Position
	p.nextToken() // consume '{'
	
	// Parse scatter targets
	var targets []ScatterTarget
	
	for p.current.Type != TOKEN_RBRACE && p.current.Type != TOKEN_EOF {
		target, err := p.parseScatterTarget()
		if err != nil {
			return nil, err
		}
		targets = append(targets, target)
		
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
	
	return &ScatterStmt{
		Pos:     pos,
		Targets: targets,
		Value:   value,
	}, nil
}

// parseScatterTarget parses a single scatter target: var, ?var, ?var = default, @var
func (p *Parser) parseScatterTarget() (ScatterTarget, error) {
	target := ScatterTarget{
		Pos: p.current.Position,
	}
	
	// Check for optional (?) or rest (@)
	if p.current.Type == TOKEN_QUESTION {
		target.Optional = true
		p.nextToken() // consume '?'
	} else if p.current.Type == TOKEN_AT {
		target.Rest = true
		p.nextToken() // consume '@'
	}
	
	// Parse identifier
	if p.current.Type != TOKEN_IDENTIFIER {
		return target, fmt.Errorf("expected identifier in scatter target")
	}
	target.Name = p.current.Value
	p.nextToken()
	
	// Check for default value (only for optional)
	if target.Optional && p.current.Type == TOKEN_ASSIGN {
		p.nextToken() // consume '='
		defaultExpr, err := p.ParseExpression(PREC_LOWEST)
		if err != nil {
			return target, err
		}
		target.Default = defaultExpr
	}
	
	return target, nil
}
