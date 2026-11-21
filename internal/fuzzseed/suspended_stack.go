package fuzzseed

import (
	"fmt"
	"strings"
)

// SuspendedStackKinds names the contexts SuspendedStack nests, in the order a
// layer's kind choice selects them.
var SuspendedStackKinds = []string{
	"verb", "eval", "fork", "except", "finally", "for-list", "for-range",
	"for-map", "while", "scatter", "expression",
	"in-finally", "move-accept", "move-exitfunc", "move-enterfunc",
	"create-initialize", "recreate-initialize", "recycle-recycle", "recycle-exitfunc",
}

// SuspendedStackVariants is the number of values of a layer's variant choice.
// in-finally uses all four, for how its handler was entered: the try body fell
// through, returned, raised an error an outer except catches, or left an
// enclosing loop with break. eval, scatter and expression read 1 as eval()
// and 2 as call_function("eval", ...); scatter and expression read 0 and 3
// as a verb call. The move, create, recreate and recycle kinds read 2 as
// calling their builtin through call_function. The other kinds ignore it.
const SuspendedStackVariants = 4

const (
	maxSuspendedStacks     = 3
	maxSuspendedStackDepth = 6

	// suspendedStackObject is the #0 property holding the test object, so
	// eval() source, verbs and fork bodies, which share no variables with the
	// setup, reach its verbs and properties.
	suspendedStackObject = "$suspended_stack"

	// suspendedStackSuspend is the call every suspension makes.
	suspendedStackSuspend = "suspend()"
)

// SuspendedStack generates a program whose tasks reach suspend() at the bottom
// of a chosen stack of nested contexts, then pass a value v back out through
// every layer. Each layer wraps v with its own number and the state it kept
// across the suspension (loop position, verb arguments, the handler's error,
// eval()'s {1, value}, a finally handler's pending return, error or break,
// move()'s remaining hooks), so a layer resumed with a wrong continuation
// changes the returned list or a property of the test object.
//
// A fork layer's child carries the inner layers: it stores their value in its
// own property f<layer>, and the parent suspends beside the queued child.
//
// The program must run as a wizard: its setup defines a property on #0, and
// move() then proceeds whatever accept returns (each accept returns 1 anyway).
//
// The choices are the number of stacks, then for each stack its depth, two
// bytes per layer from the outside in (kind, variant) and one for the form of
// the bottom suspension. Every construct is ToastStunt syntax and builtins,
// with no clock, random number or task id. Choice bytes replay only through
// this renderer.
func SuspendedStack(choices []byte) string {
	s := suspendedStack{g: generator{choices: choices}}
	var body []string
	for i, count := 0, 1+s.g.choose(maxSuspendedStacks); i < count; i++ {
		body = append(body, s.nest(1+s.g.choose(maxSuspendedStackDepth))...)
		body = append(body, "results = {@results, v};")
	}
	lines := []string{
		"o = create(#-1);",
		`add_property(#0, "suspended_stack", o, {o.owner, "r"});`,
	}
	lines = append(lines, s.setup...)
	lines = append(lines, "results = {};")
	lines = append(lines, body...)
	lines = append(lines, "return results;")
	return strings.Join(lines, "\n")
}

type suspendedStack struct {
	g      generator
	layers int      // layers rendered so far; each layer's number is unique in the program
	setup  []string // statements defining the layers' verbs, properties and objects
}

// verb installs code as a verb of object, a setup variable.
func (s *suspendedStack) verb(object, name string, code ...string) {
	quoted := make([]string, len(code))
	for i, line := range code {
		quoted[i] = mooString(line)
	}
	s.setup = append(s.setup,
		fmt.Sprintf(`add_verb(%s, {o.owner, "rxd", "%s"}, {"this", "none", "this"});`, object, name),
		fmt.Sprintf(`c = set_verb_code(%s, "%s", {%s});`, object, name, strings.Join(quoted, ", ")),
		fmt.Sprintf(`if (c) raise(E_INVARG, "%s does not compile", c); endif`, name))
}

// property defines a property of the test object and returns an expression
// for it that is valid in any frame.
func (s *suspendedStack) property(name, value string) string {
	s.setup = append(s.setup, fmt.Sprintf(`add_property(o, "%s", %s, {o.owner, "rw"});`, name, value))
	return suspendedStackObject + "." + name
}

// nest returns statements that run depth nested layers around one suspension
// and leave the value that came back out in v.
func (s *suspendedStack) nest(depth int) []string {
	if depth == 0 {
		if s.g.choose(2) == 0 {
			return []string{suspendedStackSuspend + ";", "v = 0;"}
		}
		// The list under construction is on the value stack at the suspension.
		return []string{"v = {0, " + suspendedStackSuspend + "};"}
	}
	s.layers++
	k := s.layers
	kind, variant := SuspendedStackKinds[s.g.choose(len(SuspendedStackKinds))], s.g.choose(SuspendedStackVariants)
	inner := s.nest(depth - 1)
	// loop runs inner on the second of three iterations of head, so the loop
	// has both finished and pending iterations at the suspension.
	loop := func(head []string, second, other, end string) []string {
		lines := []string{fmt.Sprintf("a%d = {};", k)}
		lines = append(lines, head...)
		lines = append(lines, fmt.Sprintf("if (%s)", second))
		lines = append(lines, inner...)
		return append(lines,
			fmt.Sprintf("a%d = {@a%d, v};", k, k),
			"else",
			fmt.Sprintf("a%d = {@a%d, %s};", k, k, other),
			"endif",
			end,
			fmt.Sprintf("v = a%d;", k))
	}
	switch kind {
	case "verb":
		return []string{"v = " + s.verbCall(k, inner) + ";"}
	case "eval":
		return []string{
			fmt.Sprintf("r%d = %s;", k, evalCall(k, inner, variant == 2)),
			fmt.Sprintf("if (!r%d[1]) raise(E_INVARG, tostr(%d), r%d[2]); endif", k, k, k),
			fmt.Sprintf("v = r%d;", k),
		}
	case "fork":
		forked := s.property(fmt.Sprintf("f%d", k), "0")
		lines := append([]string{"fork (0)"}, inner...)
		return append(lines,
			fmt.Sprintf("%s = {%d, v};", forked, k),
			"endfork",
			suspendedStackSuspend+";",
			fmt.Sprintf("v = %d;", k))
	case "except":
		lines := append([]string{"try"}, inner...)
		return append(lines,
			fmt.Sprintf("raise(E_RANGE, tostr(%d), v);", k),
			fmt.Sprintf("except e%d (E_RANGE)", k),
			fmt.Sprintf("v = {e%d[1], e%d[2], e%d[3]};", k, k, k),
			"endtry")
	case "finally":
		lines := append([]string{"try"}, inner...)
		return append(lines, "finally", fmt.Sprintf("v = {%d, v};", k), "endtry")
	case "for-list":
		return loop([]string{fmt.Sprintf("for x%d in ({%d, %d, %d})", k, k, k+1, k+2)},
			fmt.Sprintf("x%d == %d", k, k+1), fmt.Sprintf("x%d", k), "endfor")
	case "for-range":
		return loop([]string{fmt.Sprintf("for x%d in [%d..%d]", k, k, k+2)},
			fmt.Sprintf("x%d == %d", k, k+1), fmt.Sprintf("x%d", k), "endfor")
	case "for-map":
		return loop([]string{fmt.Sprintf("for x%d, y%d in ([%d -> %d, %d -> 0, %d -> %d])", k, k, k, -k, k+1, k+2, k)},
			fmt.Sprintf("y%d == %d", k, k+1), fmt.Sprintf("{y%d, x%d}", k, k), "endfor")
	case "while":
		return loop([]string{
			fmt.Sprintf("i%d = 0;", k),
			fmt.Sprintf("while (i%d < 3)", k),
			fmt.Sprintf("i%d = i%d + 1;", k, k),
		}, fmt.Sprintf("i%d == 2", k), fmt.Sprintf("i%d", k), "endwhile")
	case "scatter":
		// The optional target's default runs in the middle of the scatter.
		return []string{
			fmt.Sprintf("{b%d, ?c%d = %s, @d%d} = {%d};", k, k, s.call(variant, k, inner), k, k),
			fmt.Sprintf("v = {b%d, c%d, d%d};", k, k, k),
		}
	case "expression":
		// The call runs with a partly built list on the value stack.
		return []string{fmt.Sprintf("v = {%d, %s, %d + 1};", k, s.call(variant, k, inner), k)}
	case "in-finally":
		return s.inFinally(variant, k, inner)
	default:
		return s.builtinHook(kind, variant == 2, k, inner)
	}
}

// inFinally runs inner inside a finally handler body. variant picks how the
// handler was entered; each result shows the inner value and that what was
// pending at the suspension then happened.
func (s *suspendedStack) inFinally(variant, k int, inner []string) []string {
	switch variant {
	case 0: // the try body fell through, and the statement after endtry runs
		lines := []string{"try", fmt.Sprintf("w%d = %d;", k, k), "finally"}
		lines = append(lines, inner...)
		return append(lines, "endtry", fmt.Sprintf("v = {w%d, v};", k))
	case 1:
		// return in the try body of a verb. The returned value is fixed
		// before the handler runs, so the inner value leaves by a property.
		passed := s.property(fmt.Sprintf("p%d", k), "0")
		code := []string{"try", "return {args[1], 1};", "finally"}
		code = append(code, inner...)
		code = append(code, fmt.Sprintf("%s = v;", passed), "endtry", "return 0;")
		s.verb("o", fmt.Sprintf("s%d", k), code...)
		return []string{
			fmt.Sprintf("r%d = %s:s%d(%d);", k, suspendedStackObject, k, k),
			fmt.Sprintf("v = {r%d, %s};", k, passed),
		}
	case 2: // an error on its way to the except around the try/finally
		lines := []string{"try", "try", fmt.Sprintf("raise(E_PERM, tostr(%d), %d);", k, k), "finally"}
		lines = append(lines, inner...)
		return append(lines,
			fmt.Sprintf("w%d = v;", k),
			"endtry",
			"v = 0;",
			fmt.Sprintf("except e%d (E_PERM)", k),
			fmt.Sprintf("v = {e%d[1], e%d[2], e%d[3], w%d};", k, k, k, k),
			"endtry")
	default:
		// break out of the enclosing loop: the count after endtry is reached
		// on the first iteration only.
		lines := []string{
			fmt.Sprintf("n%d = 0;", k),
			fmt.Sprintf("for x%d in [1..3]", k),
			"try",
			fmt.Sprintf("if (x%d == 2) break; endif", k),
			"finally",
			fmt.Sprintf("if (x%d == 2)", k),
		}
		lines = append(lines, inner...)
		return append(lines,
			fmt.Sprintf("w%d = v;", k),
			"endif",
			"endtry",
			fmt.Sprintf("n%d = n%d + 1;", k, k),
			"endfor",
			fmt.Sprintf("v = {n%d, w%d};", k, k))
	}
}

// builtinHook runs inner inside a verb that a builtin calls and continues
// after: move(what, where)'s where:accept, the old location's exitfunc or
// where:enterfunc; create(parent)'s and recreate(old, parent)'s
// initialize; recycle(what)'s what:recycle or the exitfunc of what's
// location. indirect calls the builtin through call_function. The hook
// stores the inner value and whether its this or argument is the object the
// builtin acts on. The result is a check of the builtin's return value
// (move()'s and recycle()'s 0 itself), what the hook stored, and the
// builtin's effect: whether what ended in where, and for exitfunc whether
// where:enterfunc then ran; whether the created object has parent; whether
// what is still valid.
func (s *suspendedStack) builtinHook(kind string, indirect bool, k int, inner []string) []string {
	// The objects the builtin acts on: what and where for move(), parent and
	// the object to recreate for create() and recreate(), what for recycle().
	// Each is a setup variable of its name and a property of the test object.
	names := []string{fmt.Sprintf("t%d", k), fmt.Sprintf("c%d", k)}
	switch kind {
	case "create-initialize":
		names = []string{fmt.Sprintf("p%d", k)}
	case "recreate-initialize":
		names = []string{fmt.Sprintf("p%d", k), fmt.Sprintf("x%d", k)}
	case "recycle-recycle", "recycle-exitfunc":
		names = []string{fmt.Sprintf("d%d", k)}
	}
	refs := make([]string, len(names))
	for _, name := range names {
		s.setup = append(s.setup, fmt.Sprintf("%s = create(#-1);", name))
	}
	for i, name := range names {
		refs[i] = s.property(name, name)
	}
	hooked := s.property(fmt.Sprintf("h%d", k), "0")
	hook := func(check string) []string {
		return append(append([]string{}, inner...), fmt.Sprintf("%s = {%d, v, %s};", hooked, k, check))
	}
	call := func(name string, args ...string) string {
		if indirect {
			return fmt.Sprintf(`r%d = call_function("%s", %s);`, k, name, strings.Join(args, ", "))
		}
		return fmt.Sprintf("r%d = %s(%s);", k, name, strings.Join(args, ", "))
	}
	r := fmt.Sprintf("r%d", k)
	var lines, result []string
	switch kind {
	case "move-accept", "move-exitfunc", "move-enterfunc":
		what, where, whatRef, whereRef := names[0], names[1], refs[0], refs[1]
		lines = []string{call("move", whatRef, whereRef)}
		result = []string{r, hooked, fmt.Sprintf("%s.location == %s", whatRef, whereRef)}
		hook := hook("args[1] == " + whatRef)
		switch kind {
		case "move-accept":
			s.verb(where, "accept", append(hook, "return 1;")...)
		case "move-exitfunc":
			from, entered := fmt.Sprintf("a%d", k), s.property(fmt.Sprintf("n%d", k), "0")
			s.setup = append(s.setup, fmt.Sprintf("%s = create(#-1);", from))
			s.verb(from, "accept", "return 1;")
			s.setup = append(s.setup, fmt.Sprintf("move(%s, %s);", what, from))
			s.verb(from, "exitfunc", hook...)
			s.verb(where, "accept", "return 1;")
			s.verb(where, "enterfunc", entered+" = 1;")
			result = append(result, entered)
		default: // move-enterfunc
			s.verb(where, "accept", "return 1;")
			s.verb(where, "enterfunc", hook...)
		}
	case "create-initialize", "recreate-initialize":
		// The created object inherits initialize from parent.
		parent, parentRef := names[0], refs[0]
		s.verb(parent, "initialize", hook("parent(this) == "+parentRef)...)
		if kind == "create-initialize" {
			lines = []string{call("create", parentRef)}
			result = []string{fmt.Sprintf("typeof(%s) == OBJ && parent(%s) == %s", r, r, parentRef), hooked}
			break
		}
		// recreate() takes the number of an object recycled in the setup.
		old, oldRef := names[1], refs[1]
		s.setup = append(s.setup, fmt.Sprintf("recycle(%s);", old))
		lines = []string{call("recreate", oldRef, parentRef)}
		result = []string{fmt.Sprintf("%s == %s && parent(%s) == %s", r, oldRef, oldRef, parentRef), hooked}
	default: // recycle-recycle, recycle-exitfunc
		what, whatRef := names[0], refs[0]
		if kind == "recycle-recycle" {
			s.verb(what, "recycle", hook("this == "+whatRef)...)
		} else {
			location := fmt.Sprintf("l%d", k)
			s.setup = append(s.setup, fmt.Sprintf("%s = create(#-1);", location))
			s.verb(location, "accept", "return 1;")
			s.setup = append(s.setup, fmt.Sprintf("move(%s, %s);", what, location))
			s.verb(location, "exitfunc", hook("args[1] == "+whatRef)...)
		}
		lines = []string{call("recycle", whatRef)}
		result = []string{r, hooked, fmt.Sprintf("valid(%s)", whatRef)}
	}
	return append(lines, "v = {"+strings.Join(result, ", ")+"};")
}

// call returns an expression that runs inner in another frame, a verb's or
// eval()'s, and yields what that frame returns.
func (s *suspendedStack) call(variant, k int, inner []string) string {
	switch variant {
	case 1, 2:
		return evalCall(k, inner, variant == 2)
	}
	return s.verbCall(k, inner)
}

// verbCall installs inner as verb s<k> of the test object and returns a call
// of it. The verb returns its argument with v.
func (s *suspendedStack) verbCall(k int, inner []string) string {
	s.verb("o", fmt.Sprintf("s%d", k), append(append([]string{}, inner...), "return {args[1], v};")...)
	return fmt.Sprintf("%s:s%d(%d)", suspendedStackObject, k, k)
}

// evalCall returns a call of eval() on inner, made directly or, when indirect,
// through call_function. A completed eval() returns {1, {k, v}}.
func evalCall(k int, inner []string, indirect bool) string {
	source := mooString(strings.Join(inner, " ") + fmt.Sprintf(" return {%d, v};", k))
	if indirect {
		return `call_function("eval", ` + source + ")"
	}
	return "eval(" + source + ")"
}

// mooString returns text as a MOO string literal.
func mooString(text string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(text) + `"`
}
