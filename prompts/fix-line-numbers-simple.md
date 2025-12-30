# Task: Skip String-Only Lines in Line Number Computation

## Simple Fix

Add `LineMap map[int]int` to `VerbProgram` (in `db/object.go`). This maps raw source lines to effective lines (skipping string-only lines).

## Implementation

### 1. Add to VerbProgram (db/object.go)
```go
type VerbProgram struct {
	Statements []parser.Stmt
	LineMap    map[int]int // Raw line -> Effective line (skipping doc strings)
}
```

### 2. Compute in CompileVerb (db/verbs.go)
After parsing, compute LineMap:
```go
func ComputeLineMap(stmts []parser.Stmt) map[int]int {
	lineMap := make(map[int]int)
	effectiveLine := 0
	lastRawLine := 0

	for _, stmt := range stmts {
		rawLine := stmt.Position().Line

		// Check if this statement is ONLY a string literal
		if isDocString(stmt) {
			// Don't increment effectiveLine, but still map this line
			lineMap[rawLine] = effectiveLine
		} else {
			effectiveLine++
			lineMap[rawLine] = effectiveLine
		}
		lastRawLine = rawLine
	}

	// Map any lines beyond last statement
	for i := lastRawLine + 1; i <= lastRawLine + 100; i++ {
		lineMap[i] = effectiveLine
	}

	return lineMap
}

func isDocString(stmt parser.Stmt) bool {
	exprStmt, ok := stmt.(*parser.ExprStmt)
	if !ok || exprStmt.Expr == nil {
		return false
	}
	_, isLiteral := exprStmt.Expr.(*parser.LiteralExpr)
	// It's a doc string if it's just a literal expression (typically string)
	return isLiteral
}
```

### 3. Use in traceback (task/traceback.go)
The ActivationFrame needs access to LineMap to convert raw line to effective line.

This might require passing LineMap through the execution context.

## Test

After fix, when running `news` command, error line should be a real code line, not "line 1" (which is a doc string).

## CRITICAL

- Keep it simple
- Don't add runtime line tracking
- Just compute the mapping at compile time and use it for display
