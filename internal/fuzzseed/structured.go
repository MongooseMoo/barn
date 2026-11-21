package fuzzseed

import (
	"slices"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/config"
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
	errorNames      = []string{
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

// excludedBuiltins are never called. A fuzz process must not touch files,
// the network or the host process, change process-wide state, write the
// process log, wait for input or stop the server; and it must not turn a
// string into a call, which would reach these by name. Every file_* builtin
// is excluded too. eval and pass are VM descriptors, outside
// builtins.Descriptors().
var excludedBuiltins = map[string]bool{
	"db_disk_size":    true, // measures the database file
	"memory_usage":    true, // reads /proc/self/statm
	"getenv":          true, // reads the host environment
	"exec":            true, // starts a process
	"read_http":       true, // reads a network connection
	"read":            true, // waits for connection input
	"shutdown":        true,
	"dump_database":   true, // writes a checkpoint
	"reseed_random":   true, // reseeds the process-wide generator
	"run_gc":          true, // runs the Go collector for the whole process
	"server_log":      true, // writes the process log
	"log_cache_stats": true,
	"argon2":          true, // memory and time cost come from the arguments
	"argon2_verify":   true,
	"call_function":   true, // calls any builtin by name
}

type builtinShape struct {
	name      string
	signature *builtins.Signature
	sqlite    bool // a config.SQLite builtin, called by sqliteCall
}

// builtinShapes holds every public core builtin not excluded above, sorted by
// name, with the registry's argument signature, and then every SQLite
// builtin, sorted by name. The SQLite builtins come last so that a choice
// byte below the number of core builtins names the builtin it named before
// they were offered. stapleShapes, the value and
// suspend builtins the generator called before it drew from the registry,
// keep their own expression choice: FuzzCheckpointResumeEquivalence needs
// programs that suspend, and drawing suspend from the whole registry made
// them seven times rarer (51 instead of 351 of 15000 choice lists).
// Each stays registered in config.Core with the VM descriptors;
// structured_test.go checks that.
var builtinShapes, stapleShapes = coreBuiltinShapes(
	"length", "tostr", "toint", "tofloat", "typeof", "toliteral",
	"listappend", "listinsert", "listdelete", "listset", "setadd",
	"setremove", "is_member", "mapkeys", "mapvalues", "mapdelete",
	"maphaskey", "abs", "min", "max", "strsub", "index", "rindex",
	"strcmp", "equal", "reverse", "explode", "raise", "suspend", "valid",
	"parent", "children",
)

func coreBuiltinShapes(staples ...string) (all, staple []builtinShape) {
	var sqlite []builtinShape
	for _, d := range builtins.Descriptors() {
		if d.Visibility != builtins.Public || excludedBuiltins[d.Name] || strings.HasPrefix(d.Name, "file_") {
			continue
		}
		shape := builtinShape{name: d.Name, signature: d.Signature}
		switch d.Capability {
		case config.Core:
			all = append(all, shape)
			if slices.Contains(staples, d.Name) {
				staple = append(staple, shape)
			}
		case config.SQLite:
			shape.sqlite = true
			sqlite = append(sqlite, shape)
		}
	}
	byName := func(a, b builtinShape) int { return strings.Compare(a.name, b.name) }
	slices.SortFunc(all, byName)
	slices.SortFunc(staple, byName)
	slices.SortFunc(sqlite, byName)
	if len(staple) != len(staples) {
		panic("fuzzseed: a staple builtin is not an offered core builtin")
	}
	return append(all, sqlite...), staple
}

// SQLite programs. A program that calls a SQLite builtin gets a preamble
// and a postamble (sqliteWrap). Every database is ":memory:": sqlite_open is
// only ever given that literal path, so no program touches a file (an empty
// path would be a temporary file on disk). SQL text comes only from
// sqliteStatements, never from an expression, so no statement can ATTACH a
// file, write one, or run without bound (a recursive CTE, a table that
// doubles itself): a running statement is not stopped by the task's tick or
// seconds limits on either engine. The handle variable is sqliteHandleVar,
// which is never in scope, so a program reads the handle only as a SQLite
// builtin's argument.
const sqliteHandleVar = "db"

// sqliteStatements is the SQL the generated calls run: each is ASCII on one
// line, deterministic, and bounded. The preamble creates table t (a, b).
var sqliteStatements = []string{
	"SELECT * FROM t ORDER BY rowid",
	"SELECT a, b FROM t ORDER BY rowid",
	"SELECT typeof(a), typeof(b) FROM t ORDER BY rowid",
	"SELECT * FROM t WHERE a = ?",
	"SELECT count(*), sum(a), max(b) FROM t",
	"SELECT group_concat(b) FROM t",
	"INSERT INTO t VALUES (?, ?)",
	"INSERT INTO t (a) VALUES (?)",
	"INSERT INTO t VALUES (1, 'x')",
	"INSERT INTO t VALUES (?, ?) RETURNING a, b",
	"UPDATE t SET b = ? WHERE a = ?",
	"DELETE FROM t WHERE a = ?",
	"CREATE TABLE u (x INTEGER PRIMARY KEY, y TEXT)",
	"INSERT INTO u (y) VALUES (?)",
	"SELECT * FROM u ORDER BY x",
	"DROP TABLE t",
	"SELECT ?",
	"SELECT ?, ?",
	"SELECT typeof(?), quote(?)",
	"SELECT length(?), hex(?)",
	"SELECT CAST(? AS INTEGER), CAST(? AS REAL), CAST(? AS TEXT)",
	"SELECT ? = ?, ? IS NULL",
	"SELECT 1",
	"SELECT 1.5, 0.1 + 0.2, 1e300 * 1e10",
	"SELECT 'abc', '', NULL",
	"SELECT '#5', '12', ' 12', '1e3', '-0', '0x10'",
	"SELECT 2147483648, -9223372036854775808, 9223372036854775807 + 1",
	"SELECT abs(-9223372036854775807 - 1)",
	"SELECT x'4142', 'a' || char(10) || 'b'",
	"SELECT 1, 2 UNION ALL SELECT 3, 4",
	"VALUES (1, 'a'), (2, 'b')",
	"WITH c(x) AS (VALUES (1), (2)) SELECT x FROM c",
	"SELECT 'x' REGEXP 'x'",
	"PRAGMA user_version",
	"BEGIN",
	"COMMIT",
	"ROLLBACK",
	"SELECT 1; SELECT 2",
	"INSERT INTO t VALUES (2, 'y'); SELECT * FROM t",
	"SELECT * FROM missing",
	"SELEC 1",
	"",
}

// sqliteLimitNames are sqlite_limit's category names (Toast sqlite.cc
// bf_sqlite_limit), and one that is not.
var sqliteLimitNames = []string{
	"LIMIT_LENGTH", "LIMIT_SQL_LENGTH", "LIMIT_COLUMN", "LIMIT_EXPR_DEPTH",
	"LIMIT_COMPOUND_SELECT", "LIMIT_VDBE_OP", "LIMIT_FUNCTION_ARG",
	"LIMIT_ATTACHED", "LIMIT_LIKE_PATTERN_LENGTH", "LIMIT_VARIABLE_NUMBER",
	"LIMIT_TRIGGER_DEPTH", "LIMIT_WORKER_THREADS", "LIMIT_NONE",
}

// sqliteParamTypes are the types a bound parameter is drawn from: every
// value type a list literal can hold.
var sqliteParamTypes = []int64{0, 9, 2, 1, 3, 4, 10}

// Program builds one bounded MOO program from fuzz bytes. Each byte is one
// construction choice, so a mutation changes one decision and minimization
// shortens the decision list. Exhausted input reads as zero, which selects the
// first (smallest) alternative, so every input yields a complete program.
func Program(choices []byte) *ir.Program { return program(choices, false) }

// ASCIIProgram is Program(choices) with each non-ASCII string piece spelled
// as an ASCII one, from the same choices. The Toast differential compares
// this spelling: it excludes non-ASCII source, because Barn strings are
// Unicode and Toast strings are bytes, and "é" in stringPieces put about nine
// in ten generated programs out of its reach.
func ASCIIProgram(choices []byte) *ir.Program { return program(choices, true) }

func program(choices []byte, ascii bool) *ir.Program {
	g := &generator{choices: choices, budget: maxNodes, ascii: ascii}
	g.scope = append(g.scope, initializedVariables...)
	statements := []ir.Stmt{
		assign("n", g.intLiteral()),
		assign("f", g.floatLiteral()),
		assign("s", g.stringLiteral()),
		assign("l", &ir.ListExpr{Elements: []ir.Expr{g.intLiteral(), g.intLiteral(), g.stringLiteral()}}),
		assign("m", &ir.MapExpr{Pairs: []ir.MapPair{{Key: g.stringLiteral(), Value: g.intLiteral()}, {Key: g.intLiteral(), Value: g.stringLiteral()}}}),
		assign("o", g.objLiteral()),
	}
	body := g.block(maxTopLevel, 0)
	if g.sqlite {
		body = sqliteWrap(body)
	}
	return &ir.Program{Statements: append(statements, body...)}
}

// sqliteWrap runs body with sqliteHandleVar holding a new ":memory:"
// database that has table t (a, b), and closes every open handle after it,
// however body ends. The preamble first closes any handle an earlier program
// left open (one aborted by its tick or seconds limit never ran its
// postamble), so each program starts with exactly one open handle.
//
// set_thread_mode(0) comes first. Toast runs sqlite_query and sqlite_execute
// through background_thread (background.cc), which suspends the task unless
// the activation's thread mode is off; an eval activation starts with it on
// (execute.cc, RUN_ACTIV.threaded = DEFAULT_THREAD_MODE), and Toast's
// emergency mode, the differential's Toast lane, never resumes a suspended
// task. With it off both engines run the statement inline.
//
// Every SQLite call here is caught: a non-wizard gets E_PERM from each, and
// the program goes on with E_PERM as its handle. Toast's sqlite_close raises
// E_PERM for a handle a suspended statement still holds.
func sqliteWrap(body []ir.Stmt) []ir.Stmt {
	call := func(name string, args ...ir.Expr) *ir.BuiltinCallExpr {
		if args == nil {
			args = []ir.Expr{}
		}
		return &ir.BuiltinCallExpr{Name: name, Args: args}
	}
	caught := func(e ir.Expr) ir.Expr { return &ir.CatchExpr{Expr: e, IsAny: true} }
	handle := &ir.IdentifierExpr{Name: sqliteHandleVar}
	closeAll := func() ir.Stmt {
		return &ir.CollectionLoopStmt{
			Value:      sqliteHandleVar,
			Collection: &ir.CatchExpr{Expr: call("sqlite_handles"), IsAny: true, Default: &ir.ListExpr{}},
			Body:       []ir.Stmt{&ir.ExprStmt{Expr: caught(call("sqlite_close", handle))}},
		}
	}
	return []ir.Stmt{
		&ir.ExprStmt{Expr: call("set_thread_mode", &ir.LiteralExpr{Kind: ir.LiteralInt, IntValue: 0})},
		closeAll(),
		assign(sqliteHandleVar, caught(call("sqlite_open", &ir.LiteralExpr{Kind: ir.LiteralString, StringValue: ":memory:"}))),
		&ir.ExprStmt{Expr: caught(call("sqlite_query", handle, &ir.LiteralExpr{Kind: ir.LiteralString, StringValue: "CREATE TABLE t (a, b)"}))},
		&ir.TryStmt{Body: body, Finalizer: &ir.Finalizer{Body: []ir.Stmt{closeAll()}}},
	}
}

// Source formats Program(choices) with the production formatter.
func Source(choices []byte) (string, error) { return format(Program(choices)) }

// ASCIISource formats ASCIIProgram(choices) with the production formatter.
func ASCIISource(choices []byte) (string, error) { return format(ASCIIProgram(choices)) }

func format(program *ir.Program) (string, error) {
	lines, err := parser.FormatMOOChecked(program)
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
	indexArgs int  // > 0 inside index or range brackets, where ^ and $ are legal
	ascii     bool // spell non-ASCII string pieces in ASCII (ASCIIProgram)
	sqlite    bool // the program calls a SQLite builtin, so sqliteWrap wraps it
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
		g.unusedForLabel()
		collection := g.expr(1)
		g.bind(value)
		if index != "" {
			g.bind(index)
		}
		return &ir.CollectionLoopStmt{Value: value, Index: index, Collection: collection, Body: g.loopBody(depth, value, index)}
	case 5:
		value := g.pick(assignableNames)
		g.unusedForLabel()
		start, end := g.expr(1), g.expr(1)
		g.bind(value)
		return &ir.RangeLoopStmt{Value: value, Start: start, End: end, Body: g.loopBody(depth, value)}
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

// unusedForLabel draws the choices a for-loop label once took and discards
// them. Toast's for loop takes no label (parser.y: "for ID in", "for ID, ID
// in"), but drawing the same choices keeps recorded corpus inputs generating
// the programs they did, less the label.
func (g *generator) unusedForLabel() {
	_ = g.optionalName()
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
		finalizer := &ir.Finalizer{Body: g.block(maxBlockLen, depth+1)}
		if len(stmt.Handlers) == 0 {
			stmt.Finalizer = finalizer
			return stmt
		}
		// MOO has no try/except/finally (parser_stmt.go:parseTryStatement):
		// nest the try/except in a try/finally, which runs the same way.
		return &ir.TryStmt{Body: []ir.Stmt{stmt}, Finalizer: finalizer}
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
	choice := g.choose(18)
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
		return g.builtinCall(stapleShapes, depth+1)
	case 17:
		return g.builtinCall(builtinShapes, depth+1)
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
		return g.list(depth + 1)
	case 14:
		return g.mapLiteral(depth + 1)
	case 15:
		return g.variable()
	default:
		return &ir.IndexBoundaryExpr{Boundary: ir.IndexBoundary(g.choose(2))}
	}
}

// list builds a list expression. The parser leaves an empty list's elements
// nil, and an empty call argument list non-nil (parser.go:190, :373, :492).
func (g *generator) list(depth int) ir.Expr {
	elements := g.arguments(depth)
	if len(elements) == 0 {
		elements = nil
	}
	return &ir.ListExpr{Elements: elements}
}

// mapLiteral builds a map expression. There is no ir.ListRangeExpr: the
// parser rejects {a..b} list literals.
func (g *generator) mapLiteral(depth int) ir.Expr {
	var pairs []ir.MapPair
	for i, count := 0, g.choose(maxListLen+1); i < count; i++ {
		pairs = append(pairs, ir.MapPair{Key: g.expr(depth), Value: g.expr(depth)})
	}
	return &ir.MapExpr{Pairs: pairs}
}

// builtinCall calls one of shapes. Usually the arguments follow the
// registry's signature, so the call passes argument admission and runs the
// builtin's body; otherwise they are arbitrary, to reach E_ARGS and E_TYPE.
func (g *generator) builtinCall(shapes []builtinShape, depth int) *ir.BuiltinCallExpr {
	shape := shapes[g.choose(len(shapes))]
	if shape.sqlite {
		return g.sqliteCall(shape.name, depth)
	}
	if g.choose(4) == 0 {
		return &ir.BuiltinCallExpr{Name: shape.name, Args: g.arguments(depth)}
	}
	sig := shape.signature
	optional := maxListLen // a variadic tail
	if sig.MaxArgs >= 0 {
		optional = int(sig.MaxArgs - sig.MinArgs)
	}
	args := make([]ir.Expr, int(sig.MinArgs)+g.choose(optional+1))
	for i := range args {
		args[i] = g.typedArgument(g.argumentType(sig, i), depth)
	}
	return &ir.BuiltinCallExpr{Name: shape.name, Args: args}
}

// sqliteCall calls the named SQLite builtin. It never takes the arbitrary
// argument path: sqlite_open's path is always ":memory:" and SQL is always one
// of sqliteStatements (see sqliteHandleVar). The other arguments follow
// Toast's registration (sqlite.cc register_sqlite), with the handle usually
// sqliteHandleVar and sometimes any expression, to reach E_TYPE and E_INVARG.
func (g *generator) sqliteCall(name string, depth int) *ir.BuiltinCallExpr {
	g.sqlite = true
	str := func(value string) ir.Expr { return &ir.LiteralExpr{Kind: ir.LiteralString, StringValue: value} }
	var args []ir.Expr
	switch name {
	case "sqlite_open":
		args = []ir.Expr{str(":memory:")}
		if g.choose(2) == 1 {
			args = append(args, g.typedArgument(0, depth))
		}
	case "sqlite_handles":
		args = []ir.Expr{}
	case "sqlite_query":
		args = []ir.Expr{g.sqliteHandle(depth), str(g.pick(sqliteStatements))}
		if g.choose(2) == 1 {
			args = append(args, g.typedArgument(-3, depth))
		}
	case "sqlite_execute":
		args = []ir.Expr{g.sqliteHandle(depth), str(g.pick(sqliteStatements))}
		var params []ir.Expr
		for i, count := 0, g.choose(maxListLen+1); i < count; i++ {
			params = append(params, g.typedArgument(sqliteParamTypes[g.choose(len(sqliteParamTypes))], depth))
		}
		args = append(args, &ir.ListExpr{Elements: params})
	case "sqlite_limit":
		args = []ir.Expr{g.sqliteHandle(depth)}
		if g.choose(3) == 0 {
			args = append(args, g.typedArgument(0, depth))
		} else {
			args = append(args, str(g.pick(sqliteLimitNames)))
		}
		args = append(args, g.typedArgument(0, depth))
	default: // sqlite_close, sqlite_info, sqlite_interrupt, sqlite_last_insert_row_id
		args = []ir.Expr{g.sqliteHandle(depth)}
	}
	return &ir.BuiltinCallExpr{Name: name, Args: args}
}

// sqliteHandle is the handle argument of a SQLite call.
func (g *generator) sqliteHandle(depth int) ir.Expr {
	if g.choose(4) == 0 {
		return g.typedArgument(0, depth)
	}
	return &ir.IdentifierExpr{Name: sqliteHandleVar}
}

// argumentType is the type the signature admits at position i: -1 for any
// value, -2 for a number and -3 for a flag (INT or BOOL), as in
// builtins.Signature.
func (g *generator) argumentType(sig *builtins.Signature, i int) int64 {
	if i >= len(sig.ArgTypes) {
		if sig.VariadicType != nil {
			return *sig.VariadicType
		}
		return -1
	}
	if alternatives := sig.Alternatives[i]; len(alternatives) != 0 {
		if choice := g.choose(len(alternatives) + 1); choice > 0 {
			return alternatives[choice-1]
		}
	}
	return sig.ArgTypes[i]
}

// typedArgument is mostly a value of type typ, sometimes any expression.
func (g *generator) typedArgument(typ int64, depth int) ir.Expr {
	if g.choose(4) == 0 {
		return g.expr(depth)
	}
	switch typ {
	case 0:
		return g.intLiteral()
	case 1:
		return g.objLiteral()
	case 2:
		return g.stringLiteral()
	case 3:
		return g.errLiteral()
	case 4:
		return g.list(depth)
	case 9:
		return g.floatLiteral()
	case 10:
		return g.mapLiteral(depth)
	case -2:
		if g.choose(2) == 0 {
			return g.intLiteral()
		}
		return g.floatLiteral()
	case -3:
		// The parser reads true and false as variables, never as
		// ir.LiteralBool, so the generator spells them the same way.
		switch g.choose(4) {
		case 0:
			return &ir.IdentifierExpr{Name: "false"}
		case 1:
			return &ir.IdentifierExpr{Name: "true"}
		}
		return g.intLiteral()
	}
	return g.expr(depth)
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
		piece := g.pick(stringPieces)
		if g.ascii && piece == "é" {
			piece = "e"
		}
		value.WriteString(piece)
	}
	return &ir.LiteralExpr{Kind: ir.LiteralString, StringValue: value.String()}
}

func (g *generator) objLiteral() ir.Expr {
	return &ir.LiteralExpr{Kind: ir.LiteralObj, ObjID: objLiterals[g.choose(len(objLiterals))]}
}

func (g *generator) errLiteral() ir.Expr {
	return &ir.LiteralExpr{Kind: ir.LiteralErr, ErrorName: g.pick(errorNames)}
}
