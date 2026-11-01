package parser_test

import (
	"testing"

	"github.com/MongooseMoo/barn/internal/fuzzseed"
	"github.com/MongooseMoo/barn/parser"
)

func FuzzParseFormatRoundTrip(f *testing.F) {
	fuzzseed.Add(f)
	f.Fuzz(func(t *testing.T, source string) {
		program, err := parser.NewParser(source).ParseProgram()
		if err != nil {
			return
		}
		assertDirectProgramFormatRoundTrip(t, program)
	})
}
