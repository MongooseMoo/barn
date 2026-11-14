package builtins

import (
	"fmt"
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
	source   string            // the joined source the program was parsed from
	spelling map[string]string // upper-case variable name -> the program's spelling
}

// programAST is the fork's tree for source: what its decompiler gives for the
// compiled program, so `elseif' arms, folded negative literals and the
// program's variable spellings are reported, and a lone `;' is not.
func programAST(source string, program *ir.Program) (types.Value, error) {
	names, err := compiler.NativeVariableNames(program, astLanguageVersion)
	if err != nil {
		return types.Value{}, err
	}
	w := &astWriter{source: source, spelling: make(map[string]string, len(names))}
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

// isElseIfArm reports whether an IfStmt the parser lowered into an Else was
// written as an `elseif' arm rather than as a nested `if'.
func (w *astWriter) isElseIfArm(s *ir.IfStmt) bool {
	at := s.Pos.Offset
	return at >= 0 && at+6 <= len(w.source) && strings.EqualFold(w.source[at:at+6], "elseif")
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
				if next, ok := current.Else[0].(*ir.IfStmt); ok && w.isElseIfArm(next) {
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

func astDescriptors() []Descriptor {
	return []Descriptor{
		function("parse_ast", builtinParseAST, 1, 1, 4),
	}
}
