package parser

import (
	"errors"
	"strings"
	"testing"
)

func TestNestedCollectionParseErrorDetailIsBounded(t *testing.T) {
	tests := []struct {
		name string
		open string
	}{
		{name: "list", open: "{"},
		{name: "map", open: "["},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			const shallowDepth = 8
			const deepDepth = 32_000
			const maxDetailLength = 128

			shallow := parseErrorDetail(t, strings.Repeat(test.open, shallowDepth)+";")
			deep := parseErrorDetail(t, strings.Repeat(test.open, deepDepth)+";")

			// Over-limit input now stops before reaching the malformed terminal
			// token. Compare like failures while retaining the bounded-detail
			// assertion for both the shallow grammar error and depth rejection.
			limited := parseErrorDetail(t, strings.Repeat(test.open, 1024)+";")
			if deep != limited {
				t.Fatalf("depth-limit detail grows with nesting: %q versus %q", limited, deep)
			}
			if len(deep) > maxDetailLength || len(shallow) > maxDetailLength {
				t.Fatalf("detail lengths = %d, %d, want at most %d", len(shallow), len(deep), maxDetailLength)
			}
		})
	}
}

func parseErrorDetail(t *testing.T, source string) string {
	t.Helper()

	_, err := NewParser(source).ParseProgram()
	if err == nil {
		t.Fatal("ParseProgram() succeeded, want syntax error")
	}

	var parseErr *ParseError
	if !errors.As(err, &parseErr) {
		t.Fatalf("ParseProgram() error = %T, want *ParseError", err)
	}
	if parseErr.Detail == nil {
		t.Fatal("ParseError.Detail is nil")
	}

	return parseErr.Detail.Error()
}
