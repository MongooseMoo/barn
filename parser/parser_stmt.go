package parser

import "fmt"

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
	case TOKEN_RETURN:
		return p.parseReturnStatement()
	case TOKEN_BREAK:
		return p.parseBreakStatement()
	case TOKEN_CONTINUE:
		return p.parseContinueStatement()
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
func (p *Parser) parseBreakStatement() (Stmt, error) {
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

	return &BreakStmt{
		Pos:   pos,
		Label: label,
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
