package builtins

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func TestGunrayANSITags(t *testing.T) {
	ctx := newTestExecution()
	raw := "\x1b[31m"
	removed := admitTestBuiltin("remove_ansi", ctx, []types.Value{types.NewStr(raw + "x[ReD][random][unknown]")})
	if removed.Flow != types.FlowNormal || removed.Val.Str() != raw+"x[unknown]" {
		t.Errorf("remove_ansi yielded %q (flow %v)", removed.Val.Str(), removed.Flow)
	}

	got := admitTestBuiltin("parse_ansi", ctx, []types.Value{types.NewStr("[random][RANDOM]")})
	s := got.Val.Str()
	if got.Flow != types.FlowNormal || len(s) != 10 {
		t.Fatalf("parse_ansi yielded %q (flow %v)", s, got.Flow)
	}
	for _, code := range []string{s[:5], s[5:]} {
		switch code {
		case "\x1b[31m", "\x1b[32m", "\x1b[33m", "\x1b[34m", "\x1b[35m":
		default:
			t.Errorf("random tag yielded %q, outside the Toast palette", code)
		}
	}
}

func TestGunrayNumericConversions(t *testing.T) {
	tests := []struct {
		builtin string
		input   string
		want    types.Value
	}{
		{"tofloat", "0x1p2", types.NewFloat(4)},
		{"tofloat", "-0X1.8P+2", types.NewFloat(-6)},
		{"tofloat", "0x1.8", types.NewFloat(1.5)},
		{"tofloat", "+0x7", types.NewFloat(7)},
		{"tofloat", "0x1_2p0", types.None},
		{"tofloat", "0x1p", types.None},
		{"toobj", "# 7", types.NewObj(7)},
		{"toobj", "  #\t-3 ", types.NewObj(-3)},
		{"toobj", "# 7x", types.NewObj(0)},
	}

	ctx := newTestExecution()
	for _, tc := range tests {
		t.Run(tc.builtin+" "+tc.input, func(t *testing.T) {
			got := admitTestBuiltin(tc.builtin, ctx, []types.Value{types.NewStr(tc.input)})
			if tc.want == types.None {
				if got.Error != types.E_INVARG {
					t.Errorf("got %v, want E_INVARG", got)
				}
			} else if got.Flow != types.FlowNormal || !got.Val.Equal(tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestGunrayEmbeddedJSON(t *testing.T) {
	ctx := newTestExecution()
	mode := types.NewStr("embedded-types")
	for _, tc := range []struct {
		text string
		want types.Value
	}{
		{`"true|bool"`, types.NewBool(true)},
		{`"maybe|bool"`, types.NewBool(false)},
		{`"True|bool"`, types.NewBool(false)},
		{`"abc|int"`, types.NewInt(0)},
		{`"  -42.5|int"`, types.NewInt(-42)},
		{`"99999999999999999999|int"`, types.NewInt(9223372036854775807)},
	} {
		t.Run(tc.text, func(t *testing.T) {
			got := admitTestBuiltin("parse_json", ctx, []types.Value{types.NewStr(tc.text), mode})
			if got.Flow != types.FlowNormal || got.Val.Type() != tc.want.Type() || !got.Val.Equal(tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}

	for _, val := range []types.Value{
		types.NewStr("42|int"),
		types.NewStr("true|bool"),
		types.NewStr("|str"),
		types.NewStr("42|INT"),
		types.NewMap([][2]types.Value{{types.NewStr("2|int"), types.NewStr("42|int")}, {types.NewBool(true), types.NewInt(1)}}),
	} {
		t.Run(val.String(), func(t *testing.T) {
			json := admitTestBuiltin("generate_json", ctx, []types.Value{val, mode})
			if json.Flow != types.FlowNormal {
				t.Fatalf("generate_json yielded %v", json)
			}
			got := admitTestBuiltin("parse_json", ctx, []types.Value{json.Val, mode})
			if got.Flow != types.FlowNormal || got.Val.Type() != val.Type() || !got.Val.Equal(val) {
				t.Errorf("round trip through %s yielded %v, want %v", json.Val, got, val)
			}
		})
	}
}

func TestGunrayMOOPatterns(t *testing.T) {
	tests := []struct {
		name, builtin, subject, pattern string
		flags                           int64
		start, end, subStart, subEnd    int64
		bad                             bool
	}{
		{name: "wrong direction begin", subject: "a!", pattern: "a%<"},
		{name: "wrong direction end", subject: "!a", pattern: "%>a"},
		{name: "right direction begin", subject: "!a", pattern: "%<a", start: 2, end: 2},
		{name: "right direction end", subject: "a!", pattern: "a%>", start: 1, end: 1},
		{name: "whole word reverse", builtin: "rmatch", subject: "ba a", pattern: "%<a%>", start: 4, end: 4},
		{name: "reverse inside word", builtin: "rmatch", subject: "ba", pattern: "%<a"},
		{name: "underscore separates words", subject: "a_b", pattern: "a%>_%<b", start: 1, end: 3},
		{name: "whole word anchored", subject: "aa", pattern: "^a%>"},
		{name: "backreference", subject: "aa", pattern: "%(a%)%1", flags: 1, start: 1, end: 2, subStart: 1, subEnd: 1},
		{name: "backreference with trailing digit", subject: "aa2", pattern: "%(a%)%12", start: 1, end: 3},
		{name: "case insensitive backreference", subject: "éaA", pattern: "%(a%)%1", start: 2, end: 3, subStart: 2, subEnd: 2},
		{name: "ASCII boundary with backreference", subject: "éaa", pattern: "%b%(a%)%1", start: 2, end: 3, subStart: 2, subEnd: 2},
		{name: "ASCII non-boundary with backreference", subject: "éaa", pattern: "%B%(a%)%1"},
		{name: "class with backreference", subject: "éaa", pattern: "%(%w%)%1", flags: 1, start: 2, end: 3, subStart: 2, subEnd: 2},
		{name: "negated class with backreference", subject: "éaa", pattern: "%W%(a%)%1", flags: 1, start: 1, end: 3, subStart: 2, subEnd: 2},
		{name: "unequal backreference", subject: "ab", pattern: "%(a%)%1", flags: 1},
		{name: "undefined backreference", subject: "%9", pattern: "%9"},
		{name: "unmatched backreference", subject: "b", pattern: "%(a%)?%1b"},
		{name: "undefined backreference in another branch", subject: "b", pattern: "%9%|b", start: 1, end: 1},
		{name: "rightmost overlapping backreference", builtin: "rmatch", subject: "aaAA", pattern: "%(a%)%1", start: 3, end: 4, subStart: 3, subEnd: 3},
		{name: "second backreference in rightmost match", builtin: "rmatch", subject: "abb ABB", pattern: "%(a%)%(b%)%2", start: 5, end: 7, subStart: 5, subEnd: 5},
		{name: "line anchor in rightmost extended match", builtin: "rmatch", subject: "xaa\nAA", pattern: "^%(a%)%1", start: 5, end: 6, subStart: 5, subEnd: 5},
		{name: "interior caret", subject: "abc", pattern: "a^b", bad: true},
		{name: "escaped caret", subject: "a^b", pattern: "a%^b", start: 1, end: 3},
		{name: "caret in class", subject: "a^b", pattern: "a[x^]b", start: 1, end: 3},
		{name: "caret in alternative", subject: "a\nb", pattern: "x%|^b", start: 3, end: 3},
		{name: "caret in group", subject: "a\nb", pattern: "%(^b%)", start: 3, end: 3, subStart: 3, subEnd: 3},
		{name: "beginning of line", subject: "a\nb", pattern: "^b", start: 3, end: 3},
		{name: "end of line", subject: "a\nb", pattern: "a$", start: 1, end: 1},
	}

	ctx := newTestExecution()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			builtin := tc.builtin
			if builtin == "" {
				builtin = "match"
			}
			got := admitTestBuiltin(builtin, ctx, []types.Value{types.NewStr(tc.subject), types.NewStr(tc.pattern), types.NewInt(tc.flags)})
			if tc.bad {
				if got.Error != types.E_INVARG {
					t.Fatalf("got %v, want E_INVARG", got)
				}
				return
			}
			if got.Flow != types.FlowNormal {
				t.Fatalf("got %v, want a normal result", got)
			}
			if tc.start == 0 {
				if got.Val.Len() != 0 {
					t.Errorf("got %s, want no match", got.Val)
				}
				return
			}
			if got.Val.Len() != 4 || got.Val.Get(1).Int() != tc.start || got.Val.Get(2).Int() != tc.end {
				t.Fatalf("got %s, want match positions %d–%d", got.Val, tc.start, tc.end)
			}
			if tc.subStart != 0 {
				group := got.Val.Get(3).Get(1)
				if group.Get(1).Int() != tc.subStart || group.Get(2).Int() != tc.subEnd {
					t.Errorf("got %s, want capture positions %d–%d", group, tc.subStart, tc.subEnd)
				}
			}
		})
	}
}
