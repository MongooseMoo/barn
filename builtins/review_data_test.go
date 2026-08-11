package builtins

import (
	"math"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// TestReview_Data_* tests written by the analyst agent to probe suspected bugs
// in the data-type builtins (lists, maps, strings, math, json, url, ansi, pcre).

func reviewDataCtx() *Execution {
	ctx := newTestExecution()
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
	got := result.Val.Int()
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
	got := result.Val
	if got.Len() != 1 {
		t.Errorf("unique({\"hello\",\"HELLO\"}) = %d elements, want 1 (MOO strings are case-insensitive)", got.Len())
	}
}

// is_member() is case-SENSITIVE by default, matching ToastStunt: bf_is_member
// (collection.cc:84) sets case_matters = (argcount < 3) || is_true(arg3), so the
// 2-arg form is_member(value, list) compares with case_matters=true. Hence
// is_member("HELLO", {"hello"}) returns 0. (NOTE: setadd/setremove DO fold case —
// list.cc:151,163 call ismember(...,0) — but is_member itself does not.)
func TestReview_Data_IsMemberStrCaseSensitive(t *testing.T) {
	ctx := reviewDataCtx()
	list := types.NewList([]types.Value{types.NewStr("hello")})

	// Case mismatch -> not found (case-sensitive).
	miss := builtinIsMember(ctx, []types.Value{types.NewStr("HELLO"), list})
	if !miss.IsNormal() {
		t.Fatalf("is_member returned error: %v", miss.Error)
	}
	if got := miss.Val.Int(); got != 0 {
		t.Errorf("is_member(\"HELLO\", {\"hello\"}) = %d, want 0 (Toast is_member is case-sensitive, collection.cc:84)", got)
	}

	// Exact case -> found at position 1.
	hit := builtinIsMember(ctx, []types.Value{types.NewStr("hello"), list})
	if !hit.IsNormal() {
		t.Fatalf("is_member returned error: %v", hit.Error)
	}
	if got := hit.Val.Int(); got != 1 {
		t.Errorf("is_member(\"hello\", {\"hello\"}) = %d, want 1", got)
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
	saList := saResult.Val
	if saList.Len() != 1 {
		t.Fatalf("setadd({\"hello\"}, \"HELLO\") has %d elements, want 1", saList.Len())
	}

	// unique must also collapse {"hello","HELLO"} to one element.
	dupeList := types.NewList([]types.Value{types.NewStr("hello"), types.NewStr("HELLO")})
	uqResult := builtinUnique(ctx, []types.Value{dupeList})
	if !uqResult.IsNormal() {
		t.Fatalf("unique returned error: %v", uqResult.Error)
	}
	uqList := uqResult.Val
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
	got := result.Val
	first := got.Get(1).Int()
	if first != 3 {
		t.Errorf("sort({1,2,3}, {}, 0, 1) first element = %d, want 3 (reverse flag ignored)", first)
	}
}

// ── PCRE ──────────────────────────────────────────────────────────────────────

// F30 (CORRECTED): the original analyst finding claimed pcre_match("", ".*")
// should return a match because ".*" matches the empty string. The Toast SOURCE
// refutes this: bf_pcre_match's match loop is `while (offset < subject_length)`
// (toaststunt/src/pcre_moo.cc:208). For an empty subject subject_length == 0, so
// the loop NEVER iterates and the function returns its initial `new_list(0)` =>
// {} (pcre_moo.cc:188,320), regardless of pattern. The conformance suite pins
// this too: pcre_match_empty_subject (moo-conformance-tests
// .../builtins/pcre.yaml:201-205) expects value: []. Barn's empty-subject
// short-circuit (builtins/pcre.go) therefore MATCHES Toast; removing it would
// diverge (Go's regexp matches "" with ".*" -> [[0 0]]). This test pins Toast's
// TRUE result and guards against a "fix" that makes Barn return a spurious match.
func TestReview_Data_PcreMatchEmptySubject(t *testing.T) {
	ctx := reviewDataCtx()
	// Empty subject + an empty-string-matching pattern: Toast returns {}.
	result := builtinPcreMatch(ctx, []types.Value{
		types.NewStr(""),   // subject
		types.NewStr(".*"), // pattern (matches "" in PCRE, but loop never runs)
	})
	if !result.IsNormal() {
		t.Fatalf("pcre_match(\"\", \".*\") returned error: %v", result.Error)
	}
	got := result.Val
	if got.Len() != 0 {
		t.Errorf("pcre_match(\"\", \".*\") = %d entries, want 0 ({}) — Toast's loop "+
			"`while (offset < subject_length)` never iterates for an empty subject "+
			"(pcre_moo.cc:208)", got.Len())
	}

	// A genuine non-match on a NON-empty subject must also return {} (loop runs,
	// pcre2_match returns NOMATCH, break -> empty list). Confirms the empty-subject
	// short-circuit isn't masking the normal non-match path.
	nm := builtinPcreMatch(ctx, []types.Value{
		types.NewStr("foobar"),
		types.NewStr("baz"),
	})
	if !nm.IsNormal() {
		t.Fatalf("pcre_match(\"foobar\", \"baz\") returned error: %v", nm.Error)
	}
	if n := nm.Val.Len(); n != 0 {
		t.Errorf("pcre_match(\"foobar\", \"baz\") = %d entries, want 0 ({})", n)
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
		got := result.Val.Str()
		if got != c.want {
			t.Errorf("capitalize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// mapkeys returns rbtree-traversal order, which sorts cross-type keys by
// Toast's RUNTIME var_type values (structures.h: pointer-carrying types have
// 0x80 OR'd in, floats/bools do not): INT(0) < OBJ(1) < ERR(3) < FLOAT(9) <
// STR(0x82). Pinned by conformance map_dump_persistence (expected keys
// {2, #1, E_TYPE, 1.5, "tail"}).
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
	keys := result.Val
	if keys.Len() != 5 {
		t.Fatalf("mapkeys returned %d keys, want 5", keys.Len())
	}
	isInt := keys.Get(1).Type() == types.TYPE_INT
	isObj := keys.Get(2).Type() == types.TYPE_OBJ
	isErr := keys.Get(3).Type() == types.TYPE_ERR
	isFloat := keys.Get(4).Type() == types.TYPE_FLOAT
	isStr := keys.Get(5).Type() == types.TYPE_STR
	if !(isInt && isObj && isErr && isFloat && isStr) {
		t.Logf("mapkeys order: [1]=%v [2]=%v [3]=%v [4]=%v [5]=%v",
			keys.Get(1), keys.Get(2), keys.Get(3), keys.Get(4), keys.Get(5))
		t.Errorf("mapkeys ordering does not match Toast INT<OBJ<ERR<FLOAT<STR")
	}
}
