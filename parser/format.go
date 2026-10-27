package parser

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/MongooseMoo/barn/ir"
)

// Non-binary precedence aliases. Binary precedence, spelling, and associativity
// all come from binaryOperatorSpecs.
const (
	precedenceLowest   = PREC_LOWEST
	precedenceAssign   = PREC_ASSIGNMENT
	precedenceTernary  = PREC_TERNARY
	precedenceUnary    = PREC_UNARY
	precedenceProperty = PREC_POSTFIX
)

// FormatMOO converts a semantic verb program back to MOO source lines.
// Invalid input returns nil; use FormatMOOChecked to obtain the error.
func FormatMOO(program *ir.Program) []string {
	lines, _ := formatMOOChecked(program, false)
	return lines
}

// FormatMOOFullyParenthesized emits Toast's fully-parenthesized decompile form.
// Invalid input returns nil; use FormatMOOFullyParenthesizedChecked for the error.
func FormatMOOFullyParenthesized(program *ir.Program) []string {
	lines, _ := formatMOOChecked(program, true)
	return lines
}

// FormatMOOChecked validates recursive formatter input before producing output.
// Canonical MOO source cannot contain NUL. Rejection returns nil source lines.
func FormatMOOChecked(program *ir.Program) ([]string, error) {
	return formatMOOChecked(program, false)
}

// FormatMOOFullyParenthesizedChecked returns complete source or an error.
func FormatMOOFullyParenthesizedChecked(program *ir.Program) ([]string, error) {
	return formatMOOChecked(program, true)
}

// FormattingError is a frontend representability failure. No output is returned
// when any nested formatter records one; Unwrap preserves its specific cause.
type FormattingError struct {
	Position ir.Position
	Cause    error
}

func (e *FormattingError) Error() string { return e.Cause.Error() }
func (e *FormattingError) Unwrap() error { return e.Cause }

type mooFormatter struct{ err error }

func (f *mooFormatter) fail(pos ir.Position, cause error) string {
	if f.err == nil {
		f.err = &FormattingError{Position: pos, Cause: cause}
	}
	return ""
}

func formatMOOChecked(program *ir.Program, fullyParenthesized bool) ([]string, error) {
	if err := ir.Validate(program); err != nil {
		return nil, err
	}
	if len(program.Statements) == 0 {
		return []string{}, nil
	}

	f := &mooFormatter{}
	var lines []string
	for _, stmt := range program.Statements {
		line := f.stmt(stmt, 0, fullyParenthesized)
		if strings.IndexByte(line, 0) >= 0 {
			return nil, &FormattingError{Position: stmt.Position(), Cause: ErrNULInSource}
		}
		if f.err != nil {
			return nil, f.err
		}
		lines = append(lines, strings.Split(line, "\n")...)
	}
	return lines, nil
}

// stmt converts a statement to source code
func (f *mooFormatter) stmt(stmt ir.Stmt, indent int, fullyParenthesized bool) string {
	indentStr := strings.Repeat("  ", indent)

	switch s := stmt.(type) {
	case *ir.EmptyStmt:
		return indentStr + ";"
	case *ir.ExprStmt:
		return indentStr + f.expr(s.Expr, precedenceLowest, fullyParenthesized) + ";"

	case *ir.ReturnStmt:
		if s.Value == nil {
			return indentStr + "return;"
		}
		return indentStr + "return " + f.expr(s.Value, precedenceLowest, fullyParenthesized) + ";"

	case *ir.IfStmt:
		var sb strings.Builder
		sb.WriteString(indentStr + "if (" + f.expr(s.Condition, precedenceLowest, fullyParenthesized) + ")\n")
		current := s
		for {
			for _, bodyStmt := range current.Body {
				sb.WriteString(f.stmt(bodyStmt, indent+1, fullyParenthesized) + "\n")
			}
			if len(current.Else) == 1 {
				if next, ok := current.Else[0].(*ir.IfStmt); ok {
					sb.WriteString(indentStr + "elseif (" + f.expr(next.Condition, precedenceLowest, fullyParenthesized) + ")\n")
					current = next
					continue
				}
			}
			if len(current.Else) > 0 {
				sb.WriteString(indentStr + "else\n")
				for _, bodyStmt := range current.Else {
					sb.WriteString(f.stmt(bodyStmt, indent+1, fullyParenthesized) + "\n")
				}
			}
			break
		}
		sb.WriteString(indentStr + "endif")
		return strings.TrimSuffix(sb.String(), "\n")

	case *ir.WhileStmt:
		var sb strings.Builder
		if s.Label != "" {
			sb.WriteString(indentStr + "while " + f.name(s.Pos, s.Label) + " (" + f.expr(s.Condition, precedenceLowest, fullyParenthesized) + ")\n")
		} else {
			sb.WriteString(indentStr + "while (" + f.expr(s.Condition, precedenceLowest, fullyParenthesized) + ")\n")
		}
		for _, bodyStmt := range s.Body {
			sb.WriteString(f.stmt(bodyStmt, indent+1, fullyParenthesized) + "\n")
		}
		sb.WriteString(indentStr + "endwhile")
		return strings.TrimSuffix(sb.String(), "\n")

	case *ir.CollectionLoopStmt:
		var sb strings.Builder
		sb.WriteString(indentStr + "for ")
		if s.Label != "" {
			sb.WriteString(f.name(s.Pos, s.Label) + " ")
		}
		if s.Index != "" {
			sb.WriteString(f.name(s.Pos, s.Value) + ", " + f.name(s.Pos, s.Index) + " in (" + f.expr(s.Collection, precedenceLowest, fullyParenthesized) + ")\n")
		} else {
			sb.WriteString(f.name(s.Pos, s.Value) + " in (" + f.expr(s.Collection, precedenceLowest, fullyParenthesized) + ")\n")
		}
		for _, bodyStmt := range s.Body {
			sb.WriteString(f.stmt(bodyStmt, indent+1, fullyParenthesized) + "\n")
		}
		sb.WriteString(indentStr + "endfor")
		return strings.TrimSuffix(sb.String(), "\n")

	case *ir.RangeLoopStmt:
		var sb strings.Builder
		sb.WriteString(indentStr + "for ")
		if s.Label != "" {
			sb.WriteString(f.name(s.Pos, s.Label) + " ")
		}
		sb.WriteString(f.name(s.Pos, s.Value) + " in [" + f.expr(s.Start, precedenceLowest, fullyParenthesized) + ".." + f.expr(s.End, precedenceLowest, fullyParenthesized) + "]\n")
		for _, bodyStmt := range s.Body {
			sb.WriteString(f.stmt(bodyStmt, indent+1, fullyParenthesized) + "\n")
		}
		sb.WriteString(indentStr + "endfor")
		return strings.TrimSuffix(sb.String(), "\n")

	case *ir.BreakStmt:
		if s.Label != "" {
			return indentStr + "break " + f.name(s.Pos, s.Label) + ";"
		}
		return indentStr + "break;"

	case *ir.ContinueStmt:
		if s.Label != "" {
			return indentStr + "continue " + f.name(s.Pos, s.Label) + ";"
		}
		return indentStr + "continue;"

	case *ir.TryStmt:
		var sb strings.Builder
		sb.WriteString(indentStr + "try\n")
		for _, bodyStmt := range s.Body {
			sb.WriteString(f.stmt(bodyStmt, indent+1, fullyParenthesized) + "\n")
		}
		for _, handler := range s.Handlers {
			sb.WriteString(indentStr + "except ")
			if handler.Variable != "" {
				sb.WriteString(f.name(handler.Pos, handler.Variable) + " ")
			}
			sb.WriteString("(")
			if handler.IsAny {
				sb.WriteString("ANY")
			} else {
				for i, code := range handler.Codes {
					if i > 0 {
						sb.WriteString(", ")
					}
					sb.WriteString(f.errorName(handler.Pos, code))
				}
			}
			sb.WriteString(")\n")
			for _, bodyStmt := range handler.Body {
				sb.WriteString(f.stmt(bodyStmt, indent+1, fullyParenthesized) + "\n")
			}
		}
		if s.Finalizer != nil {
			sb.WriteString(indentStr + "finally\n")
			for _, bodyStmt := range s.Finalizer.Body {
				sb.WriteString(f.stmt(bodyStmt, indent+1, fullyParenthesized) + "\n")
			}
		}
		sb.WriteString(indentStr + "endtry")
		return strings.TrimSuffix(sb.String(), "\n")

	case *ir.ForkStmt:
		var sb strings.Builder
		sb.WriteString(indentStr + "fork ")
		if s.VarName != "" {
			sb.WriteString(f.name(s.Pos, s.VarName) + " ")
		}
		sb.WriteString("(" + f.expr(s.Delay, precedenceLowest, fullyParenthesized) + ")\n")
		for _, bodyStmt := range s.Body {
			sb.WriteString(f.stmt(bodyStmt, indent+1, fullyParenthesized) + "\n")
		}
		sb.WriteString(indentStr + "endfork")
		return strings.TrimSuffix(sb.String(), "\n")

	default:
		return f.fail(ir.Position{}, fmt.Errorf("unsupported statement %T", stmt))
	}
}

// expr converts an expression to source code
func (f *mooFormatter) expr(expr ir.Expr, parentPrecedence int, fullyParenthesized bool) string {
	switch e := expr.(type) {
	case *ir.LiteralExpr:
		return f.literal(e)

	case *ir.IdentifierExpr:
		return canonicalIdentifierName(f.name(e.Pos, e.Name))

	case *ir.UnaryExpr:
		op := f.unaryOp(e.Operator)
		operand := f.expr(e.Operand, precedenceUnary, fullyParenthesized)
		result := op + operand
		if fullyParenthesized && parentPrecedence != precedenceLowest {
			return "(" + result + ")"
		}
		return result

	case *ir.BinaryExpr:
		return f.binary(e, parentPrecedence, fullyParenthesized)

	case *ir.TernaryExpr:
		prec := precedenceTernary
		cond := f.expr(e.Condition, prec+1, fullyParenthesized)
		then := f.expr(e.ThenExpr, prec+1, fullyParenthesized)
		els := f.expr(e.ElseExpr, prec+1, fullyParenthesized)
		result := cond + " ? " + then + " | " + els
		if prec < parentPrecedence {
			return "(" + result + ")"
		}
		return result

	case *ir.IndexBoundaryExpr:
		if e.Boundary == ir.IndexFirst {
			return "^"
		}
		return "$"

	case *ir.IndexExpr:
		base := f.expr(e.Expr, precedenceProperty, fullyParenthesized)
		index := f.expr(e.Index, precedenceLowest, fullyParenthesized)
		return base + "[" + index + "]"

	case *ir.RangeExpr:
		base := f.expr(e.Expr, precedenceProperty, fullyParenthesized)
		start := f.expr(e.Start, precedenceLowest, fullyParenthesized)
		end := f.expr(e.End, precedenceLowest, fullyParenthesized)
		// NO spaces around ..
		return base + "[" + start + ".." + end + "]"

	case *ir.PropertyExpr:
		return f.property(e, fullyParenthesized)

	case *ir.VerbCallExpr:
		base := f.expr(e.Expr, precedenceProperty, fullyParenthesized)
		var verb string
		if e.Verb != "" {
			verb = f.name(e.Pos, e.Verb)
		} else {
			verb = "(" + f.expr(e.VerbExpr, precedenceLowest, fullyParenthesized) + ")"
		}
		args := f.args(e.Args, fullyParenthesized)
		return base + ":" + verb + "(" + args + ")"

	case *ir.BuiltinCallExpr:
		args := f.args(e.Args, fullyParenthesized)
		return f.name(e.Pos, e.Name) + "(" + args + ")"

	case *ir.SpliceExpr:
		return "@" + f.expr(e.Expr, precedenceUnary, fullyParenthesized)

	case *ir.CatchExpr:
		result := "`" + f.expr(e.Expr, precedenceTernary, fullyParenthesized)
		result += " ! "
		if e.IsAny {
			result += "ANY"
		} else {
			for i, code := range e.Codes {
				if i > 0 {
					result += ", "
				}
				result += f.errorName(e.Pos, code)
			}
		}
		if e.Default != nil {
			result += " => " + f.expr(e.Default, precedenceTernary, fullyParenthesized)
		}
		return result + "'"

	case *ir.AssignExpr:
		prec := precedenceAssign
		target := f.target(e.Target, fullyParenthesized)
		value := f.expr(e.Value, prec, fullyParenthesized)
		result := target + " = " + value
		if prec < parentPrecedence {
			return "(" + result + ")"
		}
		return result

	case *ir.ListExpr:
		var elements []string
		for _, elem := range e.Elements {
			elements = append(elements, f.expr(elem, precedenceLowest, fullyParenthesized))
		}
		return "{" + strings.Join(elements, ", ") + "}"

	case *ir.ListRangeExpr:
		start := f.expr(e.Start, precedenceLowest, fullyParenthesized)
		end := f.expr(e.End, precedenceLowest, fullyParenthesized)
		return "{" + start + ".." + end + "}"

	case *ir.MapExpr:
		var pairs []string
		for _, pair := range e.Pairs {
			key := f.expr(pair.Key, precedenceLowest, fullyParenthesized)
			val := f.expr(pair.Value, precedenceLowest, fullyParenthesized)
			pairs = append(pairs, key+" -> "+val)
		}
		return "[" + strings.Join(pairs, ", ") + "]"

	default:
		return f.fail(ir.Position{}, fmt.Errorf("unsupported expression %T", expr))
	}
}

// property handles property access with #0.prop → $prop conversion
func (f *mooFormatter) property(e *ir.PropertyExpr, fullyParenthesized bool) string {
	// Check if base is #0 (system object)
	if lit, ok := e.Expr.(*ir.LiteralExpr); ok {
		if lit.Kind == ir.LiteralObj && lit.ObjID == 0 && e.Property != "" {
			// Use $property syntax for system object
			return "$" + f.name(e.Pos, e.Property)
		}
	}

	// Otherwise use obj.property syntax
	base := f.expr(e.Expr, precedenceProperty, fullyParenthesized)
	if e.Property != "" {
		return base + "." + f.name(e.Pos, e.Property)
	}
	// Dynamic property
	return base + ".(" + f.expr(e.PropertyExpr, precedenceLowest, fullyParenthesized) + ")"
}

// binary handles binary expressions with proper precedence
func (f *mooFormatter) binary(e *ir.BinaryExpr, parentPrecedence int, fullyParenthesized bool) string {
	spec, ok := binaryOperatorBySemantic(e.Operator)
	if !ok {
		return f.fail(e.Pos, fmt.Errorf("unsupported binary operator %d", e.Operator))
	}
	leftPrecedence, rightPrecedence := spec.precedence, spec.precedence+1
	if spec.associativity == associateRight {
		leftPrecedence, rightPrecedence = spec.precedence+1, spec.precedence
	}
	left := f.expr(e.Left, leftPrecedence, fullyParenthesized)
	right := f.expr(e.Right, rightPrecedence, fullyParenthesized)

	result := left + " " + spec.spelling + " " + right

	if spec.precedence < parentPrecedence || (fullyParenthesized && parentPrecedence != precedenceLowest) {
		return "(" + result + ")"
	}
	return result
}

// unaryOp converts a unary operator to its string representation
func (f *mooFormatter) unaryOp(op ir.UnaryOperator) string {
	switch op {
	case ir.UnaryNegate:
		return "-"
	case ir.UnaryNot:
		return "!"
	case ir.UnaryBitwiseNot:
		return "~"
	default:
		return f.fail(ir.Position{}, fmt.Errorf("unsupported unary operator %d", op))
	}
}

// target formats the sealed assignment target family.
func (f *mooFormatter) target(target ir.Target, fullyParenthesized bool) string {
	switch target := target.(type) {
	case *ir.VariableTarget:
		return canonicalIdentifierName(f.name(target.Pos, target.Name))
	case *ir.PropertyTarget:
		object := f.expr(target.Object, precedenceProperty, fullyParenthesized)
		if target.Name != "" {
			return object + "." + f.name(target.Pos, target.Name)
		}
		return object + ".(" + f.expr(target.NameExpr, precedenceLowest, fullyParenthesized) + ")"
	case *ir.IndexTarget:
		return f.target(target.Collection, fullyParenthesized) + "[" + f.expr(target.Index, precedenceLowest, fullyParenthesized) + "]"
	case *ir.RangeTarget:
		return f.target(target.Collection, fullyParenthesized) + "[" + f.expr(target.Start, precedenceLowest, fullyParenthesized) + ".." + f.expr(target.End, precedenceLowest, fullyParenthesized) + "]"
	case *ir.DestructuringTarget:
		bindings := make([]string, len(target.Bindings))
		for i, binding := range target.Bindings {
			switch binding := binding.(type) {
			case *ir.RequiredBinding:
				bindings[i] = f.name(binding.Pos, binding.Name)
			case *ir.OptionalBinding:
				bindings[i] = "?" + f.name(binding.Pos, binding.Name)
				if binding.Default != nil {
					bindings[i] += " = " + f.expr(binding.Default, precedenceLowest, fullyParenthesized)
				}
			case *ir.RestBinding:
				bindings[i] = "@" + f.name(binding.Pos, binding.Name)
			default:
				return f.fail(ir.Position{}, fmt.Errorf("unsupported binding %T", binding))
			}
		}
		return "{" + strings.Join(bindings, ", ") + "}"
	default:
		return f.fail(ir.Position{}, fmt.Errorf("unsupported target %T", target))
	}
}

func canonicalIdentifierName(name string) string {
	upper := strings.ToUpper(name)
	switch upper {
	case "INT", "NUM", "OBJ", "STR", "ERR", "LIST", "FLOAT", "MAP", "ANON", "WAIF", "BOOL":
		return upper
	default:
		return name
	}
}

func (f *mooFormatter) literal(v *ir.LiteralExpr) string {
	switch v.Kind {
	case ir.LiteralInt:
		return strconv.FormatInt(v.IntValue, 10)
	case ir.LiteralFloat:
		if math.IsInf(v.FloatValue, 0) || math.IsNaN(v.FloatValue) {
			return f.fail(v.Pos, fmt.Errorf("nonfinite float has no MOO literal spelling"))
		}
		formatted := strconv.FormatFloat(v.FloatValue, 'f', -1, 64)
		if !strings.ContainsAny(formatted, ".eE") {
			formatted += ".0"
		}
		return formatted
	case ir.LiteralString:
		if strings.IndexByte(v.StringValue, 0) >= 0 {
			return f.fail(v.Pos, ErrNULInSource)
		}
		return quoteMOOString(v.StringValue)
	case ir.LiteralBool:
		if v.BoolValue {
			return "true"
		}
		return "false"
	case ir.LiteralObj:
		return fmt.Sprintf("#%d", v.ObjID)
	case ir.LiteralErr:
		return f.errorName(v.Pos, v.ErrorName)
	default:
		return f.fail(v.Pos, fmt.Errorf("unsupported literal kind %d", v.Kind))
	}
}

// name keeps MOO lexical representability in the frontend. Semantic validation
// requires a name but does not embed this language-specific token grammar.
func (f *mooFormatter) name(pos ir.Position, name string) string {
	if strings.IndexByte(name, 0) >= 0 {
		return f.fail(pos, ErrNULInSource)
	}
	lexer := NewLexer(name)
	token := lexer.NextToken()
	if token.Type != TOKEN_IDENTIFIER || token.Value != name || lexer.NextToken().Type != TOKEN_EOF {
		return f.fail(pos, fmt.Errorf("name %q has no MOO identifier spelling", name))
	}
	return name
}

func (f *mooFormatter) errorName(pos ir.Position, name string) string {
	if strings.IndexByte(name, 0) >= 0 {
		return f.fail(pos, ErrNULInSource)
	}
	if !isErrorName(name) {
		return f.fail(pos, fmt.Errorf("unknown MOO error name %q", name))
	}
	return name
}

// quoteMOOString emits a string literal using MOO's escape rules. A backslash
// only quotes the byte immediately following it, so only quotes and backslashes
// need escaping; representable non-NUL bytes are preserved verbatim. Public
// formatter paths reject NUL before returning any source.
func quoteMOOString(value string) string {
	var quoted strings.Builder
	quoted.Grow(len(value) + 2)
	quoted.WriteByte('"')
	for i := 0; i < len(value); i++ {
		if value[i] == '"' || value[i] == '\\' {
			quoted.WriteByte('\\')
		}
		quoted.WriteByte(value[i])
	}
	quoted.WriteByte('"')
	return quoted.String()
}

// args converts argument expressions to a comma-separated string
func (f *mooFormatter) args(args []ir.Expr, fullyParenthesized bool) string {
	if len(args) == 0 {
		return ""
	}
	var parts []string
	for _, arg := range args {
		parts = append(parts, f.expr(arg, precedenceLowest, fullyParenthesized))
	}
	return strings.Join(parts, ", ")
}
