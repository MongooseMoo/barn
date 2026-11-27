package builtins

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// TestThreadPoolRaisesToastMessages pins thread_pool()'s E_INVARG messages and
// values to the WSL Toast oracle (issue #1205): the pool name is checked
// before the function name, and the thread count is read as a C int.
func TestThreadPoolRaisesToastMessages(t *testing.T) {
	str := types.NewStr
	cases := []struct {
		name    string
		args    []types.Value
		message string
		value   types.Value
	}{
		{"issue 1205 input", []types.Value{str("%\\"), str("")}, "Invalid thread pool", str("")},
		{"bad pool and function", []types.Value{str("FOO"), str("x")}, "Invalid thread pool", str("x")},
		{"pool names are case sensitive", []types.Value{str("init"), str("main")}, "Invalid thread pool", str("main")},
		{"DNS is not a pool", []types.Value{str("INIT"), str("DNS")}, "Invalid thread pool", str("DNS")},
		{"bad function", []types.Value{str("FOO"), str("MAIN")}, "Invalid function", str("FOO")},
		{"function before count", []types.Value{str("FOO"), str("MAIN"), types.NewInt(-3)}, "Invalid function", str("FOO")},
		{"negative count", []types.Value{str("INIT"), str("MAIN"), types.NewInt(-3)}, "Invalid number of threads", types.NewInt(-3)},
		{"count truncated to -1", []types.Value{str("INIT"), str("MAIN"), types.NewInt(4294967295)}, "Invalid number of threads", types.NewInt(-1)},
		{"count truncated to int32 min", []types.Value{str("INIT"), str("MAIN"), types.NewInt(2147483648)}, "Invalid number of threads", types.NewInt(-2147483648)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := newTestExecution()
			ctx.IsWizard = true
			result := admitTestBuiltin("thread_pool", ctx, tc.args)
			if !result.IsError() || result.Error != types.E_INVARG {
				t.Fatalf("thread_pool%v = %+v, want E_INVARG", tc.args, result)
			}
			if got := result.ErrorMessage(); got != tc.message {
				t.Fatalf("message = %q, want %q", got, tc.message)
			}
			if got := result.Val.Get(3); !got.Equal(tc.value) {
				t.Fatalf("value = %s, want %s", got.String(), tc.value.String())
			}
		})
	}

	ctx := newTestExecution()
	ctx.IsWizard = true
	result := admitTestBuiltin("thread_pool", ctx, []types.Value{str("INIT"), str("MAIN"), types.NewInt(4294967298)})
	if result.IsError() || !result.Val.Equal(types.NewInt(1)) {
		t.Fatalf("thread_pool(\"INIT\", \"MAIN\", 4294967298) = %+v, want 1", result)
	}
}
