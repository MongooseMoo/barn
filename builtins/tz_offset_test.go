package builtins

import (
	"testing"
	"time"

	"github.com/MongooseMoo/barn/types"
)

func tzOffsetFunction(t *testing.T) BuiltinFunc {
	t.Helper()
	fn, ok := NewRegistry().Get("tz_offset")
	if !ok {
		t.Fatal("tz_offset is not registered")
	}
	return fn
}

func TestTZOffset(t *testing.T) {
	fn := tzOffsetFunction(t)
	for _, tc := range []struct{ date, zone, want string }{
		{"2026-01-15T12:00:00Z", "America/Edmonton", "-0700"},
		{"2026-07-15T12:00:00Z", "America/Edmonton", "-0600"},
		{"2026-01-15T12:00:00Z", "UTC", "+0000"},
		{"2026-01-15T12:00:00Z", "Asia/Kathmandu", "+0545"},
		{"2026-01-15T12:00:00Z", "America/St_Johns", "-0330"},
		{"2026-07-15T12:00:00Z", "America/St_Johns", "-0230"},
		{"1970-01-01T00:00:00Z", "UTC", "+0000"},
		{"1969-12-31T23:59:59Z", "UTC", "+0000"},
	} {
		t.Run(tc.zone+tc.date, func(t *testing.T) {
			stamp, err := time.Parse(time.RFC3339, tc.date)
			if err != nil {
				t.Fatal(err)
			}
			result := fn(newTestExecution(), []types.Value{types.NewStr(tc.zone), types.NewInt(stamp.Unix())})
			if result.Flow != types.FlowNormal || result.Val.Type() != types.TYPE_STR || result.Val.Str() != tc.want {
				t.Fatalf("tz_offset = %+v; want %q", result, tc.want)
			}
		})
	}
}

func TestTZOffsetCurrentTime(t *testing.T) {
	fn := tzOffsetFunction(t)
	ctx := newTestExecution()
	zone := types.NewStr("America/Edmonton")
	before := time.Now().Unix()
	result := fn(ctx, []types.Value{zone})
	after := time.Now().Unix()
	// Bracket the call so a daylight-saving transition cannot make this flaky.
	start := fn(ctx, []types.Value{zone, types.NewInt(before)})
	end := fn(ctx, []types.Value{zone, types.NewInt(after)})
	if result.Flow != types.FlowNormal || start.Flow != types.FlowNormal || end.Flow != types.FlowNormal {
		t.Fatalf("current-time lookup failed: result=%+v before=%+v after=%+v", result, start, end)
	}
	if !result.Val.Equal(start.Val) && !result.Val.Equal(end.Val) {
		t.Fatalf("tz_offset(zone) = %v; current timestamp offsets = %v, %v", result.Val, start.Val, end.Val)
	}
	if ctx.IrreversibleSideEffect {
		t.Fatal("tz_offset marked the task irreversible")
	}
}

func TestTZOffsetErrors(t *testing.T) {
	fn := tzOffsetFunction(t)
	for _, tc := range []struct {
		name string
		args []types.Value
		want types.ErrorCode
	}{
		{"unknown zone", []types.Value{types.NewStr("Barn/NoSuchZone")}, types.E_INVARG},
		{"integer zone", []types.Value{types.NewInt(0)}, types.E_TYPE},
		{"string timestamp", []types.Value{types.NewStr("UTC"), types.NewStr("0")}, types.E_TYPE},
		{"float timestamp", []types.Value{types.NewStr("UTC"), types.NewFloat(0)}, types.E_TYPE},
		{"missing zone", nil, types.E_ARGS},
		{"extra argument", []types.Value{types.NewStr("UTC"), types.NewInt(0), types.NewInt(0)}, types.E_ARGS},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := fn(newTestExecution(), tc.args)
			if result.Flow != types.FlowException || result.Error != tc.want {
				t.Fatalf("tz_offset = %+v; want %v", result, tc.want)
			}
		})
	}
}
