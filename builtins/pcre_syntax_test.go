package builtins

import (
	"reflect"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// Toast runs pcre_match()/pcre_replace() on PCRE2. These pin the PCRE features
// Go's RE2 regexp cannot express (moo-conformance-tests builtins/pcre.yaml,
// lookaround/backreference section, all Toast-green): every Mongoose `say`
// runs $url_utils.url_re, which ends in a negative lookbehind, through
// #1033:find_urls.

const mongooseURLRe = `(?:\w+://|www\.)[^ ,.?!#%=+][^ ]*(?<![.,;:!?)])`

func pcreMatchList(t *testing.T, subject, pattern string, caseMatters, findAll int64) types.Value {
	t.Helper()
	res := builtinPcreMatch(newTestExecution(), []types.Value{
		types.NewStr(subject), types.NewStr(pattern), types.NewInt(caseMatters), types.NewInt(findAll),
	})
	if res.Error != types.E_NONE {
		t.Fatalf("pcre_match(%q, %q): %v", subject, pattern, res.Error)
	}
	return res.Val
}

func pcreCapture(t *testing.T, entry types.Value, key string) (match string, pos []int64) {
	t.Helper()
	group, ok := entry.MapGet(types.NewStr(key))
	if !ok {
		t.Fatalf("no group %q in %v", key, entry)
	}
	m, _ := group.MapGet(types.NewStr("match"))
	p, _ := group.MapGet(types.NewStr("position"))
	for i := 1; i <= p.Len(); i++ {
		pos = append(pos, p.Get(i).Int())
	}
	return m.Str(), pos
}

func TestPcreMatchLookbehindURLPattern(t *testing.T) {
	if got := pcreMatchList(t, "Hello there, this is a benchmark message!", mongooseURLRe, 0, 1); got.Len() != 0 {
		t.Fatalf("plain speech matched: %v", got)
	}
	got := pcreMatchList(t, "see https://example.com/x, now", mongooseURLRe, 0, 1)
	if got.Len() != 1 {
		t.Fatalf("want one URL, got %v", got)
	}
	match, pos := pcreCapture(t, got.Get(1), "0")
	if match != "https://example.com/x" || !reflect.DeepEqual(pos, []int64{5, 25}) {
		t.Fatalf("got %q at %v", match, pos)
	}
}

func TestPcreMatchLookaroundAndBackreference(t *testing.T) {
	got := pcreMatchList(t, "price: $42 or $7", `(?<=\$)\d+`, 0, 1)
	if got.Len() != 2 {
		t.Fatalf("lookbehind: want 2 matches, got %v", got)
	}
	if m, _ := pcreCapture(t, got.Get(2), "0"); m != "7" {
		t.Fatalf("lookbehind second match = %q", m)
	}
	got = pcreMatchList(t, "foobar foobaz", `foo(?!bar)`, 0, 1)
	if _, pos := pcreCapture(t, got.Get(1), "0"); !reflect.DeepEqual(pos, []int64{8, 10}) {
		t.Fatalf("negative lookahead position = %v", pos)
	}
	got = pcreMatchList(t, "abcabc xyzxy", `(\w{3})\1`, 0, 1)
	whole, _ := pcreCapture(t, got.Get(1), "0")
	group, _ := pcreCapture(t, got.Get(1), "1")
	if got.Len() != 1 || whole != "abcabc" || group != "abc" {
		t.Fatalf("backreference: %v", got)
	}
}

// PCRE numbers capture groups left to right whatever their names; regexp2
// follows .NET and numbers unnamed groups first. The MOO-visible keys must be
// PCRE's: (?P<name>...)(...) yields "name" and "2", never "1".
func TestPcreMatchGroupKeysUsePCRENumbering(t *testing.T) {
	got := pcreMatchList(t, "John 42", `(?P<name>[A-Za-z]+) ([0-9]+)`, 0, 0)
	entry := got.Get(1)
	name, _ := pcreCapture(t, entry, "name")
	num, _ := pcreCapture(t, entry, "2")
	if name != "John" || num != "42" {
		t.Fatalf("name=%q 2=%q entry=%v", name, num, entry)
	}
	if _, ok := entry.MapGet(types.NewStr("1")); ok {
		t.Fatalf("unnamed group took .NET number 1: %v", entry)
	}
	// Non-participating group: empty match, empty position.
	got = pcreMatchList(t, "b", `(a)?(b)`, 0, 0)
	if m, pos := pcreCapture(t, got.Get(1), "1"); m != "" || len(pos) != 0 {
		t.Fatalf("non-participating group = %q %v", m, pos)
	}
}

func TestPcreCaptureGroupsScanner(t *testing.T) {
	for pattern, want := range map[string][]string{
		mongooseURLRe:                    nil,
		`(a)(?:b)(?<c>d)(?P<e>f)(?'g'h)`: {"", "c", "e", "g"},
		`(?<=x)(y)(?!z)[(](w)`:           {"", ""},
		`\((a)[^)]\)`:                    {""},
		`[]a](b)[^]c](d)`:                {"", ""},
		`(?i)(a)`:                        {""},
	} {
		if got := pcreCaptureGroups(pattern); !reflect.DeepEqual(got, want) {
			t.Errorf("pcreCaptureGroups(%q) = %q, want %q", pattern, got, want)
		}
	}
}

// Positions are Toast's 1-based byte positions even when the subject holds
// multi-byte characters, which regexp2 indexes by rune.
func TestPcreMatchPositionsAreByteOffsets(t *testing.T) {
	got := pcreMatchList(t, "héllo wörld", `w\S+`, 0, 0)
	match, pos := pcreCapture(t, got.Get(1), "0")
	if match != "wörld" || !reflect.DeepEqual(pos, []int64{8, 13}) {
		t.Fatalf("got %q at %v", match, pos)
	}
}

func TestPcreReplaceLookbehind(t *testing.T) {
	res := builtinPcreReplace(newTestExecution(), []types.Value{
		types.NewStr("price: $42"), types.NewStr(`s/(?<=\$)\d+/99/`),
	})
	if res.Error != types.E_NONE || res.Val.Str() != "price: $99" {
		t.Fatalf("got %v %v", res.Error, res.Val)
	}
}
