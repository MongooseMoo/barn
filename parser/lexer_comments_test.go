package parser

import (
	"errors"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/verb"
)

func TestBlockCommentProgramContract(t *testing.T) {
	for _, source := range []string{
		"/* one */ return 7; /* last */",
		"/* one */\n /* two */ /* three */\nreturn 7;",
		"return 18 /* spacer */ / /* spacer */ 3;",
		"/* outer /* inner */ return 7;",
		`return "/* text */ // text";`,
		"/**/\t/**/\r\n/**/",
	} {
		t.Run(source, func(t *testing.T) {
			if program, err := NewParser(source).ParseProgram(); err != nil || program == nil {
				t.Fatalf("valid program rejected: %v", err)
			}
		})
	}
	for _, source := range []string{
		"// one\nreturn 7;", "// one\n// two\nreturn 7;",
		"/* outer /* inner */ outer */ return 7;",
	} {
		t.Run(source, func(t *testing.T) {
			program, err := NewParser(source).ParseProgram()
			var syntax *ParseError
			if program != nil || !errors.As(err, &syntax) || syntax.Error() != "syntax error" {
				t.Fatalf("program=%v error=%v; want ordinary syntax error without partial program", program, err)
			}
		})
	}
	for _, source := range []string{"return 7; /* unfinished", "/*", "/**", "return\n/*\nunfinished"} {
		program, err := NewParser(source).ParseProgram()
		var syntax *ParseError
		if program != nil || !errors.As(err, &syntax) || syntax.Msg != "End of program while in a comment" {
			t.Fatalf("program=%v error=%v; want lexical comment diagnostic", program, err)
		}
		wantLine := 1 + strings.Count(source[:strings.Index(source, "/*")], "\n")
		if syntax.Position.Line != wantLine {
			t.Fatalf("line=%d want %d", syntax.Position.Line, wantLine)
		}
	}
}

func TestBlockCommentTokenPositions(t *testing.T) {
	source := "/*a\nb*/\n /*c*/ return 7;"
	l := NewLexer(source)
	got := l.NextToken()
	want := verb.Position{Line: 3, Column: 8, Offset: strings.Index(source, "return")}
	if got.Type != TOKEN_RETURN || got.Position != want {
		t.Fatalf("token=%v position=%+v want RETURN at %+v", got.Type, got.Position, want)
	}
	unterminated := NewLexer("\n  /* unfinished").NextToken()
	if unterminated.Type != TOKEN_ILLEGAL || unterminated.Position != (verb.Position{Line: 2, Column: 3, Offset: 3}) {
		t.Fatalf("unterminated comment token: %+v", unterminated)
	}
	if next := l.NextToken(); next.Type != TOKEN_INT || next.Value != "7" {
		t.Fatal(next)
	}
	if slash := NewLexer("/").NextToken(); slash.Type != TOKEN_SLASH {
		t.Fatal(slash)
	}
	double := NewLexer("//")
	for range 2 {
		if slash := double.NextToken(); slash.Type != TOKEN_SLASH {
			t.Fatal(slash)
		}
	}
	// Unterminated comment returns one diagnostic token, then makes progress to EOF.
	broken := NewLexer("/*")
	if broken.NextToken().Type != TOKEN_ILLEGAL || broken.NextToken().Type != TOKEN_EOF {
		t.Fatal("comment error did not reach EOF")
	}
}
