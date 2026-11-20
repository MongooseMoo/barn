package compiler

import "github.com/MongooseMoo/barn/ir"

// statementLines numbers a program's statements the way Toast numbers the
// lines its tracebacks, callers() and queued-task headers report: by
// counting statements in the program's decompiled listing, one statement per
// line, from line 1 (decompile.cc find_hot_node and find_line_number). The
// source text's layout plays no part. An empty statement compiles to nothing
// and is not listed, so it takes no line; blank source lines are not counted;
// two statements on one source line are two lines; an expression spread over
// several source lines is one line. A compound statement's header is one line,
// each `elseif`, `else`, `except` and `finally` one more, and its closing
// keyword one more. elseIfArms marks the if statements written as `elseif`
// arms (parser.ElseIfArms); an `else` holding an if statement is otherwise an
// `else` line followed by a nested if.
//
// The result maps each statement to its line, and each except arm to the line
// its codes are evaluated on.
type statementLines struct {
	statements map[ir.Stmt]int
	handlers   map[*ir.ExceptionHandler]int
}

func numberStatements(program *ir.Program, elseIfArms map[*ir.IfStmt]bool) *statementLines {
	lines := &statementLines{
		statements: make(map[ir.Stmt]int),
		handlers:   make(map[*ir.ExceptionHandler]int),
	}
	lines.block(program.Statements, 1, elseIfArms)
	return lines
}

// block numbers stmts starting at line and returns the first line after them.
func (lines *statementLines) block(stmts []ir.Stmt, line int, elseIfArms map[*ir.IfStmt]bool) int {
	for _, stmt := range stmts {
		if _, empty := stmt.(*ir.EmptyStmt); empty {
			continue
		}
		lines.statements[stmt] = line
		switch s := stmt.(type) {
		case *ir.IfStmt:
			line = lines.ifArms(s, line, elseIfArms)
		case *ir.WhileStmt:
			line = lines.block(s.Body, line+1, elseIfArms)
		case *ir.CollectionLoopStmt:
			line = lines.block(s.Body, line+1, elseIfArms)
		case *ir.RangeLoopStmt:
			line = lines.block(s.Body, line+1, elseIfArms)
		case *ir.ForkStmt:
			line = lines.block(s.Body, line+1, elseIfArms)
		case *ir.TryStmt:
			line = lines.block(s.Body, line+1, elseIfArms)
			for i := range s.Handlers {
				lines.handlers[&s.Handlers[i]] = line
				line = lines.block(s.Handlers[i].Body, line+1, elseIfArms)
			}
			if s.Finalizer != nil {
				line = lines.block(s.Finalizer.Body, line+1, elseIfArms)
			}
		}
		// The statement itself, or the closing keyword of a compound one.
		line++
	}
	return line
}

// ifArms numbers an if statement whose `if` (or `elseif`) line is line, with
// its later arms, and returns the line of its `endif`.
func (lines *statementLines) ifArms(s *ir.IfStmt, line int, elseIfArms map[*ir.IfStmt]bool) int {
	line = lines.block(s.Body, line+1, elseIfArms)
	if len(s.Else) == 1 {
		if arm, ok := s.Else[0].(*ir.IfStmt); ok && elseIfArms[arm] {
			lines.statements[arm] = line
			return lines.ifArms(arm, line, elseIfArms)
		}
	}
	// An `else` holding only empty statements compiles to nothing and is not
	// listed.
	if next := lines.block(s.Else, line+1, elseIfArms); next != line+1 {
		line = next
	}
	return line
}
