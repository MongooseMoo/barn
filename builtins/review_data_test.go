package builtins

import (
	"math"
	"testing"

	"barn/kernel"
	"barn/types"
)

// TestReview_Data_* tests written by the analyst agent to probe suspected bugs
// in the data-type builtins (lists, maps, strings, math, json, url, ansi, pcre).

func reviewDataCtx() *kernel.TaskContext {
	ctx := kernel.NewTaskContext()
	ctx.IsWizard = true
	return ctx
}

// ── MATH ──────────────────────────────────────────────────────────────────────

// abs(math.MinInt64) returns MinInt64 UNCHANGED — this matches Toast.
// Toast's bf_abs (toaststunt/src/numbers.cc:513-526) does a plain integer
// negation with NO overflow check:
//
//	if (r.type == TYPE_INT) {
//	    if (r.v.num < 0)
//	        r.v.num = -r.v.num;
//	}
//
// On INT_MIN, C's two's-complement `-x` overflows back to INT_MIN (still
// negative), and Toast returns it as-is — no E_FLOAT, no E_INVARG. Verified
// with the WSL oracle: `abs(-9223372036854775807 - 1)` => -9223372036854775808.
// Barn's builtinAbs (math.go:27-30) mirrors this exactly, so it is correct.
// This test pins that behavior to prevent a well-meaning "overflow check"
// regression that would diverge from Toast.
func TestReview_Data_AbsMinInt64Overflow(t *testing.T) {
	ctx := reviewDataCtx()
	result := builtinAbs(ctx, []types.Value{types.NewInt(math.MinInt64)})
	if !result.IsNormal() {
		t.Fatalf("abs(MinInt64) returned error %v, want value MinInt64 (Toast returns it unchanged)", result.Error)
	}
	got := result.Val.(types.IntValue).Val
	if got != math.MinInt64 {
		t.Errorf("abs(MinInt64) = %d, want %d (two's-complement overflow; matches Toast bf_abs)", got, int64(math.MinInt64))
	}
}

// ── LISTS ─────────────────────────────────────────────────────────────────────

// HIGH: unique() uses elem.String() (includes quotes; case-sensitive) for the
// dedup key, so {"hello","HELLO"} is not deduplicated even though MOO string
// equality is case-insensitive (StrValue.Equal uses EqualFold).
func TestReview_Data_UniqueStrCaseInsensitive(t *testing.T) {
	ctx := reviewDataCtx()
	list := types.NewList([]types.Value{types.NewStr("hello"), types.NewStr("HELLO")})
	result := builtinUnique(ctx, []types.Value{list})
	if !result.IsNormal() {
		t.Fatalf("unique returned error: %v", result.Error)
	}
	got := result.Val.(types.ListValue)
	if got.Len() != 1 {
		t.Errorf("unique({\"hello\",\"HELLO\"}) = %d elements, want 1 (MOO strings are case-insensitive)", got.Len())
	}
}

// HIGH: is_member() uses strictEqual (case-SENSITIVE) for list search, but
// setadd/setremove use Equal (case-INSENSITIVE). In MOO "hello" == "HELLO",
// so is_member("HELLO", {"hello"}) should return 1.
func TestReview_Data_IsMemberStrCaseSensitiveBug(t *testing.T) {
	ctx := reviewDataCtx()
	list := types.NewList([]types.Value{types.NewStr("hello")})
	result := builtinIsMember(ctx, []types.Value{types.NewStr("HELLO"), list})
	if !result.IsNormal() {
		t.Fatalf("is_member returned error: %v", result.Error)
	}
	got := result.Val.(types.IntValue).Val
	if got != 1 {
		t.Errorf("is_member(\"HELLO\", {\"hello\"}) = %d, want 1 (MOO string equality is case-insensitive)", got)
	}
}

// HIGH: setadd uses Equal (case-insensitive) but unique uses String() (case-sensitive).
// They must agree: an element added by setadd must be treated as a duplicate by unique.
// setadd({"hello"}, "HELLO") → {"hello"} (sees them as equal).
// unique({"hello","HELLO"}) must also return {"hello"}.
func TestReview_Data_SetaddUniqueConsistency(t *testing.T) {
	ctx := reviewDataCtx()

	// setadd sees "HELLO" as already present (case-insensitive match).
	list := types.NewList([]types.Value{types.NewStr("hello")})
	saResult := builtinSetadd(ctx, []types.Value{list, types.NewStr("HELLO")})
	if !saResult.IsNormal() {
		t.Fatalf("setadd returned error: %v", saResult.Error)
	}
	saList := saResult.Val.(types.ListValue)
	if saList.Len() != 1 {
		t.Fatalf("setadd({\"hello\"}, \"HELLO\") has %d elements, want 1", saList.Len())
	}

	// unique must also collapse {"hello","HELLO"} to one element.
	dupeList := types.NewList([]types.Value{types.NewStr("hello"), types.NewStr("HELLO")})
	uqResult := builtinUnique(ctx, []types.Value{dupeList})
	if !uqResult.IsNormal() {
		t.Fatalf("unique returned error: %v", uqResult.Error)
	}
	uqList := uqResult.Val.(types.ListValue)
	if uqList.Len() != 1 {
		t.Errorf("setadd sees them as equal (len=1) but unique keeps both (len=%d) — inconsistent", uqList.Len())
	}
}

// MEDIUM: sort(list, keys, natural, reverse) silently ignores keys/natural/reverse.
// sort({1,2,3}, {}, 0, 1) with reverse=1 should return {3,2,1} but returns {1,2,3}.
func TestReview_Data_SortReverseIgnored(t *testing.T) {
	ctx := reviewDataCtx()
	list := types.NewList([]types.Value{types.NewInt(1), types.NewInt(2), types.NewInt(3)})
	result := builtinSort(ctx, []types.Value{
		list,
		types.NewList([]types.Value{}), // keys (empty = identity)
		types.NewInt(0),                // natural
		types.NewInt(1),                // reverse = true
	})
	if !result.IsNormal() {
		t.Fatalf("sort returned error: %v", result.Error)
	}
	got := result.Val.(types.ListValue)
	first := got.Get(1).(types.IntValue).Val
	if first != 3 {
		t.Errorf("sort({1,2,3}, {}, 0, 1) first element = %d, want 3 (reverse flag ignored)", first)
	}
}

// ── PCRE ──────────────────────────────────────────────────────────────────────

// HIGH: pcre_match returns {} immediately for an empty subject without attempting
// the match. Patterns like ".*" match the empty string. Toast returns a match.
func TestReview_Data_PcreMatchEmptySubject(t *testing.T) {
	ctx := reviewDataCtx()
	// Pattern ".*" matches the empty string — should produce one match.
	result := builtinPcreMatch(ctx, []types.Value{
		types.NewStr(""),   // subject
		types.NewStr(".*"), // pattern
	})
	if !result.IsNormal() {
		t.Fatalf("pcre_match returned error: %v", result.Error)
	}
	got := result.Val.(types.ListValue)
	if got.Len() == 0 {
		t.Errorf("pcre_match(\"\", \".*\") = {} (empty), want a match result for the empty string")
	}
}

// ── STRINGS ───────────────────────────────────────────────────────────────────

// F18: capitalize() must uppercase ONLY the first character, leaving the rest
// untouched — matching the MOO library verb $string_utils:capitalize
// ("string with first letter capitalized"; mongoose.db:73488). capitalize is
// not a ToastStunt C++ builtin (no match in toaststunt/src/; oracle rejects
// `capitalize(...)` as an unknown builtin), so the MOO library verb's
// "upcase the first letter only" behavior is authoritative. The old code used
// the deprecated strings.Title, which title-cased EVERY word and even
// upper-cased after apostrophes ("it's a test" -> "It'S A Test").
func TestReview_Data_CapitalizeDeprecatedTitle(t *testing.T) {
	ctx := reviewDataCtx()
	cases := []struct{ in, want string }{
		{"hello world", "Hello world"}, // only first char, not every word
		{"it's a test", "It's a test"}, // no spurious cap after apostrophe
		{"ABC", "ABC"},                 // already-capitalized first char unchanged
		{"", ""},                       // empty string stays empty
		{"123abc", "123abc"},           // non-letter first char left as-is
	}
	for _, c := range cases {
		result := builtinCapitalize(ctx, []types.Value{types.NewStr(c.in)})
		if !result.IsNormal() {
			t.Fatalf("capitalize(%q) returned error: %v", c.in, result.Error)
		}
		got := result.Val.(types.StrValue).Value()
		if got != c.want {
			t.Errorf("capitalize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// MEDIUM: mapvalues comment says "INT < FLOAT < OBJ < ERR < STR" but
// CompareMapKeys actually implements "INT < OBJ < FLOAT < ERR < STR".
// Validate the ACTUAL ordering (not the comment) to catch future regressions.
func TestReview_Data_MapkeysActualOrder(t *testing.T) {
	ctx := reviewDataCtx()
	// Build map with int, obj, float, err, str keys.
	m := types.NewMap([][2]types.Value{
		{types.NewStr("z"), types.NewInt(1)},
		{types.NewFloat(2.0), types.NewInt(2)},
		{types.NewInt(10), types.NewInt(3)},
		{types.NewObj(5), types.NewInt(4)},
		{types.NewErr(types.E_PERM), types.NewInt(5)},
	})
	result := builtinMapkeys(ctx, []types.Value{m})
	if !result.IsNormal() {
		t.Fatalf("mapkeys returned error: %v", result.Error)
	}
	keys := result.Val.(types.ListValue)
	if keys.Len() != 5 {
		t.Fatalf("mapkeys returned %d keys, want 5", keys.Len())
	}
	// Actual code order: INT(0) < OBJ(1) < FLOAT(2) < ERR(3) < STR(4)
	// Comment claims: INT < FLOAT < OBJ < ERR < STR — if comment is right, test fails
	_, isInt := keys.Get(1).(types.IntValue)
	_, isObj := keys.Get(2).(types.ObjValue)
	_, isFloat := keys.Get(3).(types.FloatValue)
	_, isErr := keys.Get(4).(types.ErrValue)
	_, isStr := keys.Get(5).(types.StrValue)
	if !(isInt && isObj && isFloat && isErr && isStr) {
		// Document the discrepancy: the mapkeys comment says INT<FLOAT<OBJ<ERR<STR
		// but the code (CompareMapKeys) does INT<OBJ<FLOAT<ERR<STR.
		// This test verifies what the CODE actually does; if Toast says otherwise,
		// the implementation order is the bug.
		t.Logf("mapkeys order: [1]=%T [2]=%T [3]=%T [4]=%T [5]=%T",
			keys.Get(1), keys.Get(2), keys.Get(3), keys.Get(4), keys.Get(5))
		t.Errorf("mapkeys ordering does not match INT<OBJ<FLOAT<ERR<STR (as implemented in CompareMapKeys)")
	}
}
