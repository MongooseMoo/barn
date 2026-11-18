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
}

const (
	maxSuspendedStacks     = 3
	maxSuspendedStackDepth = 6

	// suspendedStackObject is the #0 property holding the test object, so
	// eval() source and fork bodies, which share no variables with the code
	// around them, reach its verbs and properties.
	suspendedStackObject = "$suspended_stack"

	// suspendedStackSuspend is the call every suspension makes.
	suspendedStackSuspend = "suspend()"
)

// SuspendedStack generates a program whose tasks reach suspend() at the bottom
// of a chosen stack of nested contexts, then pass a value v back out through
// every layer. Each layer wraps v with its own number and the state it kept
// across the suspension (loop position, verb arguments, the handler's error,
// eval()'s {1, value}), so a layer resumed with a wrong continuation changes
// the returned list or a property of the test object.
//
// A fork layer's child carries the inner layers: it stores their value in its
// own property f<layer>, and the parent suspends beside the queued child.
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
	for _, layer := range s.forks {
		lines = append(lines, fmt.Sprintf(`add_property(o, "f%d", 0, {o.owner, "rw"});`, layer))
	}
	for _, verb := range s.verbs {
		code := make([]string, len(verb.code))
		for i, line := range verb.code {
			code[i] = mooString(line)
		}
		lines = append(lines,
			fmt.Sprintf(`add_verb(o, {o.owner, "rxd", "s%d"}, {"this", "none", "this"});`, verb.layer),
			fmt.Sprintf(`c = set_verb_code(o, "s%d", {%s});`, verb.layer, strings.Join(code, ", ")),
			fmt.Sprintf(`if (c) raise(E_INVARG, "s%d does not compile", c); endif`, verb.layer))
	}
	lines = append(lines, "results = {};")
	lines = append(lines, body...)
	lines = append(lines, "return results;")
	return strings.Join(lines, "\n")
}

type suspendedStack struct {
	g      generator
	layers int                  // layers rendered so far; each layer's number is unique in the program
	verbs  []suspendedStackVerb // verbs the setup installs on the test object
	forks  []int                // fork layers, each with a property f<layer>
}

type suspendedStackVerb struct {
	layer int
	code  []string
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
	kind, variant := SuspendedStackKinds[s.g.choose(len(SuspendedStackKinds))], s.g.choose(2)
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
			fmt.Sprintf("r%d = %s;", k, evalCall(k, inner)),
			fmt.Sprintf("if (!r%d[1]) raise(E_INVARG, tostr(%d), r%d[2]); endif", k, k, k),
			fmt.Sprintf("v = r%d;", k),
		}
	case "fork":
		s.forks = append(s.forks, k)
		lines := append([]string{"fork (0)"}, inner...)
		return append(lines,
			fmt.Sprintf("%s.f%d = {%d, v};", suspendedStackObject, k, k),
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
	default: // expression: the call runs with a partly built list on the value stack
		return []string{fmt.Sprintf("v = {%d, %s, %d + 1};", k, s.call(variant, k, inner), k)}
	}
}

// call returns an expression that runs inner in another frame, a verb's or
// eval()'s, and yields what that frame returns.
func (s *suspendedStack) call(variant, k int, inner []string) string {
	if variant == 0 {
		return s.verbCall(k, inner)
	}
	return evalCall(k, inner)
}

// verbCall installs inner as verb s<k> of the test object and returns a call
// of it. The verb returns its argument with v.
func (s *suspendedStack) verbCall(k int, inner []string) string {
	code := append(append([]string{}, inner...), "return {args[1], v};")
	s.verbs = append(s.verbs, suspendedStackVerb{layer: k, code: code})
	return fmt.Sprintf("%s:s%d(%d)", suspendedStackObject, k, k)
}

// evalCall returns a call of eval() on inner. A completed eval() returns
// {1, {k, v}}.
func evalCall(k int, inner []string) string {
	source := strings.Join(inner, " ") + fmt.Sprintf(" return {%d, v};", k)
	return "eval(" + mooString(source) + ")"
}

// mooString returns text as a MOO string literal.
func mooString(text string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(text) + `"`
}
