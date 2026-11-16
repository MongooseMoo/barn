package builtins

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// TestFlagArgumentsAdmitIntOrBool: ToastStunt 61f2b7a registers these
// builtins' flag positions TYPE_FLAG (-3). call_bi_func admits INT or BOOL
// there (functions.cc), and function_info reports -3.
func TestFlagArgumentsAdmitIntOrBool(t *testing.T) {
	ctx := newTestExecution()
	for name, want := range map[string]string{
		"explode":         `{"explode", 1, 3, {2, 2, -3}}`,
		"file_grep":       `{"file_grep", 2, 3, {0, 2, -3}}`,
		"is_member":       `{"is_member", 2, 3, {-1, -1, -3}}`,
		"isa":             `{"isa", 2, 3, {-1, -1, -3}}`,
		"locate_by_name":  `{"locate_by_name", 1, 2, {2, -3}}`,
		"locations":       `{"locations", 1, 3, {1, 1, -3}}`,
		"maphaskey":       `{"maphaskey", 2, 3, {10, -1, -3}}`,
		"occupants":       `{"occupants", 1, 4, {4, -1, -3, -3}}`,
		"pcre_match":      `{"pcre_match", 2, 4, {2, 2, -3, -3}}`,
		"queued_tasks":    `{"queued_tasks", 0, 2, {-3, -3}}`,
		"set_thread_mode": `{"set_thread_mode", 0, 1, {-3}}`,
		"sort":            `{"sort", 1, 4, {4, 4, -3, -3}}`,
		"switch_player":   `{"switch_player", 2, 3, {1, 1, -3}}`,
	} {
		info := admitTestBuiltin("function_info", ctx, []types.Value{types.NewStr(name)})
		if !info.IsNormal() || info.Val.String() != want {
			t.Errorf("function_info(%q) = %v (%v), want %s", name, info.Val, info.Error, want)
		}
	}

	list := types.NewList([]types.Value{types.NewInt(3), types.NewInt(1), types.NewInt(2)})
	empty := types.NewList(nil)
	got := admitTestBuiltin("sort", ctx, []types.Value{list, empty, types.NewBool(false), types.NewBool(true)})
	if !got.IsNormal() || got.Val.String() != "{3, 2, 1}" {
		t.Fatalf("sort({3, 1, 2}, {}, false, true) = %v (%v), want {3, 2, 1}", got.Val, got.Error)
	}
	got = admitTestBuiltin("sort", ctx, []types.Value{list, empty, types.NewInt(0), types.NewFloat(1.5)})
	if got.Error != types.E_TYPE {
		t.Fatalf("sort({3, 1, 2}, {}, 0, 1.5) error = %v, want E_TYPE", got.Error)
	}
}
