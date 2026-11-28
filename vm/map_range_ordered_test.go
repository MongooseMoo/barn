package vm

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func TestOrderedMapRangeOps(t *testing.T) {
	for _, test := range []struct{ program, want string }{
		{`m = []; for i in [1..80]; m[i] = i; endfor return {m[^], m[$], mapkeys(m[35..37])};`, `{1, 80, {35, 36, 37}}`},
		{`m = [1 -> "one", 2 -> "two", 3 -> "three"]; m[3..1] = [2 -> "new"]; return m;`, `[1 -> "one", 2 -> "two", 3 -> "three"]`},
		{`return ["a" -> 1, "b" -> 2, "c" -> 3]["B".."C"];`, `["b" -> 2, "c" -> 3]`},
		{`m = []; return m[^..$];`, `[]`},
	} {
		t.Run(test.program, func(t *testing.T) {
			got := runBytecodeProgram(t, test.program, nil, nil)
			if got.Flow != types.FlowReturn || got.Val.String() != test.want {
				t.Fatalf("result = %+v, want %s", got, test.want)
			}
		})
	}
}
