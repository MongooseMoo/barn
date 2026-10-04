package parser

import (
	"errors"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/verb"
)

func TestRawNULSourceRejected(t *testing.T) {
	for _, tc := range []struct {
		source       string
		line, column int
	}{
		{"\x00return 2;", 1, 1},
		{"return 1;\x00return 2;", 1, 10},
		{"\nreturn 1;\x00", 2, 10},
		{"return \"a\x00b\";", 1, 10},
		{"return \"a\\\x00b\";", 1, 11},
		{"/*\n \x00 */ return 1;", 2, 2},
		{"return 1;\x00\x00/* unfinished", 1, 10},
	} {
		t.Run(tc.source, func(t *testing.T) {
			program, err := NewParser(tc.source).ParseProgram()
			var syntax *ParseError
			if program != nil || !errors.As(err, &syntax) || !errors.Is(err, ErrNULInSource) || syntax.Line != tc.line || syntax.Msg != "NUL byte is not representable in MOO source" {
				t.Fatalf("program=%v error=%+v; want explicit NUL error on line %d without partial program", program, err, tc.line)
			}
			l := NewLexer(tc.source)
			var illegal Token
			for range len(tc.source) + 1 {
				token := l.NextToken()
				if token.Type == TOKEN_ILLEGAL {
					illegal = token
					break
				}
				if token.Type == TOKEN_EOF {
					t.Fatal("raw NUL silently became EOF")
				}
			}
			want := verb.Position{Line: tc.line, Column: tc.column, Offset: strings.IndexByte(tc.source, 0)}
			if illegal.Position != want || !strings.Contains(illegal.Value, "NUL") {
				t.Fatalf("token=%+v want illegal NUL at %+v", illegal, want)
			}
			for range len(tc.source) + 1 {
				if l.NextToken().Type == TOKEN_EOF {
					return
				}
			}
			t.Fatal("lexer failed to make progress after NUL")
		})
	}
}
