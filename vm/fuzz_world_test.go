package vm

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/config"
	dbformat "github.com/MongooseMoo/barn/db/format"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// The populated world the source-execution lanes run in. It starts from the
// checkpoint-resume store (#0 and #2, wizard players) and adds, as the
// wizard #0:
//
//	#4 root     the ancestor of every other valid object; defines p (1),
//	            secret (unreadable), open (the programmer's) and the waif
//	            property :wp
//	#5 middle   child of #4; overrides p (2)
//	#3 leaf     child of #5, owned by the programmer, fertile, inside #6;
//	            inherits p from #5 and defines mine
//	#6          the programmer player, child of #4, in the room #2
//
// #0 is a child of #4 and #2 of #5, so every object the structured generator
// names (#0 to #3) dispatches its verb names v, look, foo and initialize. #1
// is a hole, as a recycled slot is. The verbs call each other, pass() up the
// chain, raise, catch, recurse, create waifs and run with either owner's
// permissions.
const (
	sourceFuzzWizard     = types.ObjID(0)
	sourceFuzzProgrammer = types.ObjID(6)
)

var sourceFuzzWorldObjects = []string{
	`delete_property(#0, "p");`,
	`delete_property(#2, "p");`,
	`leaf = create(#-1);`,
	`root = create(#-1);`,
	`middle = create(root);`,
	`programmer = create(root);`,
	`chparent(leaf, middle);`,
	`chparent(#0, root);`,
	`chparent(#2, middle);`,
	`root.name = "root"; middle.name = "middle"; leaf.name = "leaf"; programmer.name = "programmer";`,
	`set_player_flag(programmer, 1);`,
	`programmer.programmer = 1;`,
	`programmer.owner = programmer;`,
	`leaf.owner = programmer;`,
	`leaf.f = 1;`,
	`add_property(root, "p", 1, {#0, "rc"});`,
	`add_property(root, "secret", "hidden", {#0, ""});`,
	`add_property(root, "open", {}, {programmer, "rw"});`,
	`add_property(root, ":wp", 0, {#0, "rw"});`,
	`middle.p = 2;`,
	`add_property(leaf, "mine", {1, "two"}, {programmer, "rw"});`,
	`set_property_info(leaf, "open", {programmer, "r"});`,
	`move(programmer, #2);`,
	`move(leaf, programmer);`,
}

// sourceFuzzWorldVerbs are added after the objects, so create() above runs
// no initialize verb. owner is a variable of the setup program.
var sourceFuzzWorldVerbs = []struct {
	object, owner, perms, names string
	code                        []string
}{
	{"root", "#0", "rxd", "v", []string{`return {"root", this, player, @args};`}},
	{"root", "#0", "rxd", "foo", []string{`raise(E_INVARG, "root foo", args);`}},
	{"root", "#0", "rxd", "look", []string{`w = new_waif();`, `w.wp = this.p;`, `return {this.p, w.wp, w.owner};`}},
	{"root", "#0", "rxd", "initialize", []string{`this.open = {@this.open, caller};`, `return this:chain(@args);`}},
	{"root", "#0", "rxd", "chain", []string{`return this:v(@args);`}},
	{"root", "programmer", "rx", "set_p", []string{`this.p = args[1];`, `return this.p;`}},
	{"root", "#0", "rd", "hidden", []string{`return "hidden";`}},
	{"root", "#0", "rxd", "w*rite", []string{`return verb;`}},
	{"root", "#0", "rxd", "deep", []string{`return args ? this:deep(@args[2..$]) | callers();`}},
	{"middle", "#0", "rxd", "v", []string{`return {"middle", @pass(@args)};`}},
	{"middle", "#0", "rxd", "foo", []string{`try`, `return pass(@args);`, `except e (ANY)`, `return {"caught", e[1], e[2]};`, `endtry`}},
	{"middle", "programmer", "rxd", "look", []string{`return {this.p, this.secret};`}},
	{"leaf", "programmer", "rxd", "v", []string{`return {"leaf", @pass(@args)};`}},
	{"leaf", "programmer", "rxd", "initialize", []string{`return {eval("return player;"), pass(@args)};`}},
	{"leaf", "programmer", "rxd", "foo", []string{`return {this.mine, pass(@args)};`}},
	{"leaf", "programmer", "rxd", "recurse", []string{`return this:recurse();`}},
	{"leaf", "programmer", "rxd", "e", []string{`return eval("return {player, caller, this};");`}},
}

// sourceFuzzWorldSetup returns the MOO program that builds the world.
func sourceFuzzWorldSetup() []string {
	lines := append([]string(nil), sourceFuzzWorldObjects...)
	for _, verb := range sourceFuzzWorldVerbs {
		code := make([]string, len(verb.code))
		for i, line := range verb.code {
			code[i] = strconv.Quote(line)
		}
		lines = append(lines,
			"add_verb("+verb.object+", {"+verb.owner+", "+strconv.Quote(verb.perms)+", "+strconv.Quote(verb.names)+`}, {"this", "none", "this"});`,
			// A name with a * wildcard is matched by its full spelling.
			"set_verb_code("+verb.object+", "+strconv.Quote(strings.ReplaceAll(verb.names, "*", ""))+", {"+strings.Join(code, ", ")+"});",
		)
	}
	return append(lines, "return 1;")
}

// sourceFuzzWorldSeeds are raw-source seeds that call every world verb on #3,
// which inherits them all, and read and write its properties. #3:look runs
// the middle's override, so #4:look reaches the root's waif verb.
func sourceFuzzWorldSeeds() []string {
	seeds := []string{
		`return {#3.p, #3.secret, #3.open, #3.mine, #3.name, #6.programmer};`,
		`#3.p = 5; #3.mine = {}; return {#3:set_p(4), #3.p, #5.p};`,
		`return #4:look();`,
	}
	seen := make(map[string]bool)
	for _, verb := range sourceFuzzWorldVerbs {
		name := strings.ReplaceAll(verb.names, "*", "")
		if !seen[name] {
			seen[name] = true
			seeds = append(seeds, `return #3:`+name+`(1, "a");`)
		}
	}
	return seeds
}

// sourceFuzzWorld is the world built once per process and saved as a
// database. Each execution loads its own store from it, so no execution sees
// another's writes, creations, waifs or recycles.
type sourceFuzzWorld struct {
	path string
}

func newSourceFuzzWorld(tb testing.TB) sourceFuzzWorld {
	tb.Helper()
	registry, err := builtins.NewRegistryFromDescriptors(config.Core, Descriptors())
	if err != nil {
		tb.Fatal(err)
	}
	program, diagnostics := registry.Compiler().CompileMOO(sourceFuzzWorldSetup())
	if len(diagnostics) != 0 {
		tb.Fatalf("world setup does not compile: %v", diagnostics)
	}
	session := newTestSessionWithTaskManager(registry)
	defer func() {
		if err := session.Close(); err != nil {
			tb.Error(err)
		}
	}()
	store := newCheckpointResumeStore(tb)
	machine := newSourceFuzzVM(store, session, 1, defaultTickLimit)
	runAs(machine, sourceFuzzWizard, true)
	if result := machine.Run(program); result.Flow != types.FlowReturn {
		tb.Fatalf("world setup ended with flow %v error %v: %v", result.Flow, result.Error, result.Val)
	}
	path := filepath.Join(tb.TempDir(), "world.db")
	if err := dbformat.WriteCheckpoint(path, store, nil, nil, nil); err != nil {
		tb.Fatal(err)
	}
	return sourceFuzzWorld{path: path + ".new"}
}

// fresh returns a new store holding the world.
func (world sourceFuzzWorld) fresh(tb testing.TB) *dbstore.Store {
	tb.Helper()
	database, err := dbformat.LoadDatabaseReadOnly(world.path)
	if err != nil {
		tb.Fatal(err)
	}
	store, err := database.NewStoreFromDatabase()
	if err != nil {
		tb.Fatal(err)
	}
	return store
}

// sourceFuzzPrincipal is who an input's root task runs as.
type sourceFuzzPrincipal struct {
	name   string
	player types.ObjID
	wizard bool
}

// Every input runs once as each.
var sourceFuzzPrincipals = []sourceFuzzPrincipal{
	{name: "wizard", player: sourceFuzzWizard, wizard: true},
	{name: "programmer", player: sourceFuzzProgrammer},
}

// runSourceFuzzWorld compiles source and runs it in a fresh copy of world.
func runSourceFuzzWorld(t *testing.T, world sourceFuzzWorld, source string, principal sourceFuzzPrincipal) []types.Result {
	t.Helper()
	registry, program, diagnostics := compileFuzzSource(t, source)
	if len(diagnostics) != 0 {
		t.Fatalf("%s does not compile: %v", source, diagnostics)
	}
	return runSourceFuzzProgram(t, world, registry, program, principal)
}

// The source-execution lanes must reach real verb dispatch, inherited and
// overridden properties, pass() and permission checks, not stop at the first
// object lookup of an empty store.
func TestSourceFuzzWorldDispatchesVerbsAndPass(t *testing.T) {
	world := newSourceFuzzWorld(t)
	for _, check := range []struct {
		source     string
		wizard     string
		programmer string
	}{
		{`return #3:v(7);`, `{"leaf", "middle", "root", #3, #0, 7}`, `{"leaf", "middle", "root", #3, #6, 7}`},
		{`return {#0.p, #4.p, #5.p, #3.p};`, `{1, 1, 2, 2}`, `{1, 1, 2, 2}`},
		{`return #3.secret;`, `"hidden"`, `E_PERM`},
		{`return #3:foo();`, `{{1, "two"}, {"caught", E_INVARG, "root foo"}}`, `{{1, "two"}, {"caught", E_INVARG, "root foo"}}`},
		{`return #3:hidden();`, `E_VERBNF`, `E_VERBNF`},
		{`return #3:initialize();`, `{{1, #0}, {"leaf", "middle", "root", #3, #0}}`, `{{1, #6}, {"leaf", "middle", "root", #3, #6}}`},
	} {
		for _, principal := range sourceFuzzPrincipals {
			want := check.programmer
			if principal.wizard {
				want = check.wizard
			}
			results := runSourceFuzzWorld(t, world, check.source, principal)
			if got := sourceFuzzOutcome(results[0]); got != want {
				t.Errorf("%s as %s: got %s, want %s", check.source, principal.name, got, want)
			}
		}
	}
}

// The raw lane's world seeds call every world verb through #3, which inherits
// them all, so mutation starts from inputs that reach each of them.
func TestSourceFuzzWorldSeedsReachEveryVerb(t *testing.T) {
	world := newSourceFuzzWorld(t)
	seeds := sourceFuzzWorldSeeds()
	for _, verb := range sourceFuzzWorldVerbs {
		call := "#3:" + strings.ReplaceAll(verb.names, "*", "") + "("
		found := false
		for _, seed := range seeds {
			found = found || strings.Contains(seed, call)
		}
		if !found {
			t.Errorf("no seed calls %s", call)
		}
	}
	for _, seed := range seeds {
		for _, principal := range sourceFuzzPrincipals {
			result := runSourceFuzzWorld(t, world, seed, principal)[0]
			if result.Flow == types.FlowException && result.Error == types.E_VERBNF && !strings.Contains(seed, ":hidden(") {
				t.Errorf("%s as %s: E_VERBNF", seed, principal.name)
			}
		}
	}
}

// Each execution starts from the same world: a write, a create or a recycle
// in one input is not seen by the next.
func TestSourceFuzzWorldExecutionsDoNotShareState(t *testing.T) {
	world := newSourceFuzzWorld(t)
	wizard := sourceFuzzPrincipals[0]
	first := runSourceFuzzWorld(t, world, `#3.p = 99; create(#4); recycle(#5); return max_object();`, wizard)
	if got := sourceFuzzOutcome(first[0]); got != "#7" {
		t.Fatalf("first execution returned %s, want #7", got)
	}
	second := runSourceFuzzWorld(t, world, `return {#3.p, max_object(), valid(#5)};`, wizard)
	if got := sourceFuzzOutcome(second[0]); got != "{2, #6, 1}" {
		t.Fatalf("second execution saw %s, want {2, #6, 1}", got)
	}
}

// sourceFuzzOutcome renders a task's result as its returned value or its
// error code.
func sourceFuzzOutcome(result types.Result) string {
	if result.Flow == types.FlowException {
		return types.NewErr(result.Error).String()
	}
	return result.Val.String()
}
