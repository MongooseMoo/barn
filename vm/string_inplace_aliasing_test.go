package vm

import (
	"testing"

	dbstore "barn/db/store"
	"barn/types"
)

func TestStringInplaceAppendAliasing(t *testing.T) {
	cases := []struct {
		name string
		code string
		want string
	}{
		{
			"accumulator length",
			`s = ""; for i in [1..1000]; s = s + "x"; endfor; return length(s);`,
			"1000",
		},
		{
			"alias before append, original unchanged",
			`s = "a"; t = s; s = s + "b"; return {s, t};`,
			`{"ab", "a"}`,
		},
		{
			"built string alias then both append diverge",
			`s = ""; for i in [1..5]; s = s + "x"; endfor; t = s; s = s + "a"; t = t + "b"; return {s, t};`,
			`{"xxxxxa", "xxxxxb"}`,
		},
		{
			"captured in list stays frozen",
			`s = "a"; l = {s}; s = s + "b"; return {l[1], s};`,
			`{"a", "ab"}`,
		},
		{
			"captured as map value stays frozen",
			`s = "a"; m = ["k" -> s]; s = s + "b"; return {m["k"], s};`,
			`{"a", "ab"}`,
		},
		{
			"captured as map key stays frozen",
			`s = "a"; m = [s -> 7]; s = s + "b"; return {m["a"], s};`,
			`{7, "ab"}`,
		},
		{
			"normal concat expression",
			`return "a" + "b" + "c";`,
			`"abc"`,
		},
		{
			"non-string rhs still raises type error",
			`s = "a"; s = s + 1; return s;`,
			`E_TYPE`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := runBytecodeProgram(t, tc.code, nil, nil)
			if tc.want == "E_TYPE" {
				if res.Flow != types.FlowException || res.Error != types.E_TYPE {
					t.Fatalf("got flow=%v error=%v val=%v, want E_TYPE", res.Flow, res.Error, res.Val)
				}
				return
			}
			if got := res.Val.String(); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestStringInplaceAppendPropertyCapture(t *testing.T) {
	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetOwner(0)
	root.SetName("Root")
	root.SetFlags(dbstore.FlagRead | dbstore.FlagWrite | dbstore.FlagProgrammer)
	root.SetProperty("scratch", dbstore.NewProperty("scratch", types.NewStr(""), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
	if err := store.Add(root.Build()); err != nil {
		t.Fatalf("store.Add failed: %v", err)
	}

	res := runBytecodeProgram(t, `s = "a"; #0.scratch = s; s = s + "b"; return {#0.scratch, s};`, store, nil)
	if got, want := res.Val.String(), `{"a", "ab"}`; got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}
