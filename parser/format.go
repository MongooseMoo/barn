package parser

import (
	"barn/verb"
	"fmt"
	"strconv"
	"strings"
)

// Operator precedence levels (higher = tighter binding)
const (
	precedenceLowest     = iota
	precedenceAssign     // =
	precedenceTernary    // ? |
	precedenceOr         // ||
	precedenceAnd        // &&
	precedenceBitOr      // |
	precedenceBitXor     // ^
	precedenceBitAnd     // &
	precedenceEquality   // == !=
	precedenceComparison // < <= > >= in
	precedenceShift      // << >>
	precedenceAdditive   // + -
	precedenceMultiply   // * / %
	precedenceExponent   // ^
	precedenceUnary      // - ! ~
	precedenceProperty   // . : [] (highest - property access, verb call, index)
)

// FormatMOO converts a semantic verb program back to MOO source lines.
func FormatMOO(program *verb.Program) []string {
	if len(program.Statements) == 0 {
		return []string{}
	}

	var lines []string
	for _, stmt := range program.Statements {
		line := unparseStmt(stmt, 0)
		lines = append(lines, strings.Split(line, "\n")...)
	}
	return lines
}

// unparseStmt converts a statement to source code
func unparseStmt(stmt verb.Stmt, indent int) string {
	indentStr := strings.Repeat("  ", indent)

	switch s := stmt.(type) {
	case *verb.ExprStmt:
		if s.Expr == nil {
			return indentStr + ";"
		}
		return indentStr + unparseExpr(s.Expr, precedenceLowest) + ";"

	case *verb.ReturnStmt:
		if s.Value == nil {
			return indentStr + "return;"
		}
		return indentStr + "return " + unparseExpr(s.Value, precedenceLowest) + ";"

	case *verb.IfStmt:
		var sb strings.Builder
		sb.WriteString(indentStr + "if (" + unparseExpr(s.Condition, precedenceLowest) + ")\n")
		for _, bodyStmt := range s.Body {
			sb.WriteString(unparseStmt(bodyStmt, indent+1) + "\n")
		}
		if len(s.Else) > 0 {
			sb.WriteString(indentStr + "else\n")
			for _, bodyStmt := range s.Else {
				sb.WriteString(unparseStmt(bodyStmt, indent+1) + "\n")
			}
		}
		sb.WriteString(indentStr + "endif")
		return strings.TrimSuffix(sb.String(), "\n")

	case *verb.WhileStmt:
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

	case *verb.CollectionLoopStmt:
		var sb strings.Builder
		sb.WriteString(indentStr + "for ")
		if s.Label != "" {
			sb.WriteString(s.Label + " ")
		}
		if s.Index != "" {
			sb.WriteString(s.Value + ", " + s.Index + " in (" + unparseExpr(s.Collection, precedenceLowest) + ")\n")
		} else {
			sb.WriteString(s.Value + " in (" + unparseExpr(s.Collection, precedenceLowest) + ")\n")
		}
		for _, bodyStmt := range s.Body {
			sb.WriteString(unparseStmt(bodyStmt, indent+1) + "\n")
		}
		sb.WriteString(indentStr + "endfor")
		return strings.TrimSuffix(sb.String(), "\n")

	case *verb.RangeLoopStmt:
		var sb strings.Builder
		sb.WriteString(indentStr + "for ")
		if s.Label != "" {
			sb.WriteString(s.Label + " ")
		}
		sb.WriteString(s.Value + " in [" + unparseExpr(s.Start, precedenceLowest) + ".." + unparseExpr(s.End, precedenceLowest) + "]\n")
		for _, bodyStmt := range s.Body {
			sb.WriteString(unparseStmt(bodyStmt, indent+1) + "\n")
		}
		sb.WriteString(indentStr + "endfor")
		return strings.TrimSuffix(sb.String(), "\n")

	case *verb.BreakStmt:
		if s.Label != "" {
			return indentStr + "break " + s.Label + ";"
		}
		return indentStr + "break;"

	case *verb.ContinueStmt:
		if s.Label != "" {
			return indentStr + "continue " + s.Label + ";"
		}
		return indentStr + "continue;"

	case *verb.TryStmt:
		var sb strings.Builder
		sb.WriteString(indentStr + "try\n")
		for _, bodyStmt := range s.Body {
			sb.WriteString(unparseStmt(bodyStmt, indent+1) + "\n")
		}
		for _, handler := range s.Handlers {
			sb.WriteString(indentStr + "except ")
			if handler.Variable != "" {
				sb.WriteString(handler.Variable + " ")
			}
			sb.WriteString("(")
			if handler.IsAny {
				sb.WriteString("ANY")
			} else {
				for i, code := range handler.Codes {
					if i > 0 {
						sb.WriteString(", ")
					}
					sb.WriteString(code)
				}
			}
			sb.WriteString(")\n")
			for _, bodyStmt := range handler.Body {
				sb.WriteString(unparseStmt(bodyStmt, indent+1) + "\n")
			}
		}
		if s.Finalizer != nil {
			sb.WriteString(indentStr + "finally\n")
			for _, bodyStmt := range s.Finalizer.Body {
				sb.WriteString(unparseStmt(bodyStmt, indent+1) + "\n")
			}
		}
		sb.WriteString(indentStr + "endtry")
		return strings.TrimSuffix(sb.String(), "\n")

	case *verb.ForkStmt:
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
func unparseExpr(expr verb.Expr, parentPrecedence int) string {
	switch e := expr.(type) {
	case *verb.LiteralExpr:
		return unparseLiteral(e)

	case *verb.IdentifierExpr:
		return e.Name

	case *verb.UnaryExpr:
		op := unparseUnaryOp(e.Operator)
		operand := unparseExpr(e.Operand, precedenceUnary)
		return op + operand

	case *verb.BinaryExpr:
		return unparseBinaryExpr(e, parentPrecedence)

	case *verb.TernaryExpr:
		prec := precedenceTernary
		cond := unparseExpr(e.Condition, prec+1)
		then := unparseExpr(e.ThenExpr, prec+1)
		els := unparseExpr(e.ElseExpr, prec+1)
		result := cond + " ? " + then + " | " + els
		if prec < parentPrecedence {
			return "(" + result + ")"
		}
		return result

	case *verb.IndexBoundaryExpr:
		if e.Boundary == verb.IndexFirst {
			return "^"
		}
		return "$"

	case *verb.IndexExpr:
		base := unparseExpr(e.Expr, precedenceProperty)
		index := unparseExpr(e.Index, precedenceLowest)
		return base + "[" + index + "]"

	case *verb.RangeExpr:
		base := unparseExpr(e.Expr, precedenceProperty)
		start := unparseExpr(e.Start, precedenceLowest)
		end := unparseExpr(e.End, precedenceLowest)
		// NO spaces around ..
		return base + "[" + start + ".." + end + "]"

	case *verb.PropertyExpr:
		return unparsePropertyExpr(e)

	case *verb.VerbCallExpr:
		base := unparseExpr(e.Expr, precedenceProperty)
		var verb string
		if e.Verb != "" {
			verb = e.Verb
		} else {
			verb = "(" + unparseExpr(e.VerbExpr, precedenceLowest) + ")"
		}
		args := unparseArgs(e.Args)
		return base + ":" + verb + "(" + args + ")"

	case *verb.BuiltinCallExpr:
		args := unparseArgs(e.Args)
		return e.Name + "(" + args + ")"

	case *verb.SpliceExpr:
		return "@" + unparseExpr(e.Expr, precedenceUnary)

	case *verb.CatchExpr:
		result := "`" + unparseExpr(e.Expr, precedenceTernary)
		result += " ! "
		if e.IsAny {
			result += "ANY"
		} else {
			for i, code := range e.Codes {
				if i > 0 {
					result += ", "
				}
				result += code
			}
		}
		if e.Default != nil {
			result += " => " + unparseExpr(e.Default, precedenceTernary)
		}
		return result + "'"

	case *verb.AssignExpr:
		prec := precedenceAssign
		target := unparseTarget(e.Target)
		value := unparseExpr(e.Value, prec)
		result := target + " = " + value
		if prec < parentPrecedence {
			return "(" + result + ")"
		}
		return result

	case *verb.ListExpr:
		var elements []string
		for _, elem := range e.Elements {
			elements = append(elements, unparseExpr(elem, precedenceLowest))
		}
		return "{" + strings.Join(elements, ", ") + "}"

	case *verb.ListRangeExpr:
		start := unparseExpr(e.Start, precedenceLowest)
		end := unparseExpr(e.End, precedenceLowest)
		return "{" + start + ".." + end + "}"

	case *verb.MapExpr:
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
func unparsePropertyExpr(e *verb.PropertyExpr) string {
	// Check if base is #0 (system object)
	if lit, ok := e.Expr.(*verb.LiteralExpr); ok {
		if lit.Kind == verb.LiteralObj && lit.ObjID == 0 && e.Property != "" {
			// Use $property syntax for system object
			return "$" + e.Property
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
func unparseBinaryExpr(e *verb.BinaryExpr, parentPrecedence int) string {
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
func binaryPrecedence(op verb.BinaryOperator) int {
	switch op {
	case verb.BinaryOr:
		return precedenceOr
	case verb.BinaryAnd:
		return precedenceAnd
	case verb.BinaryBitOr:
		return precedenceBitOr
	case verb.BinaryBitXor:
		return precedenceBitXor
	case verb.BinaryBitAnd:
		return precedenceBitAnd
	case verb.BinaryEqual, verb.BinaryNotEqual:
		return precedenceEquality
	case verb.BinaryLess, verb.BinaryLessEqual, verb.BinaryGreater, verb.BinaryGreaterEqual, verb.BinaryIn:
		return precedenceComparison
	case verb.BinaryShiftLeft, verb.BinaryShiftRight:
		return precedenceShift
	case verb.BinaryAdd, verb.BinarySubtract:
		return precedenceAdditive
	case verb.BinaryMultiply, verb.BinaryDivide, verb.BinaryModulo:
		return precedenceMultiply
	case verb.BinaryPower:
		return precedenceExponent
	default:
		return precedenceLowest
	}
}

// unparseBinaryOp converts a semantic operator to MOO spelling.
func unparseBinaryOp(op verb.BinaryOperator) string {
	switch op {
	case verb.BinaryAdd:
		return "+"
	case verb.BinarySubtract:
		return "-"
	case verb.BinaryMultiply:
		return "*"
	case verb.BinaryDivide:
		return "/"
	case verb.BinaryModulo:
		return "%"
	case verb.BinaryPower:
		return "^"
	case verb.BinaryEqual:
		return "=="
	case verb.BinaryNotEqual:
		return "!="
	case verb.BinaryLess:
		return "<"
	case verb.BinaryGreater:
		return ">"
	case verb.BinaryLessEqual:
		return "<="
	case verb.BinaryGreaterEqual:
		return ">="
	case verb.BinaryAnd:
		return "&&"
	case verb.BinaryOr:
		return "||"
	case verb.BinaryBitAnd:
		return "&."
	case verb.BinaryBitOr:
		return "|"
	case verb.BinaryBitXor:
		return "^"
	case verb.BinaryShiftLeft:
		return "<<"
	case verb.BinaryShiftRight:
		return ">>"
	case verb.BinaryIn:
		return "in"
	default:
		return "<unknown op>"
	}
}

// unparseUnaryOp converts a unary operator to its string representation
func unparseUnaryOp(op verb.UnaryOperator) string {
	switch op {
	case verb.UnaryNegate:
		return "-"
	case verb.UnaryNot:
		return "!"
	case verb.UnaryBitwiseNot:
		return "~"
	default:
		return "<unknown unary op>"
	}
}

// unparseLiteral converts a literal syntax node to source representation.
func unparseTarget(target verb.Target) string {
	switch target := target.(type) {
	case *verb.VariableTarget:
		return target.Name
	case *verb.PropertyTarget:
		object := unparseExpr(target.Object, precedenceProperty)
		if target.Name != "" {
			return object + "." + target.Name
		}
		return object + ".(" + unparseExpr(target.NameExpr, precedenceLowest) + ")"
	case *verb.IndexTarget:
		return unparseTarget(target.Collection) + "[" + unparseExpr(target.Index, precedenceLowest) + "]"
	case *verb.RangeTarget:
		return unparseTarget(target.Collection) + "[" + unparseExpr(target.Start, precedenceLowest) + ".." + unparseExpr(target.End, precedenceLowest) + "]"
	case *verb.DestructuringTarget:
		bindings := make([]string, len(target.Bindings))
		for i, binding := range target.Bindings {
			switch binding := binding.(type) {
			case *verb.RequiredBinding:
				bindings[i] = binding.Name
			case *verb.OptionalBinding:
				bindings[i] = "?" + binding.Name
				if binding.Default != nil {
					bindings[i] += " = " + unparseExpr(binding.Default, precedenceLowest)
				}
			case *verb.RestBinding:
				bindings[i] = "@" + binding.Name
			}
		}
		return "{" + strings.Join(bindings, ", ") + "}"
	default:
		return "<unknown target>"
	}
}

func unparseLiteral(v *verb.LiteralExpr) string {
	switch v.Kind {
	case verb.LiteralInt:
		return strconv.FormatInt(v.IntValue, 10)
	case verb.LiteralFloat:
		formatted := strconv.FormatFloat(v.FloatValue, 'g', -1, 64)
		if !strings.ContainsAny(formatted, ".eE") {
			formatted += ".0"
		}
		return formatted
	case verb.LiteralString:
		// Need proper string escaping
		return strconv.Quote(v.StringValue)
	case verb.LiteralBool:
		if v.BoolValue {
			return "true"
		}
		return "false"
	case verb.LiteralObj:
		return fmt.Sprintf("#%d", v.ObjID)
	case verb.LiteralErr:
		return v.ErrorName
	default:
		return "<unknown literal>"
	}
}

// unparseArgs converts argument expressions to a comma-separated string
func unparseArgs(args []verb.Expr) string {
	if len(args) == 0 {
		return ""
	}
	var parts []string
	for _, arg := range args {
		parts = append(parts, unparseExpr(arg, precedenceLowest))
	}
	return strings.Join(parts, ", ")
}
