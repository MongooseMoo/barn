package builtins

import (
	"fmt"
	"math"
	"strings"

	"github.com/MongooseMoo/barn/compiler"
	"github.com/MongooseMoo/barn/ir"
	"github.com/MongooseMoo/barn/parser"
	"github.com/MongooseMoo/barn/types"
)

// Syntax trees as MOO values, matching the Mongoose ToastStunt fork's
// parse_ast(), verb_ast(), validate_ast() and unparse_ast() (fork
// src/ast_map.cc, docs/Features/ast.md). A program is a LIST of statement
// nodes; a node is a MAP holding a "type" string and that type's fields.
// Trees are made from the program the existing parser builds and are turned
// back into code by the existing formatter.

// astLanguageVersion is the database language version whose builtin variable
// spellings a tree reports (the fork's current version, with BOOL, true and
// false).
const astLanguageVersion = 17

func astField(key string, value types.Value) [2]types.Value {
	return [2]types.Value{types.NewStr(key), value}
}

func astNode(nodeType string, fields ...[2]types.Value) types.Value {
	return types.NewMap(append(fields, astField("type", types.NewStr(nodeType))))
}

// astWriter turns a parsed program into its tree.
type astWriter struct {
	elseIfArms map[*ir.IfStmt]bool // IfStmts written as `elseif' arms
	spelling   map[string]string   // upper-case variable name -> the program's spelling
}

// programAST is the fork's tree for source: what its decompiler gives for the
// compiled program, so `elseif' arms, folded negative literals and the
// program's variable spellings are reported, and a lone `;' is not.
func programAST(source string, program *ir.Program) (types.Value, error) {
	names, err := compiler.NativeVariableNames(program, astLanguageVersion)
	if err != nil {
		return types.Value{}, err
	}
	w := &astWriter{elseIfArms: parser.ElseIfArms(source, program), spelling: make(map[string]string, len(names))}
	for _, name := range names {
		upper := strings.ToUpper(name)
		if _, ok := w.spelling[upper]; !ok {
			w.spelling[upper] = name
		}
	}
	return w.block(program.Statements)
}

func (w *astWriter) name(name string) types.Value {
	if spelled, ok := w.spelling[strings.ToUpper(name)]; ok {
		return types.NewStr(spelled)
	}
	return types.NewStr(name)
}

func (w *astWriter) block(stmts []ir.Stmt) (types.Value, error) {
	nodes := make([]types.Value, 0, len(stmts))
	for _, stmt := range stmts {
		if _, empty := stmt.(*ir.EmptyStmt); empty {
			continue
		}
		node, err := w.stmt(stmt)
		if err != nil {
			return types.Value{}, err
		}
		nodes = append(nodes, node)
	}
	return types.NewList(nodes), nil
}

func (w *astWriter) stmt(stmt ir.Stmt) (types.Value, error) {
	switch s := stmt.(type) {
	case *ir.ExprStmt:
		expr, err := w.expr(s.Expr)
		if err != nil {
			return types.Value{}, err
		}
		return astNode("expr", astField("expr", expr)), nil

	case *ir.ReturnStmt:
		if s.Value == nil {
			return astNode("return"), nil
		}
		value, err := w.expr(s.Value)
		if err != nil {
			return types.Value{}, err
		}
		return astNode("return", astField("value", value)), nil

	case *ir.IfStmt:
		var arms []types.Value
		var fields [][2]types.Value
		for current := s; ; {
			condition, err := w.expr(current.Condition)
			if err != nil {
				return types.Value{}, err
			}
			body, err := w.block(current.Body)
			if err != nil {
				return types.Value{}, err
			}
			arms = append(arms, types.NewMap([][2]types.Value{astField("condition", condition), astField("body", body)}))
			if len(current.Else) == 1 {
				if next, ok := current.Else[0].(*ir.IfStmt); ok && w.elseIfArms[next] {
					current = next
					continue
				}
			}
			otherwise, err := w.block(current.Else)
			if err != nil {
				return types.Value{}, err
			}
			if otherwise.Len() > 0 {
				fields = append(fields, astField("else", otherwise))
			}
			break
		}
		return astNode("if", append(fields, astField("arms", types.NewList(arms)))...), nil

	case *ir.CollectionLoopStmt:
		if s.Label != "" {
			return types.Value{}, fmt.Errorf("a labelled for loop has no syntax tree")
		}
		collection, err := w.expr(s.Collection)
		if err != nil {
			return types.Value{}, err
		}
		body, err := w.block(s.Body)
		if err != nil {
			return types.Value{}, err
		}
		fields := [][2]types.Value{astField("var", w.name(s.Value)), astField("expr", collection), astField("body", body)}
		if s.Index != "" {
			fields = append(fields, astField("index", w.name(s.Index)))
		}
		return astNode("for", fields...), nil

	case *ir.RangeLoopStmt:
		if s.Label != "" {
			return types.Value{}, fmt.Errorf("a labelled for loop has no syntax tree")
		}
		from, err := w.expr(s.Start)
		if err != nil {
			return types.Value{}, err
		}
		to, err := w.expr(s.End)
		if err != nil {
			return types.Value{}, err
		}
		body, err := w.block(s.Body)
		if err != nil {
			return types.Value{}, err
		}
		return astNode("for_range", astField("var", w.name(s.Value)), astField("from", from), astField("to", to), astField("body", body)), nil

	case *ir.WhileStmt:
		condition, err := w.expr(s.Condition)
		if err != nil {
			return types.Value{}, err
		}
		body, err := w.block(s.Body)
		if err != nil {
			return types.Value{}, err
		}
		fields := [][2]types.Value{astField("condition", condition), astField("body", body)}
		if s.Label != "" {
			fields = append(fields, astField("name", w.name(s.Label)))
		}
		return astNode("while", fields...), nil

	case *ir.ForkStmt:
		delay, err := w.expr(s.Delay)
		if err != nil {
			return types.Value{}, err
		}
		body, err := w.block(s.Body)
		if err != nil {
			return types.Value{}, err
		}
		fields := [][2]types.Value{astField("delay", delay), astField("body", body)}
		if s.VarName != "" {
			fields = append(fields, astField("var", w.name(s.VarName)))
		}
		return astNode("fork", fields...), nil

	case *ir.TryStmt:
		body, err := w.block(s.Body)
		if err != nil {
			return types.Value{}, err
		}
		if s.Finalizer != nil {
			handler, err := w.block(s.Finalizer.Body)
			if err != nil {
				return types.Value{}, err
			}
			return astNode("try_finally", astField("body", body), astField("finally", handler)), nil
		}
		excepts := make([]types.Value, len(s.Handlers))
		for i, handler := range s.Handlers {
			handlerBody, err := w.block(handler.Body)
			if err != nil {
				return types.Value{}, err
			}
			fields := [][2]types.Value{astField("body", handlerBody)}
			if handler.Variable != "" {
				fields = append(fields, astField("var", w.name(handler.Variable)))
			}
			if !handler.IsAny {
				codes, err := w.args(handler.Codes)
				if err != nil {
					return types.Value{}, err
				}
				fields = append(fields, astField("codes", codes))
			}
			excepts[i] = types.NewMap(fields)
		}
		return astNode("try_except", astField("body", body), astField("excepts", types.NewList(excepts))), nil

	case *ir.BreakStmt:
		if s.Label == "" {
			return astNode("break"), nil
		}
		return astNode("break", astField("name", w.name(s.Label))), nil

	case *ir.ContinueStmt:
		if s.Label == "" {
			return astNode("continue"), nil
		}
		return astNode("continue", astField("name", w.name(s.Label))), nil
	}
	return types.Value{}, fmt.Errorf("statement %T has no syntax tree", stmt)
}

func (w *astWriter) args(exprs []ir.Expr) (types.Value, error) {
	items := make([]types.Value, len(exprs))
	for i, expr := range exprs {
		item, err := w.expr(expr)
		if err != nil {
			return types.Value{}, err
		}
		items[i] = item
	}
	return types.NewList(items), nil
}

func literalNode(value types.Value) types.Value {
	return astNode("literal", astField("value", value))
}

// numericLiteral is the value of a number literal after the parser's folding
// of unary minus into INT and FLOAT literals (fork parser.y, '-' expr).
func numericLiteral(expr ir.Expr) (types.Value, bool) {
	switch e := expr.(type) {
	case *ir.LiteralExpr:
		switch e.Kind {
		case ir.LiteralInt:
			return types.NewInt(e.IntValue), true
		case ir.LiteralFloat:
			return types.NewFloat(e.FloatValue), true
		}
	case *ir.UnaryExpr:
		if e.Operator == ir.UnaryNegate {
			if value, ok := numericLiteral(e.Operand); ok {
				if value.Type() == types.TYPE_INT {
					return types.NewInt(-value.Int()), true
				}
				return types.NewFloat(-value.Float()), true
			}
		}
	}
	return types.Value{}, false
}

func (w *astWriter) expr(expr ir.Expr) (types.Value, error) {
	if value, ok := numericLiteral(expr); ok {
		return literalNode(value), nil
	}
	switch e := expr.(type) {
	case *ir.LiteralExpr:
		switch e.Kind {
		case ir.LiteralString:
			return literalNode(types.NewStr(e.StringValue)), nil
		case ir.LiteralObj:
			return literalNode(types.NewObj(types.ObjID(e.ObjID))), nil
		case ir.LiteralErr:
			code, ok := types.ErrorFromString(e.ErrorName)
			if !ok {
				return types.Value{}, fmt.Errorf("unknown error %s", e.ErrorName)
			}
			return literalNode(types.NewErr(code)), nil
		}
		return types.Value{}, fmt.Errorf("literal kind %d has no syntax tree", e.Kind)

	case *ir.IdentifierExpr:
		return astNode("variable", astField("name", w.name(e.Name))), nil

	case *ir.UnaryExpr:
		spelling, ok := parser.UnaryOperatorSpelling(e.Operator)
		if !ok {
			return types.Value{}, fmt.Errorf("unknown unary operator %d", e.Operator)
		}
		operand, err := w.expr(e.Operand)
		if err != nil {
			return types.Value{}, err
		}
		return astNode("unary", astField("op", types.NewStr(spelling)), astField("expr", operand)), nil

	case *ir.BinaryExpr:
		spelling, ok := parser.BinaryOperatorSpelling(e.Operator)
		if !ok {
			return types.Value{}, fmt.Errorf("unknown binary operator %d", e.Operator)
		}
		lhs, err := w.expr(e.Left)
		if err != nil {
			return types.Value{}, err
		}
		rhs, err := w.expr(e.Right)
		if err != nil {
			return types.Value{}, err
		}
		return astNode("binary", astField("op", types.NewStr(spelling)), astField("lhs", lhs), astField("rhs", rhs)), nil

	case *ir.TernaryExpr:
		condition, err := w.expr(e.Condition)
		if err != nil {
			return types.Value{}, err
		}
		consequent, err := w.expr(e.ThenExpr)
		if err != nil {
			return types.Value{}, err
		}
		alternate, err := w.expr(e.ElseExpr)
		if err != nil {
			return types.Value{}, err
		}
		return astNode("conditional", astField("condition", condition), astField("consequent", consequent), astField("alternate", alternate)), nil

	case *ir.IndexBoundaryExpr:
		if e.Boundary == ir.IndexFirst {
			return astNode("first"), nil
		}
		return astNode("last"), nil

	case *ir.IndexExpr:
		return w.index(e.Expr, e.Index)

	case *ir.RangeExpr:
		return w.rangeNode(e.Expr, e.Start, e.End)

	case *ir.PropertyExpr:
		return w.prop(e.Expr, e.Property, e.PropertyExpr)

	case *ir.VerbCallExpr:
		object, err := w.expr(e.Expr)
		if err != nil {
			return types.Value{}, err
		}
		verb, err := w.nameExpr(e.Verb, e.VerbExpr)
		if err != nil {
			return types.Value{}, err
		}
		args, err := w.args(e.Args)
		if err != nil {
			return types.Value{}, err
		}
		return astNode("verb_call", astField("object", object), astField("verb", verb), astField("args", args)), nil

	case *ir.BuiltinCallExpr:
		args, err := w.args(e.Args)
		if err != nil {
			return types.Value{}, err
		}
		return astNode("call", astField("function", types.NewStr(strings.ToLower(e.Name))), astField("args", args)), nil

	case *ir.SpliceExpr:
		inner, err := w.expr(e.Expr)
		if err != nil {
			return types.Value{}, err
		}
		return astNode("splice", astField("expr", inner)), nil

	case *ir.CatchExpr:
		inner, err := w.expr(e.Expr)
		if err != nil {
			return types.Value{}, err
		}
		fields := [][2]types.Value{astField("expr", inner)}
		if !e.IsAny {
			codes, err := w.args(e.Codes)
			if err != nil {
				return types.Value{}, err
			}
			fields = append(fields, astField("codes", codes))
		}
		if e.Default != nil {
			value, err := w.expr(e.Default)
			if err != nil {
				return types.Value{}, err
			}
			fields = append(fields, astField("default", value))
		}
		return astNode("catch", fields...), nil

	case *ir.AssignExpr:
		target, err := w.target(e.Target)
		if err != nil {
			return types.Value{}, err
		}
		value, err := w.expr(e.Value)
		if err != nil {
			return types.Value{}, err
		}
		return astNode("assign", astField("target", target), astField("value", value)), nil

	case *ir.ListExpr:
		items, err := w.args(e.Elements)
		if err != nil {
			return types.Value{}, err
		}
		return astNode("list", astField("items", items)), nil

	case *ir.MapExpr:
		entries := make([]types.Value, len(e.Pairs))
		for i, pair := range e.Pairs {
			key, err := w.expr(pair.Key)
			if err != nil {
				return types.Value{}, err
			}
			value, err := w.expr(pair.Value)
			if err != nil {
				return types.Value{}, err
			}
			entries[i] = types.NewMap([][2]types.Value{astField("key", key), astField("value", value)})
		}
		return astNode("map", astField("entries", types.NewList(entries))), nil
	}
	return types.Value{}, fmt.Errorf("expression %T has no syntax tree", expr)
}

// nameExpr is the property or verb name of a node: a string literal for a
// static name, otherwise the expression.
func (w *astWriter) nameExpr(static string, dynamic ir.Expr) (types.Value, error) {
	if dynamic == nil {
		return literalNode(types.NewStr(static)), nil
	}
	return w.expr(dynamic)
}

func (w *astWriter) prop(object ir.Expr, static string, dynamic ir.Expr) (types.Value, error) {
	objectNode, err := w.expr(object)
	if err != nil {
		return types.Value{}, err
	}
	property, err := w.nameExpr(static, dynamic)
	if err != nil {
		return types.Value{}, err
	}
	return astNode("prop", astField("object", objectNode), astField("property", property)), nil
}

func (w *astWriter) index(base, index ir.Expr) (types.Value, error) {
	baseNode, err := w.expr(base)
	if err != nil {
		return types.Value{}, err
	}
	indexNode, err := w.expr(index)
	if err != nil {
		return types.Value{}, err
	}
	return astNode("index", astField("base", baseNode), astField("index", indexNode)), nil
}

func (w *astWriter) rangeNode(base, from, to ir.Expr) (types.Value, error) {
	baseNode, err := w.expr(base)
	if err != nil {
		return types.Value{}, err
	}
	fromNode, err := w.expr(from)
	if err != nil {
		return types.Value{}, err
	}
	toNode, err := w.expr(to)
	if err != nil {
		return types.Value{}, err
	}
	return astNode("range", astField("base", baseNode), astField("from", fromNode), astField("to", toNode)), nil
}

func (w *astWriter) target(target ir.Target) (types.Value, error) {
	switch t := target.(type) {
	case *ir.VariableTarget:
		return astNode("variable", astField("name", w.name(t.Name))), nil
	case *ir.PropertyTarget:
		return w.prop(t.Object, t.Name, t.NameExpr)
	case *ir.IndexTarget:
		base, err := w.target(t.Collection)
		if err != nil {
			return types.Value{}, err
		}
		index, err := w.expr(t.Index)
		if err != nil {
			return types.Value{}, err
		}
		return astNode("index", astField("base", base), astField("index", index)), nil
	case *ir.RangeTarget:
		base, err := w.target(t.Collection)
		if err != nil {
			return types.Value{}, err
		}
		from, err := w.expr(t.Start)
		if err != nil {
			return types.Value{}, err
		}
		to, err := w.expr(t.End)
		if err != nil {
			return types.Value{}, err
		}
		return astNode("range", astField("base", base), astField("from", from), astField("to", to)), nil
	case *ir.DestructuringTarget:
		items := make([]types.Value, len(t.Bindings))
		for i, binding := range t.Bindings {
			switch b := binding.(type) {
			case *ir.RequiredBinding:
				items[i] = types.NewMap([][2]types.Value{astField("kind", types.NewStr("required")), astField("var", w.name(b.Name))})
			case *ir.RestBinding:
				items[i] = types.NewMap([][2]types.Value{astField("kind", types.NewStr("rest")), astField("var", w.name(b.Name))})
			case *ir.OptionalBinding:
				fields := [][2]types.Value{astField("kind", types.NewStr("optional")), astField("var", w.name(b.Name))}
				if b.Default != nil {
					value, err := w.expr(b.Default)
					if err != nil {
						return types.Value{}, err
					}
					fields = append(fields, astField("default", value))
				}
				items[i] = types.NewMap(fields)
			default:
				return types.Value{}, fmt.Errorf("binding %T has no syntax tree", binding)
			}
		}
		return astNode("scatter", astField("items", types.NewList(items))), nil
	}
	return types.Value{}, fmt.Errorf("target %T has no syntax tree", target)
}

// sourceAST parses source and returns its tree. A program the parser accepts
// but no fork node can describe (a Barn-only spelling) is E_INVARG.
func sourceAST(lines []string) types.Result {
	source := strings.Join(lines, "\n")
	program, err := parser.NewParser(source).ParseProgram()
	if err == nil {
		var tree types.Value
		if tree, err = programAST(source, program); err == nil {
			return types.Ok(tree)
		}
	}
	return astRaise(err.Error(), types.NewList([]types.Value{types.NewStr(err.Error())}))
}

func astRaise(message string, value types.Value) types.Result {
	return types.Result{
		Flow:  types.FlowException,
		Error: types.E_INVARG,
		Val:   types.NewList([]types.Value{types.NewErr(types.E_INVARG), types.NewStr(message), value}),
	}
}

// builtinParseAST: parse_ast(LIST code) -> LIST. E_TYPE unless every line is a
// string; code that does not compile raises E_INVARG with the first compiler
// error as the message and all of them, as set_verb_code() returns them, as
// the value.
func builtinParseAST(ctx *Execution, args []types.Value) types.Result {
	code := args[0]
	lines := make([]string, code.Len())
	for i := 1; i <= code.Len(); i++ {
		line := code.Get(i)
		if line.Type() != types.TYPE_STR {
			return types.Err(types.E_TYPE)
		}
		lines[i-1] = line.Str()
	}
	if _, diagnostics := ctx.Registry.Compiler().CompileMOO(lines); len(diagnostics) > 0 {
		errors := make([]types.Value, len(diagnostics))
		for i, diagnostic := range diagnostics {
			errors[i] = types.NewStr(diagnostic.Error())
		}
		return astRaise(diagnostics[0].Error(), types.NewList(errors))
	}
	return sourceAST(lines)
}

// ---- tree to program ----------------------------------------------------

// astMaxDepth is how deeply a tree from MOO code may nest (fork AST_MAX_DEPTH).
const astMaxDepth = 1000

// astBuilder turns a tree into a program, checking that it has a shape the
// parser could have produced. Its checks, their order and its messages are the
// fork's (src/ast_map.cc, build_*). A failed check records what is wrong;
// callers add their position on the way out, so the path reads outermost first.
type astBuilder struct {
	registry *Registry
	spelling map[string]string // upper-case variable name -> first spelling
	depth    int
	err      string
	path     []string
	// elseIfArms holds the IfStmts made from the second and later arms of an
	// `if' node. Any other IfStmt alone in an Else was a hand-built `else'
	// block holding an `if'.
	elseIfArms map[*ir.IfStmt]bool
}

func newASTBuilder(registry *Registry) *astBuilder {
	b := &astBuilder{registry: registry, spelling: make(map[string]string), elseIfArms: make(map[*ir.IfStmt]bool)}
	builtinNames, _ := compiler.NativeVariableNames(&ir.Program{}, astLanguageVersion)
	for _, name := range builtinNames {
		b.spelling[strings.ToUpper(name)] = name
	}
	return b
}

// fail records the problem, cut to the fork's 199-byte message buffer.
func (b *astBuilder) fail(format string, a ...any) {
	b.err = fmt.Sprintf(format, a...)
	if len(b.err) > 199 {
		b.err = b.err[:199]
	}
}

func (b *astBuilder) atKey(key string) { b.path = append(b.path, "."+key) }
func (b *astBuilder) atIndex(i int)    { b.path = append(b.path, fmt.Sprintf("[%d]", i)) }

// message is unparse_ast()'s E_INVARG message.
func (b *astBuilder) message() string {
	var path strings.Builder
	for i := len(b.path) - 1; i >= 0; i-- {
		path.WriteString(b.path[i])
	}
	where := path.String()
	if len(where) > 200 {
		where = where[:200] + "..."
	}
	message := "Invalid AST"
	if where != "" {
		message += " at " + where
	}
	return message + ": " + b.err
}

// find looks a field up with the exact letter case of its key.
func astFind(node types.Value, key string) (types.Value, bool) {
	value, ok := node.MapGet(types.NewStr(key))
	if !ok {
		return types.Value{}, false
	}
	for _, k := range node.Keys() {
		if k.Type() == types.TYPE_STR && strings.EqualFold(k.Str(), key) {
			return value, k.Str() == key
		}
	}
	return types.Value{}, false
}

func (b *astBuilder) findRequired(node types.Value, key string) (types.Value, bool) {
	if value, ok := astFind(node, key); ok {
		return value, true
	}
	b.fail("missing \"%s\"", key)
	return types.Value{}, false
}

func (b *astBuilder) checkType(value types.Value, t types.TypeCode) bool {
	if value.Type() == t {
		return true
	}
	b.fail("expected %s, found %s", t.ToastName(), value.Type().ToastName())
	return false
}

// nodeType is the "type" of a node, or false if value is not a node.
func (b *astBuilder) nodeType(value types.Value) (string, bool) {
	if !b.checkType(value, types.TYPE_MAP) {
		return "", false
	}
	t, ok := astFind(value, "type")
	if !ok || t.Type() != types.TYPE_STR {
		b.fail("missing \"type\"")
		return "", false
	}
	return t.Str(), true
}

func astIsNode(value types.Value, nodeType string) bool {
	if value.Type() != types.TYPE_MAP {
		return false
	}
	t, ok := astFind(value, "type")
	return ok && t.Type() == types.TYPE_STR && t.Str() == nodeType
}

func (b *astBuilder) exprField(node types.Value, key string) ir.Expr {
	value, ok := b.findRequired(node, key)
	if !ok {
		return nil
	}
	expr := b.expr(value)
	if expr == nil {
		b.atKey(key)
	}
	return expr
}

// optionalExprField is (nil, true) when the field is absent.
func (b *astBuilder) optionalExprField(node types.Value, key string) (ir.Expr, bool) {
	value, ok := astFind(node, key)
	if !ok {
		return nil, true
	}
	expr := b.expr(value)
	if expr == nil {
		b.atKey(key)
		return nil, false
	}
	return expr, true
}

// nameField is ("", true) when an optional name is absent.
func (b *astBuilder) nameField(node types.Value, key string, optional bool) (string, bool) {
	value, ok := astFind(node, key)
	if !ok {
		if !optional {
			b.fail("missing \"%s\"", key)
		}
		return "", optional
	}
	if value.Type() != types.TYPE_STR || !parser.IsIdentifier(value.Str()) {
		b.fail("expected a variable name")
		b.atKey(key)
		return "", false
	}
	upper := strings.ToUpper(value.Str())
	if spelled, ok := b.spelling[upper]; ok {
		return spelled, true
	}
	b.spelling[upper] = value.Str()
	return value.Str(), true
}

func (b *astBuilder) listField(node types.Value, key string) (types.Value, bool) {
	list, ok := b.findRequired(node, key)
	if !ok {
		return types.Value{}, false
	}
	if !b.checkType(list, types.TYPE_LIST) {
		b.atKey(key)
		return types.Value{}, false
	}
	return list, true
}

func (b *astBuilder) args(list types.Value) ([]ir.Expr, bool) {
	args := make([]ir.Expr, 0, list.Len())
	for i := 1; i <= list.Len(); i++ {
		item := list.Get(i)
		var expr ir.Expr
		if astIsNode(item, "splice") {
			if inner := b.exprField(item, "expr"); inner != nil {
				expr = &ir.SpliceExpr{Expr: inner}
			}
		} else {
			expr = b.expr(item)
		}
		if expr == nil {
			b.atIndex(i)
			return nil, false
		}
		args = append(args, expr)
	}
	return args, true
}

func (b *astBuilder) argsField(node types.Value, key string) ([]ir.Expr, bool) {
	list, ok := b.listField(node, key)
	if !ok {
		return nil, false
	}
	args, ok := b.args(list)
	if !ok {
		b.atKey(key)
	}
	return args, ok
}

// codesField is (nil, true, true) for an absent "codes", which means ANY.
func (b *astBuilder) codesField(node types.Value) (codes []ir.Expr, isAny bool, ok bool) {
	list, present := astFind(node, "codes")
	if !present {
		return nil, true, true
	}
	ok = b.checkType(list, types.TYPE_LIST)
	if ok && list.Len() == 0 {
		b.fail("must not be empty; leave it out to catch ANY")
		ok = false
	}
	if ok {
		codes, ok = b.args(list)
	}
	if !ok {
		b.atKey("codes")
		return nil, false, false
	}
	return codes, false, true
}

func (b *astBuilder) bodyField(node types.Value, key string, optional bool) ([]ir.Stmt, bool) {
	if optional {
		if _, present := astFind(node, key); !present {
			return nil, true
		}
	}
	list, ok := b.listField(node, key)
	if !ok {
		return nil, false
	}
	stmts, ok := b.stmts(list)
	if !ok {
		b.atKey(key)
	}
	return stmts, ok
}

func (b *astBuilder) literal(node types.Value) ir.Expr {
	value, ok := b.findRequired(node, "value")
	if !ok {
		return nil
	}
	switch value.Type() {
	case types.TYPE_STR:
		return &ir.LiteralExpr{Kind: ir.LiteralString, StringValue: value.Str()}
	// In the program a negative number is minus applied to its magnitude, as
	// the parser writes it; the formatter prints that as Toast prints the
	// folded literal. (The most negative integer is its own magnitude, which
	// the formatter spells unsigned, so it still prints with one minus.)
	case types.TYPE_INT:
		n := value.Int()
		if n < 0 {
			return &ir.UnaryExpr{Operator: ir.UnaryNegate, Operand: &ir.LiteralExpr{Kind: ir.LiteralInt, IntValue: -n}}
		}
		return &ir.LiteralExpr{Kind: ir.LiteralInt, IntValue: n}
	case types.TYPE_FLOAT:
		f := value.Float()
		if math.Signbit(f) {
			return &ir.UnaryExpr{Operator: ir.UnaryNegate, Operand: &ir.LiteralExpr{Kind: ir.LiteralFloat, FloatValue: -f}}
		}
		return &ir.LiteralExpr{Kind: ir.LiteralFloat, FloatValue: f}
	case types.TYPE_OBJ:
		return &ir.LiteralExpr{Kind: ir.LiteralObj, ObjID: int64(value.Obj())}
	case types.TYPE_ERR:
		return &ir.LiteralExpr{Kind: ir.LiteralErr, ErrorName: value.Code().String()}
	}
	b.fail("a value of type %s cannot be written as a literal", value.Type().ToastName())
	b.atKey("value")
	return nil
}

func (b *astBuilder) operator(node types.Value, arity int) ir.Expr {
	op, ok := b.findRequired(node, "op")
	if !ok {
		return nil
	}
	if arity == 1 {
		var unary ir.UnaryOperator
		if op.Type() == types.TYPE_STR {
			unary, ok = parser.UnaryOperatorFromSpelling(op.Str())
		} else {
			ok = false
		}
		if !ok {
			b.fail("not a unary operator")
			b.atKey("op")
			return nil
		}
		operand := b.exprField(node, "expr")
		if operand == nil {
			return nil
		}
		return &ir.UnaryExpr{Operator: unary, Operand: operand}
	}
	var binary ir.BinaryOperator
	if op.Type() == types.TYPE_STR {
		binary, ok = parser.BinaryOperatorFromSpelling(op.Str())
	} else {
		ok = false
	}
	if !ok {
		b.fail("not a binary operator")
		b.atKey("op")
		return nil
	}
	lhs := b.exprField(node, "lhs")
	if lhs == nil {
		return nil
	}
	rhs := b.exprField(node, "rhs")
	if rhs == nil {
		return nil
	}
	return &ir.BinaryExpr{Left: lhs, Operator: binary, Right: rhs}
}

// staticName is the bare name a property or verb name expression stands for
// when it is a string literal with identifier syntax.
func astStaticName(expr ir.Expr) (string, bool) {
	if lit, ok := expr.(*ir.LiteralExpr); ok && lit.Kind == ir.LiteralString && parser.IsIdentifier(lit.StringValue) {
		return lit.StringValue, true
	}
	return "", false
}

func (b *astBuilder) prop(node types.Value) ir.Expr {
	object := b.exprField(node, "object")
	if object == nil {
		return nil
	}
	property := b.exprField(node, "property")
	if property == nil {
		return nil
	}
	if name, ok := astStaticName(property); ok {
		return &ir.PropertyExpr{Expr: object, Property: name}
	}
	return &ir.PropertyExpr{Expr: object, PropertyExpr: property}
}

func (b *astBuilder) verbCall(node types.Value) ir.Expr {
	object := b.exprField(node, "object")
	if object == nil {
		return nil
	}
	verb := b.exprField(node, "verb")
	if verb == nil {
		return nil
	}
	args, ok := b.argsField(node, "args")
	if !ok {
		return nil
	}
	if name, ok := astStaticName(verb); ok {
		return &ir.VerbCallExpr{Expr: object, Verb: name, Args: args}
	}
	return &ir.VerbCallExpr{Expr: object, VerbExpr: verb, Args: args}
}

func (b *astBuilder) index(node types.Value) ir.Expr {
	base := b.exprField(node, "base")
	if base == nil {
		return nil
	}
	index := b.exprField(node, "index")
	if index == nil {
		return nil
	}
	return &ir.IndexExpr{Expr: base, Index: index}
}

func (b *astBuilder) rangeExpr(node types.Value) ir.Expr {
	base := b.exprField(node, "base")
	if base == nil {
		return nil
	}
	from := b.exprField(node, "from")
	if from == nil {
		return nil
	}
	to := b.exprField(node, "to")
	if to == nil {
		return nil
	}
	return &ir.RangeExpr{Expr: base, Start: from, End: to}
}

func (b *astBuilder) scatterItem(item types.Value) ir.Binding {
	if !b.checkType(item, types.TYPE_MAP) {
		return nil
	}
	kind, ok := b.findRequired(item, "kind")
	if !ok {
		return nil
	}
	kindName := ""
	if kind.Type() == types.TYPE_STR {
		kindName = kind.Str()
	}
	if kindName != "required" && kindName != "optional" && kindName != "rest" {
		b.fail("expected \"required\", \"optional\" or \"rest\"")
		b.atKey("kind")
		return nil
	}
	name, ok := b.nameField(item, "var", false)
	if !ok {
		return nil
	}
	value, ok := b.optionalExprField(item, "default")
	if !ok {
		return nil
	}
	if value != nil && kindName != "optional" {
		b.fail("only an optional target can have a default")
		return nil
	}
	switch kindName {
	case "required":
		return &ir.RequiredBinding{Name: name}
	case "rest":
		return &ir.RestBinding{Name: name}
	}
	return &ir.OptionalBinding{Name: name, Default: value}
}

func (b *astBuilder) scatter(node types.Value) ir.Target {
	items, ok := b.listField(node, "items")
	if !ok {
		return nil
	}
	bindings := make([]ir.Binding, 0, items.Len())
	for i := 1; i <= items.Len(); i++ {
		binding := b.scatterItem(items.Get(i))
		if binding == nil {
			b.atIndex(i)
			b.atKey("items")
			return nil
		}
		bindings = append(bindings, binding)
	}
	return &ir.DestructuringTarget{Bindings: bindings}
}

// assignTarget is the parser's test of an assignment's left side (a variable,
// a property, or an index or range of one of those, fork assignable()), made
// into the target it denotes.
func assignTarget(expr ir.Expr) ir.Target {
	if r, ok := expr.(*ir.RangeExpr); ok {
		collection := collectionTarget(r.Expr)
		if collection == nil {
			return nil
		}
		return &ir.RangeTarget{Collection: collection, Start: r.Start, End: r.End}
	}
	return collectionTarget(expr)
}

func collectionTarget(expr ir.Expr) ir.CollectionTarget {
	switch e := expr.(type) {
	case *ir.IdentifierExpr:
		return &ir.VariableTarget{Name: e.Name}
	case *ir.PropertyExpr:
		return &ir.PropertyTarget{Object: e.Expr, Name: e.Property, NameExpr: e.PropertyExpr}
	case *ir.IndexExpr:
		collection := collectionTarget(e.Expr)
		if collection == nil {
			return nil
		}
		return &ir.IndexTarget{Collection: collection, Index: e.Index}
	}
	return nil
}

func (b *astBuilder) assign(node types.Value) ir.Expr {
	targetNode, ok := b.findRequired(node, "target")
	if !ok {
		return nil
	}
	var target ir.Target
	if astIsNode(targetNode, "scatter") {
		target = b.scatter(targetNode)
	} else if expr := b.expr(targetNode); expr != nil {
		if target = assignTarget(expr); target == nil {
			b.fail("cannot be assigned to")
		}
	}
	if target == nil {
		b.atKey("target")
		return nil
	}
	value := b.exprField(node, "value")
	if value == nil {
		return nil
	}
	return &ir.AssignExpr{Target: target, Value: value}
}

func (b *astBuilder) call(node types.Value) ir.Expr {
	name, ok := b.findRequired(node, "function")
	if !ok {
		return nil
	}
	function := ""
	if name.Type() == types.TYPE_STR {
		function = strings.ToLower(name.Str())
	}
	if _, known := b.registry.GetID(function); !known {
		b.fail("not a built-in function")
		b.atKey("function")
		return nil
	}
	args, ok := b.argsField(node, "args")
	if !ok {
		return nil
	}
	return &ir.BuiltinCallExpr{Name: function, Args: args}
}

func (b *astBuilder) list(node types.Value) ir.Expr {
	items, ok := b.argsField(node, "items")
	if !ok {
		return nil
	}
	return &ir.ListExpr{Elements: items}
}

func (b *astBuilder) mapExpr(node types.Value) ir.Expr {
	entries, ok := b.listField(node, "entries")
	if !ok {
		return nil
	}
	pairs := make([]ir.MapPair, 0, entries.Len())
	for i := 1; i <= entries.Len(); i++ {
		entry := entries.Get(i)
		var key, value ir.Expr
		if b.checkType(entry, types.TYPE_MAP) {
			if key = b.exprField(entry, "key"); key != nil {
				value = b.exprField(entry, "value")
			}
		}
		if value == nil {
			b.atIndex(i)
			b.atKey("entries")
			return nil
		}
		pairs = append(pairs, ir.MapPair{Key: key, Value: value})
	}
	return &ir.MapExpr{Pairs: pairs}
}

func (b *astBuilder) conditional(node types.Value) ir.Expr {
	condition := b.exprField(node, "condition")
	if condition == nil {
		return nil
	}
	consequent := b.exprField(node, "consequent")
	if consequent == nil {
		return nil
	}
	alternate := b.exprField(node, "alternate")
	if alternate == nil {
		return nil
	}
	return &ir.TernaryExpr{Condition: condition, ThenExpr: consequent, ElseExpr: alternate}
}

func (b *astBuilder) catch(node types.Value) ir.Expr {
	inner := b.exprField(node, "expr")
	if inner == nil {
		return nil
	}
	codes, isAny, ok := b.codesField(node)
	if !ok {
		return nil
	}
	value, ok := b.optionalExprField(node, "default")
	if !ok {
		return nil
	}
	return &ir.CatchExpr{Expr: inner, Codes: codes, IsAny: isAny, Default: value}
}

func (b *astBuilder) expr(value types.Value) ir.Expr {
	nodeType, ok := b.nodeType(value)
	if !ok {
		return nil
	}
	var expr ir.Expr
	if b.depth++; b.depth > astMaxDepth {
		b.fail("nested more than %d levels deep", astMaxDepth)
	} else {
		switch nodeType {
		case "literal":
			expr = b.literal(value)
		case "variable":
			if name, ok := b.nameField(value, "name", false); ok {
				expr = &ir.IdentifierExpr{Name: name}
			}
		case "binary":
			expr = b.operator(value, 2)
		case "unary":
			expr = b.operator(value, 1)
		case "prop":
			expr = b.prop(value)
		case "verb_call":
			expr = b.verbCall(value)
		case "index":
			expr = b.index(value)
		case "range":
			expr = b.rangeExpr(value)
		case "assign":
			expr = b.assign(value)
		case "call":
			expr = b.call(value)
		case "list":
			expr = b.list(value)
		case "map":
			expr = b.mapExpr(value)
		case "conditional":
			expr = b.conditional(value)
		case "catch":
			expr = b.catch(value)
		case "first":
			expr = &ir.IndexBoundaryExpr{Boundary: ir.IndexFirst}
		case "last":
			expr = &ir.IndexBoundaryExpr{Boundary: ir.IndexLast}
		case "scatter":
			b.fail("a scatter can only be the target of an assignment")
		case "splice":
			b.fail("a splice can only appear in an argument list or a list")
		default:
			b.fail("\"%s\" is not an expression", nodeType)
		}
	}
	b.depth--
	return expr
}

func (b *astBuilder) ifStmt(node types.Value) ir.Stmt {
	arms, ok := b.listField(node, "arms")
	if !ok {
		return nil
	}
	if arms.Len() == 0 {
		b.fail("must not be empty")
		b.atKey("arms")
		return nil
	}
	chain := make([]*ir.IfStmt, 0, arms.Len())
	for i := 1; i <= arms.Len(); i++ {
		arm := arms.Get(i)
		var condition ir.Expr
		var body []ir.Stmt
		ok := b.checkType(arm, types.TYPE_MAP)
		if ok {
			condition = b.exprField(arm, "condition")
			ok = condition != nil
		}
		if ok {
			body, ok = b.bodyField(arm, "body", false)
		}
		if !ok {
			b.atIndex(i)
			b.atKey("arms")
			return nil
		}
		chain = append(chain, &ir.IfStmt{Condition: condition, Body: body})
	}
	otherwise, ok := b.bodyField(node, "else", true)
	if !ok {
		return nil
	}
	chain[len(chain)-1].Else = otherwise
	for i := len(chain) - 1; i > 0; i-- {
		chain[i-1].Else = []ir.Stmt{chain[i]}
		b.elseIfArms[chain[i]] = true
	}
	return chain[0]
}

func (b *astBuilder) forStmt(node types.Value) ir.Stmt {
	value, ok := b.nameField(node, "var", false)
	if !ok {
		return nil
	}
	index, ok := b.nameField(node, "index", true)
	if !ok {
		return nil
	}
	collection := b.exprField(node, "expr")
	if collection == nil {
		return nil
	}
	body, ok := b.bodyField(node, "body", false)
	if !ok {
		return nil
	}
	return &ir.CollectionLoopStmt{Value: value, Index: index, Collection: collection, Body: body}
}

func (b *astBuilder) forRangeStmt(node types.Value) ir.Stmt {
	value, ok := b.nameField(node, "var", false)
	if !ok {
		return nil
	}
	from := b.exprField(node, "from")
	if from == nil {
		return nil
	}
	to := b.exprField(node, "to")
	if to == nil {
		return nil
	}
	body, ok := b.bodyField(node, "body", false)
	if !ok {
		return nil
	}
	return &ir.RangeLoopStmt{Value: value, Start: from, End: to, Body: body}
}

func (b *astBuilder) whileStmt(node types.Value) ir.Stmt {
	label, ok := b.nameField(node, "name", true)
	if !ok {
		return nil
	}
	condition := b.exprField(node, "condition")
	if condition == nil {
		return nil
	}
	body, ok := b.bodyField(node, "body", false)
	if !ok {
		return nil
	}
	return &ir.WhileStmt{Label: label, Condition: condition, Body: body}
}

func (b *astBuilder) forkStmt(node types.Value) ir.Stmt {
	name, ok := b.nameField(node, "var", true)
	if !ok {
		return nil
	}
	delay := b.exprField(node, "delay")
	if delay == nil {
		return nil
	}
	body, ok := b.bodyField(node, "body", false)
	if !ok {
		return nil
	}
	return &ir.ForkStmt{VarName: name, Delay: delay, Body: body}
}

func (b *astBuilder) tryExceptStmt(node types.Value) ir.Stmt {
	body, ok := b.bodyField(node, "body", false)
	if !ok {
		return nil
	}
	excepts, ok := b.listField(node, "excepts")
	if !ok {
		return nil
	}
	if excepts.Len() == 0 {
		b.fail("must not be empty")
		b.atKey("excepts")
		return nil
	}
	handlers := make([]ir.ExceptionHandler, 0, excepts.Len())
	for i := 1; i <= excepts.Len(); i++ {
		except := excepts.Get(i)
		var handler ir.ExceptionHandler
		ok := b.checkType(except, types.TYPE_MAP)
		if ok {
			handler.Variable, ok = b.nameField(except, "var", true)
		}
		if ok {
			handler.Codes, handler.IsAny, ok = b.codesField(except)
		}
		if ok {
			handler.Body, ok = b.bodyField(except, "body", false)
		}
		if !ok {
			b.atIndex(i)
			b.atKey("excepts")
			return nil
		}
		handlers = append(handlers, handler)
	}
	return &ir.TryStmt{Body: body, Handlers: handlers}
}

func (b *astBuilder) tryFinallyStmt(node types.Value) ir.Stmt {
	body, ok := b.bodyField(node, "body", false)
	if !ok {
		return nil
	}
	handler, ok := b.bodyField(node, "finally", false)
	if !ok {
		return nil
	}
	return &ir.TryStmt{Body: body, Finalizer: &ir.Finalizer{Body: handler}}
}

func (b *astBuilder) stmt(value types.Value) ir.Stmt {
	nodeType, ok := b.nodeType(value)
	if !ok {
		return nil
	}
	var stmt ir.Stmt
	if b.depth++; b.depth > astMaxDepth {
		b.fail("nested more than %d levels deep", astMaxDepth)
	} else {
		switch nodeType {
		case "if":
			stmt = b.ifStmt(value)
		case "for":
			stmt = b.forStmt(value)
		case "for_range":
			stmt = b.forRangeStmt(value)
		case "while":
			stmt = b.whileStmt(value)
		case "fork":
			stmt = b.forkStmt(value)
		case "expr":
			if expr := b.exprField(value, "expr"); expr != nil {
				stmt = &ir.ExprStmt{Expr: expr}
			}
		case "return":
			if result, ok := b.optionalExprField(value, "value"); ok {
				stmt = &ir.ReturnStmt{Value: result}
			}
		case "try_except":
			stmt = b.tryExceptStmt(value)
		case "try_finally":
			stmt = b.tryFinallyStmt(value)
		case "break", "continue":
			if label, ok := b.nameField(value, "name", true); ok {
				if nodeType == "break" {
					stmt = &ir.BreakStmt{Label: label}
				} else {
					stmt = &ir.ContinueStmt{Label: label}
				}
			}
		default:
			b.fail("\"%s\" is not a statement", nodeType)
		}
	}
	b.depth--
	return stmt
}

func (b *astBuilder) stmts(list types.Value) ([]ir.Stmt, bool) {
	stmts := make([]ir.Stmt, 0, list.Len())
	for i := 1; i <= list.Len(); i++ {
		stmt := b.stmt(list.Get(i))
		if stmt == nil {
			b.atIndex(i)
			return nil, false
		}
		stmts = append(stmts, stmt)
	}
	return stmts, true
}

// program builds the statements of a tree, or reports false with b.err set.
func (b *astBuilder) program(tree types.Value) (*ir.Program, bool) {
	if !b.checkType(tree, types.TYPE_LIST) {
		return nil, false
	}
	stmts, ok := b.stmts(tree)
	if !ok {
		return nil, false
	}
	return &ir.Program{Statements: stmts}, true
}

// builtinValidateAST: validate_ast(ast) -> 1 if unparse_ast() would accept the
// tree, otherwise 0.
func builtinValidateAST(ctx *Execution, args []types.Value) types.Result {
	if _, ok := newASTBuilder(ctx.Registry).program(args[0]); ok {
		return types.Ok(types.NewInt(1))
	}
	return types.Ok(types.NewInt(0))
}

// builtinUnparseAST: unparse_ast(LIST ast [, fully-paren [, indent]]) -> LIST,
// the code for a tree, laid out as verb_code() lays out code. A malformed tree
// raises E_INVARG with a message saying where, and 0 as the value.
func builtinUnparseAST(ctx *Execution, args []types.Value) types.Result {
	b := newASTBuilder(ctx.Registry)
	program, ok := b.program(args[0])
	if !ok {
		return astRaise(b.message(), types.NewInt(0))
	}
	lines, err := parser.FormatUncheckedWith(program, parser.FormatOptions{
		FullyParenthesized: len(args) >= 2 && args[1].Truthy(),
		Unindented:         len(args) >= 3 && !args[2].Truthy(),
		ElseIfArms:         b.elseIfArms,
	})
	if err != nil {
		return astRaise(err.Error(), types.NewInt(0))
	}
	code := make([]types.Value, len(lines))
	for i, line := range lines {
		code[i] = types.NewStr(line)
	}
	return types.Ok(types.NewList(code))
}

// builtinVerbAST: verb_ast(object, verb-desc) -> LIST, the tree of the verb's
// code, with verb_code()'s argument checks and errors.
func builtinVerbAST(ctx *Execution, args []types.Value) types.Result {
	verb, errCode := readableDescribedVerb(ctx, args[0], args[1])
	if errCode != types.E_NONE {
		return types.Err(errCode)
	}
	return sourceAST(verb.Code)
}

func astDescriptors() []Descriptor {
	return []Descriptor{
		function("parse_ast", builtinParseAST, 1, 1, 4),
		function("verb_ast", builtinVerbAST, 2, 2, -1, -1),
		function("validate_ast", builtinValidateAST, 1, 1, -1),
		function("unparse_ast", builtinUnparseAST, 1, 3, 4, -1, -1),
	}
}
