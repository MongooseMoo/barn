package vm

// Aliasing stress tests for the in-place list-append optimization. The in-place
// path mutates a backing array's spare slot, so these assert that no aliasing
// holder of a shared list ever observes a mutation through another binding.

import "testing"

func TestListInplaceAppendAliasing(t *testing.T) {
	cases := []struct {
		name string
		code string
		want string // expected result via Value.String()
	}{
		{
			"alias before append, original unchanged",
			`l = {1, 2, 3}; m = l; l = {@l, 99}; return {m, l};`,
			"{{1, 2, 3}, {1, 2, 3, 99}}",
		},
		{
			"build loop then alias then both append diverge",
			`l = {}; for i in [1..5]; l = {@l, i}; endfor; m = l; l = {@l, 6}; m = {@m, 7}; return {l, m};`,
			"{{1, 2, 3, 4, 5, 6}, {1, 2, 3, 4, 5, 7}}",
		},
		{
			"appended list captured as nested element stays frozen",
			`l = {1, 2}; outer = {l}; l = {@l, 3}; return {outer, l};`,
			"{{{1, 2}}, {1, 2, 3}}",
		},
		{
			"three-way share, each diverges",
			`l = {}; for i in [1..3]; l = {@l, i}; endfor; a = l; b = l; a = {@a, 10}; b = {@b, 20}; l = {@l, 30}; return {a, b, l};`,
			"{{1, 2, 3, 10}, {1, 2, 3, 20}, {1, 2, 3, 30}}",
		},
		{
			"multiple trailing elements via self-append idiom",
			`l = {1}; l = {@l, 2, 3, 4}; return l;`,
			"{1, 2, 3, 4}",
		},
		{
			"trailing splice via self-append idiom",
			`l = {1, 2}; m = {3, 4}; l = {@l, @m}; return {l, m};`,
			"{{1, 2, 3, 4}, {3, 4}}",
		},
		{
			"append does not disturb a prior slice of the list",
			`l = {}; for i in [1..5]; l = {@l, i}; endfor; s = l[2..4]; l = {@l, 6}; return {s, l};`,
			"{{2, 3, 4}, {1, 2, 3, 4, 5, 6}}",
		},
		{
			"reassign-through-alias chain",
			`l = {}; for i in [1..4]; l = {@l, i}; endfor; m = l; n = m; m = {@m, 99}; return {l, m, n};`,
			"{{1, 2, 3, 4}, {1, 2, 3, 4, 99}, {1, 2, 3, 4}}",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := runBytecodeProgram(t, tc.code, nil, nil)
			if got := res.Val.String(); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}
