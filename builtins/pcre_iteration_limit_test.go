package builtins

import (
	"fmt"
	"strings"
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// Toast's bf_pcre_match counts every pass of its match loop and, when a pass
// finds a match and the count has reached the limit, raises E_MAXREC before
// adding that match (pcre_moo.cc:208-233). The limit is
// $server_options.pcre_match_max_iterations read at call time, default 1000,
// clamped to 100..100000000 (pcre_moo.cc:198-202). Pinned Toast-green by
// moo-conformance-tests builtins/pcre.yaml, pcre_match_iteration_limit_*.

// newPcreIterationLimitExecution returns an execution whose $server_options
// carries pcre_match_max_iterations = value; types.None leaves it undefined.
func newPcreIterationLimitExecution(t *testing.T, value types.Value) *Execution {
	t.Helper()
	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetProperty("server_options", dbstore.NewProperty(types.NewObj(1), 0, dbstore.PropRead, false, true))
	if err := store.Add(root.Build()); err != nil {
		t.Fatal(err)
	}
	options := dbstore.NewObjectBuilder(1)
	if !value.IsNone() {
		options.SetProperty("pcre_match_max_iterations", dbstore.NewProperty(value, 0, dbstore.PropRead, false, true))
	}
	if err := store.Add(options.Build()); err != nil {
		t.Fatal(err)
	}
	ctx := newTestExecution()
	ctx.Store = store
	ctx.StoreTxn = store.DirectTxn()
	return ctx
}

func pcreMatchRepeatedA(ctx *Execution, matches int, extra ...types.Value) types.Result {
	args := append([]types.Value{types.NewStr(strings.Repeat("a", matches)), types.NewStr("a")}, extra...)
	return admitTestBuiltin("pcre_match", ctx, args)
}

func assertPcreMatchCount(t *testing.T, res types.Result, want int) {
	t.Helper()
	if !res.IsNormal() || res.Val.Type() != types.TYPE_LIST || res.Val.Len() != want {
		t.Fatalf("result = flow %v error %v, want a list of %d matches", res.Flow, res.Error, want)
	}
}

func assertPcreTooManyIterations(t *testing.T, res types.Result, loops int) {
	t.Helper()
	want := types.NewList([]types.Value{
		types.NewErr(types.E_MAXREC),
		types.NewStr(fmt.Sprintf("Too many iterations of matching loop: %d", loops)),
		types.NewInt(0),
	})
	if res.Flow != types.FlowException || res.Error != types.E_MAXREC {
		t.Fatalf("result = flow %v error %v, want raise %s", res.Flow, res.Error, want.String())
	}
	if !res.Val.Equal(want) {
		t.Fatalf("raise payload = %s, want %s", res.Val.String(), want.String())
	}
}

func TestPcreMatchIterationLimit(t *testing.T) {
	for _, test := range []struct {
		name   string
		option types.Value
		limit  int
	}{
		{"default", types.None, 1000},
		{"raised", types.NewInt(1500), 1500},
		{"lowered", types.NewInt(150), 150},
		{"clamped to 100", types.NewInt(5), 100},
		{"non-integer ignored", types.NewStr("150"), 1000},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := newPcreIterationLimitExecution(t, test.option)
			assertPcreMatchCount(t, pcreMatchRepeatedA(ctx, test.limit-1), test.limit-1)
			assertPcreTooManyIterations(t, pcreMatchRepeatedA(ctx, test.limit), test.limit)
		})
	}
}

// Toast assigns server_int_option's int to an unsigned int, so a negative
// option wraps and clamps to the maximum.
func TestPcreMatchIterationLimitNegativeOptionIsMaximum(t *testing.T) {
	ctx := newPcreIterationLimitExecution(t, types.NewInt(-1))
	assertPcreMatchCount(t, pcreMatchRepeatedA(ctx, 1200), 1200)
}

// The limit applies only to a pass that finds a match: a no-match on pass 1000
// returns the 999 matches, and find_all=0 stops after pass one.
func TestPcreMatchIterationLimitOnlyCountsMatchingPasses(t *testing.T) {
	ctx := newPcreIterationLimitExecution(t, types.None)
	res := admitTestBuiltin("pcre_match", ctx, []types.Value{
		types.NewStr(strings.Repeat("a", 999) + "bbb"), types.NewStr("a"),
	})
	assertPcreMatchCount(t, res, 999)
	assertPcreMatchCount(t, pcreMatchRepeatedA(ctx, 1000, types.NewInt(0), types.NewInt(0)), 1)
}

// Without a store there is no $server_options, so the default applies.
func TestPcreMatchIterationLimitWithoutStore(t *testing.T) {
	assertPcreTooManyIterations(t, pcreMatchRepeatedA(newTestExecution(), 1000), 1000)
}
