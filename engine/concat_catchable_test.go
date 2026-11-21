package engine

import (
	"fmt"
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/metrics"
	"github.com/MongooseMoo/barn/types"
)

// sizeLimitGrowths are the server/limits_concat_catchable.yaml growths: each
// passes the named limit (set to 1100) in the interpreter or a builtin.
var sizeLimitGrowths = []struct {
	name   string
	option string
	grow   string
}{
	{"list splice", "max_list_value_bytes", `r = {}; for i in [1..1000] r = {@r, i}; endfor`},
	{"list add tail", "max_list_value_bytes", `r = {}; for i in [1..1000] r = {1, @r}; endfor`},
	{"list plus", "max_list_value_bytes", `r = {}; for i in [1..1000] r = r + {i}; endfor`},
	{"list indexset", "max_list_value_bytes", `r = {0}; for i in [1..1000] r[1] = {@r}; endfor`},
	{"string concat", "max_string_concat", `r = ""; for i in [1..1000] r = r + "0123456789"; endfor`},
	{"string rangeset", "max_string_concat", `r = ""; for i in [1..1000] r[1..0] = "0123456789"; endfor`},
	{"map indexset", "max_map_value_bytes", `r = []; for i in [1..1000] r[i] = i; endfor`},
	{"listappend", "max_list_value_bytes", `r = {}; for i in [1..1000] r = listappend(r, i); endfor`},
	{"setadd", "max_list_value_bytes", `r = {}; for i in [1..1000] r = setadd(r, i); endfor`},
	{"tostr", "max_string_concat", `s = "0123456789"; r = s; for i in [1..200] r = tostr(r, s); endfor`},
	{"strsub", "max_string_concat", `r = "a"; for i in [1..20] r = strsub(r, "a", "aa"); endfor`},
	{"chr", "max_string_concat", `l = {}; for i in [1..1500] l = {@l, 65}; endfor r = chr(l);`},
	{"value_hash", "max_string_concat", `l = {}; for i in [1..400] l = {@l, i}; endfor r = value_hash(l);`},
	{"value_hmac", "max_string_concat", `l = {}; for i in [1..400] l = {@l, i}; endfor r = value_hmac(l, "key");`},
	{"encode_base64", "max_string_concat", `s = ""; for i in [1..90] s = s + "0123456789"; endfor r = encode_base64(s);`},
}

func concatCatchableCode(catchable int, option, grow string) string {
	return fmt.Sprintf(`add_property(#1, "max_concat_catchable", %d, {player, "r"});`+
		`add_property(#1, %q, 1100, {player, "r"});`+
		`add_property(#1, "caught", 0, {player, "rw"});`+
		`add_property(#1, "finally_ran", 0, {player, "rw"});`+
		`load_server_options();`+
		`try try %s return "completed"; except e (ANY) #1.caught = 1; return e[1]; endtry finally #1.finally_ran = 1; endtry`,
		catchable, option, grow)
}

func propertyInt(t *testing.T, store *dbstore.Store, name string) int64 {
	t.Helper()
	prop, errCode := store.DirectTxn().FindProperty(1, name)
	if errCode != types.E_NONE {
		t.Fatalf("read #1.%s: %s", name, errCode)
	}
	return prop.Value.Int()
}

// Toast execute.cc PUSH_ERROR_UNLESS_QUOTA and list.cc make_space_pack: with
// max_concat_catchable false a value past a size limit aborts the task as out
// of seconds, running no except or finally.
func TestSizeLimitAbortsTaskWhenNotCatchable(t *testing.T) {
	for _, tc := range sizeLimitGrowths {
		t.Run(tc.name, func(t *testing.T) {
			store := serverOptionsVerbStore(t, concatCatchableCode(0, tc.option, tc.grow))
			s := NewRuntime(store)
			defer s.Stop()

			result := s.CallVerb(1, "go", nil, 2)
			if result.Flow != types.FlowException || result.Error != types.E_MAXREC || result.Reason != types.ExceptionReasonSecondsBudget {
				t.Fatalf("result = flow %v error %s reason %v val %s, want the seconds abort", result.Flow, result.Error, result.Reason, result.Val.String())
			}
			if caught, ran := propertyInt(t, store, "caught"), propertyInt(t, store, "finally_ran"); caught != 0 || ran != 0 {
				t.Fatalf("caught = %d, finally_ran = %d; want neither to run", caught, ran)
			}
		})
	}
}

// With max_concat_catchable true the same growth raises a catchable E_QUOTA.
func TestSizeLimitRaisesQuotaWhenCatchable(t *testing.T) {
	for _, tc := range sizeLimitGrowths {
		t.Run(tc.name, func(t *testing.T) {
			store := serverOptionsVerbStore(t, concatCatchableCode(1, tc.option, tc.grow))
			s := NewRuntime(store)
			defer s.Stop()

			result := s.CallVerb(1, "go", nil, 2)
			if result.Flow == types.FlowException {
				t.Fatalf("go raised %s, want E_QUOTA caught", result.Error)
			}
			if got := result.Val.String(); got != "E_QUOTA" {
				t.Fatalf("got %s, want E_QUOTA", got)
			}
			if caught, ran := propertyInt(t, store, "caught"), propertyInt(t, store, "finally_ran"); caught != 1 || ran != 1 {
				t.Fatalf("caught = %d, finally_ran = %d; want both", caught, ran)
			}
		})
	}
}

// Toast aborts regardless of the verb's d flag; without it the limit must not
// become a pushed E_QUOTA value.
func TestSizeLimitAbortsVerbWithoutDebugFlag(t *testing.T) {
	store := protectedRedirectStore(t)
	verb := dbstore.NewVerb(
		[]string{"go"},
		2,
		dbstore.VerbRead|dbstore.VerbExecute,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "this"},
		[]string{`add_property(#1, "max_list_value_bytes", 1100, {player, "r"});` +
			`load_server_options();` +
			`r = {}; for i in [1..1000] r = {@r, i}; endfor return {"completed", r};`},
	)
	if _, errCode := store.AddVerb(1, verb); errCode != types.E_NONE {
		t.Fatalf("add verb: %s", errCode)
	}
	s := NewRuntime(store)
	defer s.Stop()

	result := s.CallVerb(1, "go", nil, 2)
	if result.Flow != types.FlowException || result.Error != types.E_MAXREC || result.Reason != types.ExceptionReasonSecondsBudget {
		t.Fatalf("result = flow %v error %s val %s, want the seconds abort", result.Flow, result.Error, result.Val.String())
	}
}

// Toast notifies the player of the timeout traceback ("Task ran out of
// seconds", execute.cc abort_task), not the E_MAXREC message, when no
// #0:handle_task_timeout handles the abort.
func TestSizeLimitAbortReportsRanOutOfSeconds(t *testing.T) {
	store := dbstore.NewStore()
	addServerVerbTestObject(t, store, 0, dbstore.FlagWizard)
	addServerVerbTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)
	if errCode := store.DirectTxn().DefineProperty(0, "server_options", dbstore.NewProperty(
		types.NewObj(0), 2, dbstore.PropRead, false, true,
	)); errCode != types.E_NONE {
		t.Fatalf("define server_options: %v", errCode)
	}
	store.AddVerb(0, dbstore.NewVerb([]string{"grow"}, 2,
		dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "this"},
		[]string{
			`add_property(#0, "max_list_value_bytes", 1100, {player, "r"});`,
			"load_server_options();",
			"fork (0)",
			"  r = {}; for i in [1..1000] r = {@r, i}; endfor",
			"endfork",
			"return 1;",
		}))

	s := NewRuntime(store)
	t.Cleanup(s.Stop)
	var messages []string
	s.SetTracebackSender(func(_ types.ObjID, result types.Result, _ *metrics.Command) {
		messages = append(messages, result.ErrorMessage())
	})
	if _, err := s.RunServerVerbTask(0, "grow", nil, 2); err != nil {
		t.Fatalf("run grow: %v", err)
	}
	s.ProcessReadyTasks()
	if len(messages) != 1 || messages[0] != "Task ran out of seconds" {
		t.Fatalf("traceback messages = %q, want [\"Task ran out of seconds\"]", messages)
	}
}

// Toast builds call arguments as a list with OP_LIST_ADD_TAIL, so two or more
// arguments whose list passes max_list_value_bytes raise E_QUOTA (with
// max_concat_catchable true); a lone argument is never checked.
func TestArgumentListUsesListValueByteLimit(t *testing.T) {
	const code = `add_property(#1, "max_concat_catchable", 1, {player, "r"});` +
		`x = []; for i in [1..200] x[i] = i; endfor ` +
		`add_property(#1, "max_list_value_bytes", 1100, {player, "r"});` +
		`load_server_options();` +
		"return {length(x), `maphaskey(x, 1) ! ANY', `#1:take(x) ! ANY', `#1:take(x, 1) ! ANY', `maphaskey(@{x, 1}) ! ANY'};"
	store := serverOptionsVerbStore(t, code)
	take := dbstore.NewVerb([]string{"take"}, 2,
		dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "this"},
		[]string{"return length(args);"})
	if _, errCode := store.AddVerb(1, take); errCode != types.E_NONE {
		t.Fatalf("add take: %s", errCode)
	}
	s := NewRuntime(store)
	defer s.Stop()

	result := s.CallVerb(1, "go", nil, 2)
	if result.Flow == types.FlowException {
		t.Fatalf("go raised %s", result.Error)
	}
	if got, want := result.Val.String(), `{200, E_QUOTA, 1, E_QUOTA, E_QUOTA}`; got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}
