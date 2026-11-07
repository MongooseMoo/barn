package fuzzseed

import (
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/ir"
	"github.com/MongooseMoo/barn/parser"
)

// Generation bounds. Every program stays far below the compiler's constant,
// local-variable and argument limits and the IR nesting limit, so a generated
// program that does not format, parse or compile is a finding, not noise.
const (
	maxNodes      = 160 // expression and statement nodes per program
	maxExprDepth  = 6
	maxBlockDepth = 4
	maxBlockLen   = 4 // statements per nested block
	maxTopLevel   = 8 // statements after the variable prologue
	maxListLen    = 4 // list, map, argument and code list elements
	maxStringLen  = 6
)

// The prologue initializes these variables, so programs can index, iterate and
// call builtins on real values before their first error.
var initializedVariables = []string{"n", "f", "s", "l", "m", "o"}

var (
	assignableNames = []string{"n", "f", "s", "l", "m", "o", "x", "y", "z", "acc"}
	propertyNames   = []string{"name", "owner", "p", "location", "contents"}
	verbNames       = []string{"v", "look", "foo", "initialize"}
	// Each name must stay registered in config.Core with the VM descriptors;
	// structured_test.go checks that.
	builtinNames = []string{
		"length", "tostr", "toint", "tofloat", "typeof", "toliteral",
		"listappend", "listinsert", "listdelete", "listset", "setadd",
		"setremove", "is_member", "mapkeys", "mapvalues", "mapdelete",
		"maphaskey", "abs", "min", "max", "strsub", "index", "rindex",
		"strcmp", "equal", "reverse", "explode", "raise", "suspend", "valid",
		"parent", "children",
	}
	errorNames = []string{
		"E_NONE", "E_TYPE", "E_DIV", "E_PERM", "E_PROPNF", "E_VERBNF",
		"E_VARNF", "E_INVIND", "E_RECMOVE", "E_MAXREC", "E_RANGE", "E_ARGS",
		"E_NACC", "E_INVARG", "E_QUOTA", "E_FLOAT", "E_FILE", "E_EXEC",
		"E_INTRPT",
	}
	// No newline: a MOO string literal cannot cross a source line (Toast
	// parser.y "Missing quote"; parser/format.go rejects it).
	stringPieces  = []string{"a", "b", " ", "\"", "\\", "é", "\t", "$", "%", "1"}
	intLiterals   = []int64{0, 1, 2, -1, 3, 10, 255, 1 << 31, 9223372036854775807, -9223372036854775808}
	floatLiterals = []float64{0, 0.5, 1, 1.5, 2.25, 1e10, 1e-7, 3.0e300}
	objLiterals   = []int64{0, 1, 2, 3, -1, -2, 9223372036854775807}
)

// Program builds one bounded MOO program from fuzz bytes. Each byte is one
// construction choice, so a mutation changes one decision and minimization
// shortens the decision list. Exhausted input reads as zero, which selects the
// first (smallest) alternative, so every input yields a complete program.
func Program(choices []byte) *ir.Program {
	g := &generator{choices: choices, budget: maxNodes}
	g.scope = append(g.scope, initializedVariables...)
	statements := []ir.Stmt{
		assign("n", g.intLiteral()),
		assign("f", g.floatLiteral()),
		assign("s", g.stringLiteral()),
		assign("l", &ir.ListExpr{Elements: []ir.Expr{g.intLiteral(), g.intLiteral(), g.stringLiteral()}}),
		assign("m", &ir.MapExpr{Pairs: []ir.MapPair{{Key: g.stringLiteral(), Value: g.intLiteral()}, {Key: g.intLiteral(), Value: g.stringLiteral()}}}),
		assign("o", g.objLiteral()),
	}
	return &ir.Program{Statements: append(statements, g.block(maxTopLevel, 0)...)}
}

// Source formats Program(choices) with the production formatter.
func Source(choices []byte) (string, error) {
	lines, err := parser.FormatMOOChecked(Program(choices))
	if err != nil {
		return "", err
	}
	return strings.Join(lines, "\n"), nil
}

// StructuredSource returns Source(choices) and fails the test when the
// generated program has no MOO spelling.
func StructuredSource(t testing.TB, choices []byte) string {
	t.Helper()
	source, err := Source(choices)
	if err != nil {
		t.Fatalf("generated program does not format: %v\nchoices: %x", err, choices)
	}
	return source
}

// AddChoices seeds a structured target with a few distinct decision lists.
func AddChoices(f *testing.F) {
	f.Helper()
	f.Add([]byte{})
	for i := 1; i < 16; i++ {
		seed := make([]byte, 96)
		for j := range seed {
			seed[j] = byte(i*37 + j*i)
		}
		f.Add(seed)
	}
}

type loopScope struct{ names []string }

type generator struct {
	choices   []byte
	budget    int
	scope     []string // variables assigned so far
	loops     []loopScope
	indexArgs int // > 0 inside index or range brackets, where ^ and $ are legal
}

func (g *generator) choose(n int) int {
	if len(g.choices) == 0 {
		return 0
	}
	b := g.choices[0]
	g.choices = g.choices[1:]
	return int(b) % n
}

func (g *generator) pick(names []string) string { return names[g.choose(len(names))] }

func (g *generator) spend() bool {
	if g.budget <= 0 {
		return false
	}
	g.budget--
	return true
}

func (g *generator) bind(name string) {
	for _, existing := range g.scope {
		if existing == name {
			return
		}
	}
	g.scope = append(g.scope, name)
}

func assign(name string, value ir.Expr) ir.Stmt {
	return &ir.ExprStmt{Expr: &ir.AssignExpr{Target: &ir.VariableTarget{Name: name}, Value: value}}
}

func (g *generator) block(limit, depth int) []ir.Stmt {
	count := g.choose(limit + 1)
	var statements []ir.Stmt
	for i := 0; i < count && g.spend(); i++ {
		statements = append(statements, g.statement(depth))
	}
	return statements
}

func (g *generator) statement(depth int) ir.Stmt {
	if depth >= maxBlockDepth {
		return &ir.ExprStmt{Expr: g.expr(0)}
	}
	switch g.choose(11) {
	case 0, 1:
		return &ir.ExprStmt{Expr: g.expr(0)}
	case 2:
		return g.ifStatement(depth)
	case 3:
		label := g.optionalName()
		condition := g.expr(1)
		return &ir.WhileStmt{Label: label, Condition: condition, Body: g.loopBody(depth, label)}
	case 4:
		value := g.pick(assignableNames)
		index := ""
		if g.choose(2) == 1 {
			index = g.pick(assignableNames)
			if index == value {
				index = ""
			}
		}
		label := g.optionalName()
		collection := g.expr(1)
		g.bind(value)
		if index != "" {
			g.bind(index)
		}
		return &ir.CollectionLoopStmt{Label: label, Value: value, Index: index, Collection: collection, Body: g.loopBody(depth, label, value, index)}
	case 5:
		value := g.pick(assignableNames)
		label := g.optionalName()
		start, end := g.expr(1), g.expr(1)
		g.bind(value)
		return &ir.RangeLoopStmt{Label: label, Value: value, Start: start, End: end, Body: g.loopBody(depth, label, value)}
	case 6:
		if len(g.loops) == 0 {
			return &ir.EmptyStmt{}
		}
		label := g.loopExitLabel()
		if g.choose(2) == 0 {
			return &ir.BreakStmt{Label: label}
		}
		return &ir.ContinueStmt{Label: label}
	case 7:
		if g.choose(3) == 0 {
			return &ir.ReturnStmt{}
		}
		return &ir.ReturnStmt{Value: g.expr(0)}
	case 8:
		return g.tryStatement(depth)
	case 9:
		delay := g.expr(1)
		name := g.optionalName()
		// A fork body runs as another task and cannot exit the parent's loops.
		outer := g.loops
		g.loops = nil
		body := g.block(maxBlockLen, depth+1)
		g.loops = outer
		if name != "" {
			g.bind(name)
		}
		return &ir.ForkStmt{Delay: delay, VarName: name, Body: body}
	default:
		return &ir.EmptyStmt{}
	}
}

func (g *generator) optionalName() string {
	if g.choose(3) == 0 {
		return g.pick(assignableNames)
	}
	return ""
}

func (g *generator) loopBody(depth int, names ...string) []ir.Stmt {
	g.loops = append(g.loops, loopScope{names: names})
	body := g.block(maxBlockLen, depth+1)
	g.loops = g.loops[:len(g.loops)-1]
	return body
}

// loopExitLabel names an enclosing loop of this task, or returns "" for the
// innermost loop. A loop's value and index variables also name it.
func (g *generator) loopExitLabel() string {
	var names []string
	for _, loop := range g.loops {
		for _, name := range loop.names {
			if name != "" {
				names = append(names, name)
			}
		}
	}
	if len(names) == 0 || g.choose(2) == 0 {
		return ""
	}
	return g.pick(names)
}

func (g *generator) ifStatement(depth int) ir.Stmt {
	stmt := &ir.IfStmt{Condition: g.expr(1), Body: g.block(maxBlockLen, depth+1)}
	switch g.choose(3) {
	case 1:
		stmt.Else = g.block(maxBlockLen, depth+1)
	case 2:
		if g.spend() {
			stmt.Else = []ir.Stmt{g.ifStatement(depth + 1)}
		}
	}
	return stmt
}

func (g *generator) tryStatement(depth int) ir.Stmt {
	stmt := &ir.TryStmt{Body: g.block(maxBlockLen, depth+1)}
	handlers := g.choose(3)
	for i := 0; i < handlers; i++ {
		handler := ir.ExceptionHandler{}
		if g.choose(2) == 1 {
			handler.Variable = g.pick(assignableNames)
			g.bind(handler.Variable)
		}
		handler.IsAny, handler.Codes = g.codes()
		handler.Body = g.block(maxBlockLen, depth+1)
		stmt.Handlers = append(stmt.Handlers, handler)
		if handler.IsAny {
			break // a later arm is unreachable (parser_stmt.go:525-526)
		}
	}
	if handlers == 0 || g.choose(2) == 1 {
		stmt.Finalizer = &ir.Finalizer{Body: g.block(maxBlockLen, depth+1)}
	}
	return stmt
}

// codes returns ANY or a nonempty code list, mostly error literals.
func (g *generator) codes() (bool, []ir.Expr) {
	if g.choose(2) == 0 {
		return true, nil
	}
	count := 1 + g.choose(maxListLen)
	codes := make([]ir.Expr, count)
	for i := range codes {
		if g.choose(4) == 0 {
			codes[i] = g.argument(2)
		} else {
			codes[i] = g.errLiteral()
		}
	}
	return false, codes
}

func (g *generator) expr(depth int) ir.Expr {
	if depth >= maxExprDepth || !g.spend() {
		return g.terminal()
	}
	choice := g.choose(17)
	if choice == 16 && g.indexArgs == 0 {
		choice = 0
	}
	switch choice {
	case 0, 1:
		return g.terminal()
	case 2:
		return &ir.UnaryExpr{Operator: ir.UnaryOperator(g.choose(int(ir.UnaryBitwiseNot) + 1)), Operand: g.expr(depth + 1)}
	case 3, 4:
		operator := ir.BinaryOperator(g.choose(int(ir.BinaryShiftRight) + 1))
		return &ir.BinaryExpr{Left: g.expr(depth + 1), Operator: operator, Right: g.expr(depth + 1)}
	case 5:
		return &ir.TernaryExpr{Condition: g.expr(depth + 1), ThenExpr: g.expr(depth + 1), ElseExpr: g.expr(depth + 1)}
	case 6:
		base := g.expr(depth + 1)
		g.indexArgs++
		index := g.expr(depth + 1)
		g.indexArgs--
		return &ir.IndexExpr{Expr: base, Index: index}
	case 7:
		base := g.expr(depth + 1)
		g.indexArgs++
		start, end := g.expr(depth+1), g.expr(depth+1)
		g.indexArgs--
		return &ir.RangeExpr{Expr: base, Start: start, End: end}
	case 8:
		property := &ir.PropertyExpr{Expr: g.propertyBase(depth + 1)}
		if g.choose(4) == 0 {
			property.PropertyExpr = g.expr(depth + 1)
		} else {
			property.Property = g.pick(propertyNames)
		}
		return property
	case 9:
		call := &ir.VerbCallExpr{Expr: g.expr(depth + 1)}
		if g.choose(4) == 0 {
			call.VerbExpr = g.expr(depth + 1)
		} else {
			call.Verb = g.pick(verbNames)
		}
		call.Args = g.arguments(depth + 1)
		return call
	case 10:
		return &ir.BuiltinCallExpr{Name: g.pick(builtinNames), Args: g.arguments(depth + 1)}
	case 11:
		catch := &ir.CatchExpr{Expr: g.expr(depth + 1)}
		catch.IsAny, catch.Codes = g.codes()
		if g.choose(2) == 1 {
			catch.Default = g.expr(depth + 1)
		}
		return catch
	case 12:
		return g.assignment(depth + 1)
	case 13:
		// The parser leaves an empty list's elements nil, and an empty call
		// argument list non-nil (parser.go:190, :373, :492).
		elements := g.arguments(depth + 1)
		if len(elements) == 0 {
			elements = nil
		}
		return &ir.ListExpr{Elements: elements}
	case 14:
		// No ir.ListRangeExpr: the parser rejects {a..b} list literals.
		var pairs []ir.MapPair
		for i, count := 0, g.choose(maxListLen+1); i < count; i++ {
			pairs = append(pairs, ir.MapPair{Key: g.expr(depth + 1), Value: g.expr(depth + 1)})
		}
		return &ir.MapExpr{Pairs: pairs}
	case 15:
		return g.variable()
	default:
		return &ir.IndexBoundaryExpr{Boundary: ir.IndexBoundary(g.choose(2))}
	}
}

// arguments builds a list or call argument list; only here, and in code
// lists, may an element be a splice.
func (g *generator) arguments(depth int) []ir.Expr {
	args := make([]ir.Expr, g.choose(maxListLen+1))
	for i := range args {
		args[i] = g.argument(depth)
	}
	return args
}

func (g *generator) argument(depth int) ir.Expr {
	if g.choose(5) == 0 {
		return &ir.SpliceExpr{Expr: g.expr(depth + 1)}
	}
	return g.expr(depth)
}

func (g *generator) assignment(depth int) ir.Expr {
	value := g.expr(depth + 1)
	var target ir.Target
	switch g.choose(6) {
	case 0, 1:
		name := g.pick(assignableNames)
		target = &ir.VariableTarget{Name: name}
		g.bind(name)
	case 2:
		target = g.propertyTarget(depth)
	case 3:
		g.indexArgs++
		index := g.expr(depth + 1)
		g.indexArgs--
		target = &ir.IndexTarget{Collection: g.collectionTarget(depth), Index: index}
	case 4:
		g.indexArgs++
		start, end := g.expr(depth+1), g.expr(depth+1)
		g.indexArgs--
		target = &ir.RangeTarget{Collection: g.collectionTarget(depth), Start: start, End: end}
	default:
		target = g.destructuringTarget(depth)
	}
	return &ir.AssignExpr{Target: target, Value: value}
}

func (g *generator) propertyTarget(depth int) *ir.PropertyTarget {
	target := &ir.PropertyTarget{Object: g.propertyBase(depth + 1)}
	if g.choose(4) == 0 {
		target.NameExpr = g.expr(depth + 1)
	} else {
		target.Name = g.pick(propertyNames)
	}
	return target
}

// propertyBase is the object a property is read from or written to. It is
// never a bare ^: the formatter spells (^).name as ^.name, which lexes as
// bitwise XOR, and Toast's unparser does the same (unparse.cc:264,273). That
// difference is tracked separately, not decided by this generator.
func (g *generator) propertyBase(depth int) ir.Expr {
	base := g.expr(depth)
	if boundary, ok := base.(*ir.IndexBoundaryExpr); ok && boundary.Boundary == ir.IndexFirst {
		return g.variable()
	}
	return base
}

// collectionTarget is the variable, property or nested index an indexed
// assignment writes back through.
func (g *generator) collectionTarget(depth int) ir.CollectionTarget {
	switch g.choose(4) {
	case 1:
		return g.propertyTarget(depth)
	case 2:
		if depth < maxExprDepth && g.spend() {
			g.indexArgs++
			index := g.expr(depth + 1)
			g.indexArgs--
			return &ir.IndexTarget{Collection: g.collectionTarget(depth + 1), Index: index}
		}
	}
	return &ir.VariableTarget{Name: g.pick(g.scope)}
}

func (g *generator) destructuringTarget(depth int) *ir.DestructuringTarget {
	count := 1 + g.choose(maxListLen)
	rest := false
	target := &ir.DestructuringTarget{}
	for i := 0; i < count; i++ {
		name := g.pick(assignableNames)
		switch g.choose(4) {
		case 1:
			binding := &ir.OptionalBinding{Name: name}
			if g.choose(2) == 1 {
				binding.Default = g.expr(depth + 1)
			}
			target.Bindings = append(target.Bindings, binding)
		case 2:
			if !rest {
				rest = true
				target.Bindings = append(target.Bindings, &ir.RestBinding{Name: name})
				break
			}
			fallthrough
		default:
			target.Bindings = append(target.Bindings, &ir.RequiredBinding{Name: name})
		}
		g.bind(name)
	}
	return target
}

func (g *generator) terminal() ir.Expr {
	switch g.choose(8) {
	case 0, 1:
		return g.variable()
	case 2:
		return g.intLiteral()
	case 3:
		return g.floatLiteral()
	case 4:
		return g.stringLiteral()
	case 5:
		return g.objLiteral()
	case 6:
		return g.errLiteral()
	default:
		if g.indexArgs > 0 {
			return &ir.IndexBoundaryExpr{Boundary: ir.IndexBoundary(g.choose(2))}
		}
		return g.intLiteral()
	}
}

func (g *generator) variable() ir.Expr { return &ir.IdentifierExpr{Name: g.pick(g.scope)} }

func (g *generator) intLiteral() ir.Expr {
	if choice := g.choose(len(intLiterals) + 1); choice < len(intLiterals) {
		return &ir.LiteralExpr{Kind: ir.LiteralInt, IntValue: intLiterals[choice]}
	}
	return &ir.LiteralExpr{Kind: ir.LiteralInt, IntValue: int64(g.choose(256))}
}

// floatLiteral is never negative: a minus sign is a unary operator in MOO
// source, so a negative float literal has no spelling that reparses to it.
func (g *generator) floatLiteral() ir.Expr {
	if choice := g.choose(len(floatLiterals) + 1); choice < len(floatLiterals) {
		return &ir.LiteralExpr{Kind: ir.LiteralFloat, FloatValue: floatLiterals[choice]}
	}
	return &ir.LiteralExpr{Kind: ir.LiteralFloat, FloatValue: float64(g.choose(256)) / 8}
}

func (g *generator) stringLiteral() ir.Expr {
	var value strings.Builder
	for i, n := 0, g.choose(maxStringLen+1); i < n; i++ {
		value.WriteString(g.pick(stringPieces))
	}
	return &ir.LiteralExpr{Kind: ir.LiteralString, StringValue: value.String()}
}

func (g *generator) objLiteral() ir.Expr {
	return &ir.LiteralExpr{Kind: ir.LiteralObj, ObjID: objLiterals[g.choose(len(objLiterals))]}
}

func (g *generator) errLiteral() ir.Expr {
	return &ir.LiteralExpr{Kind: ir.LiteralErr, ErrorName: g.pick(errorNames)}
}
