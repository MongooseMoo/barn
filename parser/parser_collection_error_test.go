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

			if deep != shallow {
				t.Fatalf("detail grows with nesting depth: len(depth %d) = %d, len(depth %d) = %d",
					shallowDepth, len(shallow), deepDepth, len(deep))
			}
			if len(deep) > maxDetailLength {
				t.Fatalf("detail length = %d, want at most %d", len(deep), maxDetailLength)
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
