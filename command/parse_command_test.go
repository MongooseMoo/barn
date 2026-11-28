package command

import (
	"slices"
	"testing"

	"github.com/MongooseMoo/barn/preposition"
)

func TestParseCommandWordAndPrepositionOrder(t *testing.T) {
	for _, tc := range []struct {
		input, verb, argstr, dobj, prepAlias, iobj string
		words, args                                []string
		prep                                       preposition.Spec
	}{
		{`  put "red ball" IN FRONT OF box with lid`, "put", `"red ball" IN FRONT OF box with lid`, "red ball", "in front of", "box with lid",
			[]string{"put", "red ball", "IN", "FRONT", "OF", "box", "with", "lid"}, []string{"red ball", "IN", "FRONT", "OF", "box", "with", "lid"}, preposition.InFrontOf},
		{`take bag off of shelf`, "take", "bag off of shelf", "bag", "off", "of shelf",
			[]string{"take", "bag", "off", "of", "shelf"}, []string{"bag", "off", "of", "shelf"}, preposition.Off},
		{`  "hello\ world`, "say", `hello\ world`, "hello world", "", "",
			[]string{`"`, "hello world"}, []string{"hello world"}, preposition.None},
		{`:waves`, "emote", "waves", "waves", "", "",
			[]string{":", "waves"}, []string{"waves"}, preposition.None},
		{`; 2 + 2`, "eval", "2 + 2", "2 + 2", "", "",
			[]string{";", "2", "+", "2"}, []string{"2", "+", "2"}, preposition.None},
	} {
		t.Run(tc.input, func(t *testing.T) {
			got := ParseCommand(tc.input)
			if got.Verb != tc.verb || got.Argstr != tc.argstr || got.Dobjstr != tc.dobj ||
				got.Prep != tc.prep || got.Prepstr != tc.prepAlias || got.Iobjstr != tc.iobj ||
				!slices.Equal(got.Words, tc.words) || !slices.Equal(got.Args, tc.args) {
				t.Fatalf("ParseCommand(%q) = %+v", tc.input, got)
			}
			got.Args[0] = "changed"
			if !slices.Equal(got.Words, tc.words) {
				t.Fatalf("changing Args also changed Words to %q", got.Words)
			}
		})
	}
}

func BenchmarkParseCommand(b *testing.B) {
	for _, tc := range []struct{ name, input string }{
		{"say_6words", "say these words describe my red ball"},
		{"put_preposition", `put "red ball" in front of box`},
		{"no_prep_8words", "inspect one two three four five six seven eight"},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				ParseCommand(tc.input)
			}
		})
	}
}
