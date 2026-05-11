package builtins

import (
	"barn/types"
	"testing"
)

func TestUrlDecodeToastParity(t *testing.T) {
	cases := map[string]string{
		"a%20b":             "a b",
		"a+b":               "a+b",
		"a%2Fb%3Fc%3Dd%26e": "a/b?c=d&e",
	}

	for input, want := range cases {
		t.Run(input, func(t *testing.T) {
			result := builtinUrlDecode(types.NewTaskContext(), []types.Value{types.NewStr(input)})
			if result.IsError() {
				t.Fatalf("url_decode returned %v", result.Error)
			}
			got, ok := result.Val.(types.StrValue)
			if !ok {
				t.Fatalf("url_decode returned %T", result.Val)
			}
			if got.Value() != want {
				t.Fatalf("url_decode(%q) = %q, want %q", input, got.Value(), want)
			}
		})
	}
}

func TestUrlEncodeToastParity(t *testing.T) {
	cases := map[string]string{
		"a b":       "a%20b",
		"a/b?c=d&e": "a%2Fb%3Fc%3Dd%26e",
	}

	for input, want := range cases {
		t.Run(input, func(t *testing.T) {
			result := builtinUrlEncode(types.NewTaskContext(), []types.Value{types.NewStr(input)})
			if result.IsError() {
				t.Fatalf("url_encode returned %v", result.Error)
			}
			got, ok := result.Val.(types.StrValue)
			if !ok {
				t.Fatalf("url_encode returned %T", result.Val)
			}
			if got.Value() != want {
				t.Fatalf("url_encode(%q) = %q, want %q", input, got.Value(), want)
			}
		})
	}
}
