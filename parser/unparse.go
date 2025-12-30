package parser

import (
	"barn/types"
	"fmt"
	"strconv"
	"strings"
)

// Operator precedence levels (higher = tighter binding)
const (
	precedenceLowest = iota
	precedenceAssign      // =
	precedenceTernary     // ? |
	precedenceOr          // ||
	precedenceAnd         // &&
	precedenceBitOr       // |
	precedenceBitXor      // ^
	precedenceBitAnd      // &
	precedenceEquality    // == !=
	precedenceComparison  // < <= > >= in
	precedenceShift       // << >>
	precedenceAdditive    // + -
	precedenceMultiply    // * / %
	precedenceExponent    // ^
	precedenceUnary       // - ! ~
	precedenceProperty    // . : [] (highest - property access, verb call, index)
)

// UnparseProgram converts AST statements back to source code lines
func UnparseProgram(stmts []Stmt) []string {
	if len(stmts) == 0 {
		return []string{}
	}

	var lines []string
	for _, stmt := range stmts {
		line := unparseStmt(stmt, 0)
		lines = append(lines, line)
	}
	return lines
}

// unparseStmt converts a statement to source code
func unparseStmt(stmt Stmt, indent int) string {
	indentStr := strings.Repeat("  ", indent)

	switch s := stmt.(type) {
	case *ExprStmt:
		return indentStr + unparseExpr(s.Expr, precedenceLowest) + ";"

	case *ReturnStmt:
		if s.Value == nil {
			return indentStr + "return;"
		}
		return indentStr + "return " + unparseExpr(s.Value, precedenceLowest) + ";"

	case *IfStmt:
		var sb strings.Builder
		sb.WriteString(indentStr + "if (" + unparseExpr(s.Condition, precedenceLowest) + ")\n")
		for _, bodyStmt := range s.Body {
			sb.WriteString(unparseStmt(bodyStmt, indent+1) + "\n")
		}
		for _, elseif := range s.ElseIfs {
			sb.WriteString(indentStr + "elseif (" + unparseExpr(elseif.Condition, precedenceLowest) + ")\n")
			for _, bodyStmt := range elseif.Body {
				sb.WriteString(unparseStmt(bodyStmt, indent+1) + "\n")
			}
		}
		if len(s.Else) > 0 {
			sb.WriteString(indentStr + "else\n")
			for _, bodyStmt := range s.Else {
				sb.WriteString(unparseStmt(bodyStmt, indent+1) + "\n")
			}
		}
		sb.WriteString(indentStr + "endif")
		return strings.TrimSuffix(sb.String(), "\n")

	case *WhileStmt:
		var sb strings.Builder
		if s.Label != "" {
			sb.WriteString(indentStr + "while " + s.Label + " (" + unparseExpr(s.Condition, precedenceLowest) + ")\n")
		} else {
			sb.WriteString(indentStr + "while (" + unparseExpr(s.Condition, precedenceLowest) + ")\n")
		}
		for _, bodyStmt := range s.Body {
			sb.WriteString(unparseStmt(bodyStmt, indent+1) + "\n")
		}
		sb.WriteString(indentStr + "endwhile")
		return strings.TrimSuffix(sb.String(), "\n")

	case *ForStmt:
		var sb strings.Builder
		sb.WriteString(indentStr + "for ")
		if s.Label != "" {
			sb.WriteString(s.Label + " ")
		}
		if s.Index != "" {
			sb.WriteString(s.Value + " in [" + s.Index + ".." + strconv.Itoa(len(s.Body)) + "]\n")
		} else if s.Container != nil {
			sb.WriteString(s.Value + " in (" + unparseExpr(s.Container, precedenceLowest) + ")\n")
		} else {
			// Range loop
			sb.WriteString(s.Value + " in [" + unparseExpr(s.RangeStart, precedenceLowest) + ".." + unparseExpr(s.RangeEnd, precedenceLowest) + "]\n")
		}
		for _, bodyStmt := range s.Body {
			sb.WriteString(unparseStmt(bodyStmt, indent+1) + "\n")
		}
		sb.WriteString(indentStr + "endfor")
		return strings.TrimSuffix(sb.String(), "\n")

	case *BreakStmt:
		if s.Value != nil {
			return indentStr + "break " + unparseExpr(s.Value, precedenceLowest) + ";"
		}
		if s.Label != "" {
			return indentStr + "break " + s.Label + ";"
		}
		return indentStr + "break;"

	case *ContinueStmt:
		if s.Label != "" {
			return indentStr + "continue " + s.Label + ";"
		}
		return indentStr + "continue;"

	case *TryExceptStmt:
		var sb strings.Builder
		sb.WriteString(indentStr + "try\n")
		for _, bodyStmt := range s.Body {
			sb.WriteString(unparseStmt(bodyStmt, indent+1) + "\n")
		}
		for _, except := range s.Excepts {
			sb.WriteString(indentStr + "except ")
			if except.Variable != "" {
				sb.WriteString(except.Variable + " ")
			}
			sb.WriteString("(")
			if except.IsAny {
				sb.WriteString("ANY")
			} else {
				for i, code := range except.Codes {
					if i > 0 {
						sb.WriteString(", ")
					}
					sb.WriteString(code.String())
				}
			}
			sb.WriteString(")\n")
			for _, bodyStmt := range except.Body {
				sb.WriteString(unparseStmt(bodyStmt, indent+1) + "\n")
			}
		}
		sb.WriteString(indentStr + "endtry")
		return strings.TrimSuffix(sb.String(), "\n")

	case *TryFinallyStmt:
		var sb strings.Builder
		sb.WriteString(indentStr + "try\n")
		for _, bodyStmt := range s.Body {
			sb.WriteString(unparseStmt(bodyStmt, indent+1) + "\n")
		}
		sb.WriteString(indentStr + "finally\n")
		for _, bodyStmt := range s.Finally {
			sb.WriteString(unparseStmt(bodyStmt, indent+1) + "\n")
		}
		sb.WriteString(indentStr + "endtry")
		return strings.TrimSuffix(sb.String(), "\n")

	case *TryExceptFinallyStmt:
		var sb strings.Builder
		sb.WriteString(indentStr + "try\n")
		for _, bodyStmt := range s.Body {
			sb.WriteString(unparseStmt(bodyStmt, indent+1) + "\n")
		}
		for _, except := range s.Excepts {
			sb.WriteString(indentStr + "except ")
			if except.Variable != "" {
				sb.WriteString(except.Variable + " ")
			}
			sb.WriteString("(")
			if except.IsAny {
				sb.WriteString("ANY")
			} else {
				for i, code := range except.Codes {
					if i > 0 {
						sb.WriteString(", ")
					}
					sb.WriteString(code.String())
				}
			}
			sb.WriteString(")\n")
			for _, bodyStmt := range except.Body {
				sb.WriteString(unparseStmt(bodyStmt, indent+1) + "\n")
			}
		}
		sb.WriteString(indentStr + "finally\n")
		for _, bodyStmt := range s.Finally {
			sb.WriteString(unparseStmt(bodyStmt, indent+1) + "\n")
		}
		sb.WriteString(indentStr + "endtry")
		return strings.TrimSuffix(sb.String(), "\n")

	case *ScatterStmt:
		var sb strings.Builder
		sb.WriteString(indentStr + "{")
		for i, target := range s.Targets {
			if i > 0 {
				sb.WriteString(", ")
			}
			if target.Optional {
				sb.WriteString("?")
			}
			if target.Rest {
				sb.WriteString("@")
			}
			sb.WriteString(target.Name)
			if target.Default != nil {
				sb.WriteString(" = " + unparseExpr(target.Default, precedenceLowest))
			}
		}
		sb.WriteString("} = " + unparseExpr(s.Value, precedenceLowest) + ";")
		return sb.String()

	case *ForkStmt:
		var sb strings.Builder
		sb.WriteString(indentStr + "fork ")
		if s.VarName != "" {
			sb.WriteString(s.VarName + " ")
		}
		sb.WriteString("(" + unparseExpr(s.Delay, precedenceLowest) + ")\n")
		for _, bodyStmt := range s.Body {
			sb.WriteString(unparseStmt(bodyStmt, indent+1) + "\n")
		}
		sb.WriteString(indentStr + "endfork")
		return strings.TrimSuffix(sb.String(), "\n")

	default:
		return indentStr + fmt.Sprintf("<unknown statement: %T>", stmt)
	}
}

// unparseExpr converts an expression to source code
func unparseExpr(expr Expr, parentPrecedence int) string {
	switch e := expr.(type) {
	case *LiteralExpr:
		return unparseLiteral(e.Value)

	case *IdentifierExpr:
		return e.Name

	case *UnaryExpr:
		op := unparseUnaryOp(e.Operator)
		operand := unparseExpr(e.Operand, precedenceUnary)
		return op + operand

	case *BinaryExpr:
		return unparseBinaryExpr(e, parentPrecedence)

	case *TernaryExpr:
		prec := precedenceTernary
		cond := unparseExpr(e.Condition, prec)
		then := unparseExpr(e.ThenExpr, prec)
		els := unparseExpr(e.ElseExpr, prec)
		result := cond + " ? " + then + " | " + els
		if prec < parentPrecedence {
			return "(" + result + ")"
		}
		return result

	case *ParenExpr:
		return "(" + unparseExpr(e.Expr, precedenceLowest) + ")"

	case *IndexMarkerExpr:
		if e.Marker == TOKEN_CARET {
			return "^"
		}
		return "$"

	case *IndexExpr:
		base := unparseExpr(e.Expr, precedenceProperty)
		index := unparseExpr(e.Index, precedenceLowest)
		return base + "[" + index + "]"

	case *RangeExpr:
		base := unparseExpr(e.Expr, precedenceProperty)
		start := unparseExpr(e.Start, precedenceLowest)
		end := unparseExpr(e.End, precedenceLowest)
		// NO spaces around ..
		return base + "[" + start + ".." + end + "]"

	case *PropertyExpr:
		return unparsePropertyExpr(e)

	case *VerbCallExpr:
		base := unparseExpr(e.Expr, precedenceProperty)
		var verb string
		if e.Verb != "" {
			verb = e.Verb
		} else {
			verb = "(" + unparseExpr(e.VerbExpr, precedenceLowest) + ")"
		}
		args := unparseArgs(e.Args)
		return base + ":" + verb + "(" + args + ")"

	case *BuiltinCallExpr:
		args := unparseArgs(e.Args)
		return e.Name + "(" + args + ")"

	case *SpliceExpr:
		return "@" + unparseExpr(e.Expr, precedenceUnary)

	case *CatchExpr:
		result := unparseExpr(e.Expr, precedenceTernary)
		result += " `! "
		if len(e.Codes) == 0 {
			result += "ANY"
		} else {
			for i, code := range e.Codes {
				if i > 0 {
					result += ", "
				}
				result += code.String()
			}
		}
		if e.Default != nil {
			result += " => " + unparseExpr(e.Default, precedenceTernary)
		}
		return result

	case *AssignExpr:
		prec := precedenceAssign
		target := unparseExpr(e.Target, prec)
		value := unparseExpr(e.Value, prec)
		result := target + " = " + value
		if prec < parentPrecedence {
			return "(" + result + ")"
		}
		return result

	case *ListExpr:
		var elements []string
		for _, elem := range e.Elements {
			elements = append(elements, unparseExpr(elem, precedenceLowest))
		}
		return "{" + strings.Join(elements, ", ") + "}"

	case *ListRangeExpr:
		start := unparseExpr(e.Start, precedenceLowest)
		end := unparseExpr(e.End, precedenceLowest)
		return "{" + start + ".." + end + "}"

	case *MapExpr:
		var pairs []string
		for _, pair := range e.Pairs {
			key := unparseExpr(pair.Key, precedenceLowest)
			val := unparseExpr(pair.Value, precedenceLowest)
			pairs = append(pairs, key+" -> "+val)
		}
		return "[" + strings.Join(pairs, ", ") + "]"

	default:
		return fmt.Sprintf("<unknown expr: %T>", expr)
	}
}

// unparsePropertyExpr handles property access with #0.prop → $prop conversion
func unparsePropertyExpr(e *PropertyExpr) string {
	// Check if base is #0 (system object)
	if lit, ok := e.Expr.(*LiteralExpr); ok {
		if obj, ok := lit.Value.(types.ObjValue); ok {
			if obj.ID() == 0 {
				// Use $property syntax for system object
				return "$" + e.Property
			}
		}
	}

	// Otherwise use obj.property syntax
	base := unparseExpr(e.Expr, precedenceProperty)
	if e.Property != "" {
		return base + "." + e.Property
	}
	// Dynamic property
	return base + ".(" + unparseExpr(e.PropertyExpr, precedenceLowest) + ")"
}

// unparseBinaryExpr handles binary expressions with proper precedence
func unparseBinaryExpr(e *BinaryExpr, parentPrecedence int) string {
	prec := binaryPrecedence(e.Operator)
	left := unparseExpr(e.Left, prec)
	right := unparseExpr(e.Right, prec+1) // Right-associative for same precedence
	op := unparseBinaryOp(e.Operator)

	result := left + " " + op + " " + right

	if prec < parentPrecedence {
		return "(" + result + ")"
	}
	return result
}

// binaryPrecedence returns the precedence level for a binary operator
func binaryPrecedence(op TokenType) int {
	switch op {
	case TOKEN_ASSIGN:
		return precedenceAssign
	case TOKEN_OR:
		return precedenceOr
	case TOKEN_AND:
		return precedenceAnd
	case TOKEN_BITOR:
		return precedenceBitOr
	case TOKEN_BITXOR:
		return precedenceBitXor
	case TOKEN_BITAND:
		return precedenceBitAnd
	case TOKEN_EQ, TOKEN_NE:
		return precedenceEquality
	case TOKEN_LT, TOKEN_LE, TOKEN_GT, TOKEN_GE, TOKEN_IN:
		return precedenceComparison
	case TOKEN_LSHIFT, TOKEN_RSHIFT:
		return precedenceShift
	case TOKEN_PLUS, TOKEN_MINUS:
		return precedenceAdditive
	case TOKEN_STAR, TOKEN_SLASH, TOKEN_PERCENT:
		return precedenceMultiply
	case TOKEN_CARET:
		return precedenceExponent
	default:
		return precedenceLowest
	}
}

// unparseBinaryOp converts a token type to its string representation
func unparseBinaryOp(op TokenType) string {
	switch op {
	case TOKEN_PLUS:
		return "+"
	case TOKEN_MINUS:
		return "-"
	case TOKEN_STAR:
		return "*"
	case TOKEN_SLASH:
		return "/"
	case TOKEN_PERCENT:
		return "%"
	case TOKEN_CARET:
		return "^"
	case TOKEN_EQ:
		return "=="
	case TOKEN_NE:
		return "!="
	case TOKEN_LT:
		return "<"
	case TOKEN_GT:
		return ">"
	case TOKEN_LE:
		return "<="
	case TOKEN_GE:
		return ">="
	case TOKEN_AND:
		return "&&"
	case TOKEN_OR:
		return "||"
	case TOKEN_BITAND:
		return "&"
	case TOKEN_BITOR:
		return "|"
	case TOKEN_BITXOR:
		return "^"
	case TOKEN_LSHIFT:
		return "<<"
	case TOKEN_RSHIFT:
		return ">>"
	case TOKEN_IN:
		return "in"
	default:
		return "<unknown op>"
	}
}

// unparseUnaryOp converts a unary operator to its string representation
func unparseUnaryOp(op TokenType) string {
	switch op {
	case TOKEN_MINUS:
		return "-"
	case TOKEN_NOT:
		return "!"
	case TOKEN_BITNOT:
		return "~"
	default:
		return "<unknown unary op>"
	}
}

// unparseLiteral converts a Value to its source representation
func unparseLiteral(v types.Value) string {
	switch val := v.(type) {
	case types.IntValue:
		return strconv.Itoa(int(val.Val))
	case types.FloatValue:
		return fmt.Sprintf("%g", val.Val)
	case types.StrValue:
		// Need proper string escaping
		return strconv.Quote(val.Value())
	case types.ObjValue:
		return fmt.Sprintf("#%d", val.ID())
	case types.ErrValue:
		return val.Code().String()
	default:
		return v.String()
	}
}

// unparseArgs converts argument expressions to a comma-separated string
func unparseArgs(args []Expr) string {
	if len(args) == 0 {
		return ""
	}
	var parts []string
	for _, arg := range args {
		parts = append(parts, unparseExpr(arg, precedenceLowest))
	}
	return strings.Join(parts, ", ")
}
