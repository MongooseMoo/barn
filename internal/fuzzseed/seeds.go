// Package fuzzseed shares verb-source seeds across the frontend fuzz targets.
package fuzzseed

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	dbformat "github.com/MongooseMoo/barn/db/format"
	"github.com/MongooseMoo/barn/db/store"
)

// Add includes every programmed Test.db verb, syntax boundaries, and an optional
// local corpus exported by scripts/fuzz_corpus.py. Identical bytes run once.
// The callers live one directory below the repository root.
func Add(f *testing.F) {
	f.Helper()
	seen := make(map[string]bool)
	add := func(source string) {
		if !seen[source] {
			seen[source] = true
			f.Add(source)
		}
	}
	database, err := dbformat.LoadDatabaseReadOnly(filepath.Join("..", "Test.db"))
	if err != nil {
		f.Fatal(err)
	}
	loaded, err := database.NewStoreFromDatabase()
	if err != nil {
		f.Fatal(err)
	}
	snapshot := loaded.Snapshot()
	objects := append([]*store.SnapshotObject(nil), snapshot.AnonymousObjects...)
	for _, object := range snapshot.Objects {
		objects = append(objects, object)
	}
	sort.Slice(objects, func(i, j int) bool { return objects[i].ID < objects[j].ID })
	programs := 0
	for _, object := range objects {
		for _, verb := range object.VerbList {
			if verb.HasProgram {
				programs++
				source := strings.Join(verb.Code, "\n")
				if len(verb.Code) != 0 {
					source += "\n"
				}
				add(source)
			}
		}
	}
	if programs != database.DeclaredPrograms {
		f.Fatalf("seed extraction found %d programs, dump declares %d", programs, database.DeclaredPrograms)
	}
	for _, source := range []string{
		"", "\x00", "\xff", "/*", "\"", "return (", "return 1;\x00return 2;",
		"return 1 << 1 &. 2;", "return 1 << 3 >> 2;",
		"return a = b = 1;", "return a ? b | (c ? d | e);",
		"return `args ! ANY => 0';", "return `1 / 0 ! E_DIV';",
		"return {1, 2, 3}[^..^ + 1];", "return {1, 2, 3}[-1 + $..$];",
		"{a, ?b = 2, @rest} = {1}; return rest;",
		"items = {1, 2}; items[^..$] = {3}; return items;",
		"return [1 -> {2}, \"key\" -> #0];", "return #0:(\"verb\")(@args);",
		"if (1) elseif (0) else endif", "while loop (1) break loop; endwhile",
		"for value, key in ([1 -> 2]) continue; endfor",
		"try return 1 / 0; except e (ANY) return e; finally 1; endtry",
		"fork child (0) return 1; endfork return child;",
		"suspend(0); fork (0) suspend(); endfork return {1, \"a\\\"\", [2 -> #-1], E_TYPE};",
		"/* one *//* two */ return \"a\\\"b\\\\c\xff\";",
		"return 9223372036854775807;", "return -9223372036854775808;",
		"return 18446744073709551616;", "return 1e309;", "return 1e-400;",
		"return " + strings.Repeat("(", 256) + "1" + strings.Repeat(")", 256) + ";",
		"return " + strings.Repeat("(", 257) + "1" + strings.Repeat(")", 257) + ";",
	} {
		add(source)
	}
	if directory := os.Getenv("BARN_FUZZ_CORPUS"); directory != "" {
		entries, err := os.ReadDir(directory)
		if err != nil {
			f.Fatal(err)
		}
		count := 0
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".moo" {
				continue
			}
			source, err := os.ReadFile(filepath.Join(directory, entry.Name()))
			if err != nil {
				f.Fatal(err)
			}
			add(string(source))
			count++
		}
		if count == 0 {
			f.Fatal("BARN_FUZZ_CORPUS contains no .moo seeds")
		}
	}
	f.Logf("seeded %d unique sources (%d Test.db programs before deduplication)", len(seen), programs)
}
