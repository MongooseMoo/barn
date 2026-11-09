package parser_test

import (
	"slices"
	"strings"
	"testing"

	dbformat "github.com/MongooseMoo/barn/db/format"
	"github.com/MongooseMoo/barn/parser"
)

func BenchmarkParseProgram(b *testing.B) {
	b.Run("TestDB", func(b *testing.B) {
		database, err := dbformat.LoadDatabaseReadOnly("../Test.db")
		if err != nil {
			b.Fatal(err)
		}
		loaded, err := database.NewStoreFromDatabase()
		if err != nil {
			b.Fatal(err)
		}
		snapshot := loaded.Snapshot()
		objects := snapshot.AnonymousObjects
		for _, object := range snapshot.Objects {
			objects = append(objects, object)
		}
		var sources []string
		var bytes int64
		for _, object := range objects {
			for _, verb := range object.VerbList {
				if verb.HasProgram {
					source := strings.Join(verb.Code, "\n")
					sources = append(sources, source)
					bytes += int64(len(source))
				}
			}
		}
		if len(sources) != database.DeclaredPrograms || len(sources) == 0 {
			b.Fatalf("found %d programs, dump declares %d", len(sources), database.DeclaredPrograms)
		}
		slices.Sort(sources)
		b.ReportAllocs()
		b.SetBytes(bytes)
		for b.Loop() {
			// Parse every program, including any rejected source. The database
			// census tests own acceptance; this measures the complete workload.
			for _, source := range sources {
				_, _ = parser.NewParser(source).ParseProgram()
			}
		}
		b.ReportMetric(float64(len(sources)), "programs/op")
	})
	for _, test := range []struct {
		name   string
		source string
	}{
		{"Nested", "return " + strings.Repeat("(", 128) + "a + b * c ^ d" + strings.Repeat(")", 128) + ";"},
		{"Malformed", "return " + strings.Repeat("(", 128) + "a + b * ;"},
	} {
		b.Run(test.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(test.source)))
			for b.Loop() {
				_, _ = parser.NewParser(test.source).ParseProgram()
			}
		})
	}
}
