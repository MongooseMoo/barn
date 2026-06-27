package command

import "testing"

func TestLookupIntrinsicBeforeDoCommand(t *testing.T) {
	tests := []struct {
		verb string
		want IntrinsicCommand
	}{
		{verb: ".pr", want: IntrinsicProgram},
		{verb: ".program", want: IntrinsicProgram},
		{verb: "PREFIX", want: IntrinsicPrefix},
		{verb: "outputprefix", want: IntrinsicPrefix},
		{verb: "SUFFIX", want: IntrinsicSuffix},
		{verb: "outputsuffix", want: IntrinsicSuffix},
		{verb: "eval", want: IntrinsicNone},
		{verb: ".p", want: IntrinsicNone},
	}

	for _, tt := range tests {
		if got := LookupIntrinsic(tt.verb, IntrinsicBeforeDoCommand); got != tt.want {
			t.Fatalf("LookupIntrinsic(%q, before) = %v, want %v", tt.verb, got, tt.want)
		}
	}
}

func TestLookupIntrinsicAfterVerbMiss(t *testing.T) {
	tests := []struct {
		verb string
		want IntrinsicCommand
	}{
		{verb: "eval", want: IntrinsicEval},
		{verb: "EVAL", want: IntrinsicEval},
		{verb: "prefix", want: IntrinsicNone},
	}

	for _, tt := range tests {
		if got := LookupIntrinsic(tt.verb, IntrinsicAfterVerbMiss); got != tt.want {
			t.Fatalf("LookupIntrinsic(%q, after miss) = %v, want %v", tt.verb, got, tt.want)
		}
	}
}
