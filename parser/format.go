package parser

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/MongooseMoo/barn/verb"
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
func FormatMOO(program *verb.Program) []string {
	lines, _ := formatMOOChecked(program, false)
	return lines
}

// FormatMOOFullyParenthesized emits Toast's fully-parenthesized decompile form.
// Invalid input returns nil; use FormatMOOFullyParenthesizedChecked for the error.
func FormatMOOFullyParenthesized(program *verb.Program) []string {
	lines, _ := formatMOOChecked(program, true)
	return lines
}

// FormatMOOChecked validates recursive formatter input before producing output.
// Canonical MOO source cannot contain NUL. Rejection returns nil source lines.
func FormatMOOChecked(program *verb.Program) ([]string, error) {
	return formatMOOChecked(program, false)
}

// FormatMOOFullyParenthesizedChecked returns complete source or an error.
func FormatMOOFullyParenthesizedChecked(program *verb.Program) ([]string, error) {
	return formatMOOChecked(program, true)
}

// FormattingError is a frontend representability failure. No output is returned
// when any nested formatter records one; Unwrap preserves its specific cause.
type FormattingError struct {
	Position verb.Position
	Cause    error
}

func (e *FormattingError) Error() string { return e.Cause.Error() }
func (e *FormattingError) Unwrap() error { return e.Cause }

type mooFormatter struct{ err error }

func (f *mooFormatter) fail(pos verb.Position, cause error) string {
	if f.err == nil {
		f.err = &FormattingError{Position: pos, Cause: cause}
	}
	return ""
}

func formatMOOChecked(program *verb.Program, fullyParenthesized bool) ([]string, error) {
	if err := verb.Validate(program); err != nil {
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
func (f *mooFormatter) stmt(stmt verb.Stmt, indent int, fullyParenthesized bool) string {
	indentStr := strings.Repeat("  ", indent)

	switch s := stmt.(type) {
	case *verb.EmptyStmt:
		return indentStr + ";"
	case *verb.ExprStmt:
		return indentStr + f.expr(s.Expr, precedenceLowest, fullyParenthesized) + ";"

	case *verb.ReturnStmt:
		if s.Value == nil {
			return indentStr + "return;"
		}
		return indentStr + "return " + f.expr(s.Value, precedenceLowest, fullyParenthesized) + ";"

	case *verb.IfStmt:
		var sb strings.Builder
		sb.WriteString(indentStr + "if (" + f.expr(s.Condition, precedenceLowest, fullyParenthesized) + ")\n")
		current := s
		for {
			for _, bodyStmt := range current.Body {
				sb.WriteString(f.stmt(bodyStmt, indent+1, fullyParenthesized) + "\n")
			}
			if len(current.Else) == 1 {
				if next, ok := current.Else[0].(*verb.IfStmt); ok {
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

	case *verb.WhileStmt:
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

	case *verb.CollectionLoopStmt:
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

	case *verb.RangeLoopStmt:
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

	case *verb.BreakStmt:
		if s.Label != "" {
			return indentStr + "break " + f.name(s.Pos, s.Label) + ";"
		}
		return indentStr + "break;"

	case *verb.ContinueStmt:
		if s.Label != "" {
			return indentStr + "continue " + f.name(s.Pos, s.Label) + ";"
		}
		return indentStr + "continue;"

	case *verb.TryStmt:
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

	case *verb.ForkStmt:
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
		return f.fail(verb.Position{}, fmt.Errorf("unsupported statement %T", stmt))
	}
}

// expr converts an expression to source code
func (f *mooFormatter) expr(expr verb.Expr, parentPrecedence int, fullyParenthesized bool) string {
	switch e := expr.(type) {
	case *verb.LiteralExpr:
		return f.literal(e)

	case *verb.IdentifierExpr:
		return canonicalIdentifierName(f.name(e.Pos, e.Name))

	case *verb.UnaryExpr:
		op := f.unaryOp(e.Operator)
		operand := f.expr(e.Operand, precedenceUnary, fullyParenthesized)
		result := op + operand
		if fullyParenthesized && parentPrecedence != precedenceLowest {
			return "(" + result + ")"
		}
		return result

	case *verb.BinaryExpr:
		return f.binary(e, parentPrecedence, fullyParenthesized)

	case *verb.TernaryExpr:
		prec := precedenceTernary
		cond := f.expr(e.Condition, prec+1, fullyParenthesized)
		then := f.expr(e.ThenExpr, prec+1, fullyParenthesized)
		els := f.expr(e.ElseExpr, prec+1, fullyParenthesized)
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
		base := f.expr(e.Expr, precedenceProperty, fullyParenthesized)
		index := f.expr(e.Index, precedenceLowest, fullyParenthesized)
		return base + "[" + index + "]"

	case *verb.RangeExpr:
		base := f.expr(e.Expr, precedenceProperty, fullyParenthesized)
		start := f.expr(e.Start, precedenceLowest, fullyParenthesized)
		end := f.expr(e.End, precedenceLowest, fullyParenthesized)
		// NO spaces around ..
		return base + "[" + start + ".." + end + "]"

	case *verb.PropertyExpr:
		return f.property(e, fullyParenthesized)

	case *verb.VerbCallExpr:
		base := f.expr(e.Expr, precedenceProperty, fullyParenthesized)
		var verb string
		if e.Verb != "" {
			verb = f.name(e.Pos, e.Verb)
		} else {
			verb = "(" + f.expr(e.VerbExpr, precedenceLowest, fullyParenthesized) + ")"
		}
		args := f.args(e.Args, fullyParenthesized)
		return base + ":" + verb + "(" + args + ")"

	case *verb.BuiltinCallExpr:
		args := f.args(e.Args, fullyParenthesized)
		return f.name(e.Pos, e.Name) + "(" + args + ")"

	case *verb.SpliceExpr:
		return "@" + f.expr(e.Expr, precedenceUnary, fullyParenthesized)

	case *verb.CatchExpr:
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

	case *verb.AssignExpr:
		prec := precedenceAssign
		target := f.target(e.Target, fullyParenthesized)
		value := f.expr(e.Value, prec, fullyParenthesized)
		result := target + " = " + value
		if prec < parentPrecedence {
			return "(" + result + ")"
		}
		return result

	case *verb.ListExpr:
		var elements []string
		for _, elem := range e.Elements {
			elements = append(elements, f.expr(elem, precedenceLowest, fullyParenthesized))
		}
		return "{" + strings.Join(elements, ", ") + "}"

	case *verb.ListRangeExpr:
		start := f.expr(e.Start, precedenceLowest, fullyParenthesized)
		end := f.expr(e.End, precedenceLowest, fullyParenthesized)
		return "{" + start + ".." + end + "}"

	case *verb.MapExpr:
		var pairs []string
		for _, pair := range e.Pairs {
			key := f.expr(pair.Key, precedenceLowest, fullyParenthesized)
			val := f.expr(pair.Value, precedenceLowest, fullyParenthesized)
			pairs = append(pairs, key+" -> "+val)
		}
		return "[" + strings.Join(pairs, ", ") + "]"

	default:
		return f.fail(verb.Position{}, fmt.Errorf("unsupported expression %T", expr))
	}
}

// property handles property access with #0.prop → $prop conversion
func (f *mooFormatter) property(e *verb.PropertyExpr, fullyParenthesized bool) string {
	// Check if base is #0 (system object)
	if lit, ok := e.Expr.(*verb.LiteralExpr); ok {
		if lit.Kind == verb.LiteralObj && lit.ObjID == 0 && e.Property != "" {
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
func (f *mooFormatter) binary(e *verb.BinaryExpr, parentPrecedence int, fullyParenthesized bool) string {
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
func (f *mooFormatter) unaryOp(op verb.UnaryOperator) string {
	switch op {
	case verb.UnaryNegate:
		return "-"
	case verb.UnaryNot:
		return "!"
	case verb.UnaryBitwiseNot:
		return "~"
	default:
		return f.fail(verb.Position{}, fmt.Errorf("unsupported unary operator %d", op))
	}
}

// target formats the sealed assignment target family.
func (f *mooFormatter) target(target verb.Target, fullyParenthesized bool) string {
	switch target := target.(type) {
	case *verb.VariableTarget:
		return canonicalIdentifierName(f.name(target.Pos, target.Name))
	case *verb.PropertyTarget:
		object := f.expr(target.Object, precedenceProperty, fullyParenthesized)
		if target.Name != "" {
			return object + "." + f.name(target.Pos, target.Name)
		}
		return object + ".(" + f.expr(target.NameExpr, precedenceLowest, fullyParenthesized) + ")"
	case *verb.IndexTarget:
		return f.target(target.Collection, fullyParenthesized) + "[" + f.expr(target.Index, precedenceLowest, fullyParenthesized) + "]"
	case *verb.RangeTarget:
		return f.target(target.Collection, fullyParenthesized) + "[" + f.expr(target.Start, precedenceLowest, fullyParenthesized) + ".." + f.expr(target.End, precedenceLowest, fullyParenthesized) + "]"
	case *verb.DestructuringTarget:
		bindings := make([]string, len(target.Bindings))
		for i, binding := range target.Bindings {
			switch binding := binding.(type) {
			case *verb.RequiredBinding:
				bindings[i] = f.name(binding.Pos, binding.Name)
			case *verb.OptionalBinding:
				bindings[i] = "?" + f.name(binding.Pos, binding.Name)
				if binding.Default != nil {
					bindings[i] += " = " + f.expr(binding.Default, precedenceLowest, fullyParenthesized)
				}
			case *verb.RestBinding:
				bindings[i] = "@" + f.name(binding.Pos, binding.Name)
			default:
				return f.fail(verb.Position{}, fmt.Errorf("unsupported binding %T", binding))
			}
		}
		return "{" + strings.Join(bindings, ", ") + "}"
	default:
		return f.fail(verb.Position{}, fmt.Errorf("unsupported target %T", target))
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

func (f *mooFormatter) literal(v *verb.LiteralExpr) string {
	switch v.Kind {
	case verb.LiteralInt:
		return strconv.FormatInt(v.IntValue, 10)
	case verb.LiteralFloat:
		if math.IsInf(v.FloatValue, 0) || math.IsNaN(v.FloatValue) {
			return f.fail(v.Pos, fmt.Errorf("nonfinite float has no MOO literal spelling"))
		}
		formatted := strconv.FormatFloat(v.FloatValue, 'f', -1, 64)
		if !strings.ContainsAny(formatted, ".eE") {
			formatted += ".0"
		}
		return formatted
	case verb.LiteralString:
		if strings.IndexByte(v.StringValue, 0) >= 0 {
			return f.fail(v.Pos, ErrNULInSource)
		}
		return quoteMOOString(v.StringValue)
	case verb.LiteralBool:
		if v.BoolValue {
			return "true"
		}
		return "false"
	case verb.LiteralObj:
		return fmt.Sprintf("#%d", v.ObjID)
	case verb.LiteralErr:
		return f.errorName(v.Pos, v.ErrorName)
	default:
		return f.fail(v.Pos, fmt.Errorf("unsupported literal kind %d", v.Kind))
	}
}

// name keeps MOO lexical representability in the frontend. Semantic validation
// requires a name but does not embed this language-specific token grammar.
func (f *mooFormatter) name(pos verb.Position, name string) string {
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

func (f *mooFormatter) errorName(pos verb.Position, name string) string {
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
func (f *mooFormatter) args(args []verb.Expr, fullyParenthesized bool) string {
	if len(args) == 0 {
		return ""
	}
	var parts []string
	for _, arg := range args {
		parts = append(parts, f.expr(arg, precedenceLowest, fullyParenthesized))
	}
	return strings.Join(parts, ", ")
}
