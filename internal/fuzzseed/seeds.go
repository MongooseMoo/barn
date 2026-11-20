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
		// Test.db has no range loop or range assignment, and mutation of the
		// verb seeds never produced one (compileRangeLoop, compileNestedRangeAssign).
		"n = 3; t = 0; for x in [1..n] t = t + x; endfor return t;",
		"for x in [n = 2..n + 1] endfor for y in [5..1] return y; endfor return x;",
		"for x in [\"a\"..\"b\"] endfor",
		"t = {}; for i, x in ({\"a\", \"b\", \"c\"}) t = {@t, {i, x}}; endfor return t;",
		"t = 0; for v, k in ([\"a\" -> 1, \"b\" -> 2]) t = t + v; endfor return {t, k};",
		"for c in (\"abc\") if (c == \"b\") break; endif endfor return c;",
		"outer = {}; for i in [1..3] for j in [1..3] if (j == 2) continue i; elseif (i == 3) break i; endif outer = {@outer, {i, j}}; endfor endfor return outer;",
		"for i in [1..2] while inner (1) break i; endwhile endfor",
		"for v, k in ({1, 2}) for w in [1..k] continue v; endfor endfor",
		"l = {1, 2, 3, 4}; l[2..3] = {\"x\"}; return l;",
		"l = {1, {2, 3, 4}, 5}; l[2][2..3] = {9}; return l;",
		"l = {{1, 2}, {3, 4}}; l[$][^..$] = {}; l[^][$..^] = {7}; return l;",
		"s = \"hello\"; s[2..4] = \"EY\"; s[$..$] = \"!\"; return s;",
		"l = {\"abc\", \"def\"}; l[1][2..2] = \"X\"; l[2][4..3] = \"Y\"; return l;",
		"m = [\"k\" -> {1, 2, 3}]; m[\"k\"][2..$] = {0}; return m;",
		"l = {1, 2}; l[3..5] = {}; l[0..0] = {0}; return l;",
		"s = \"ab\"; s[0..0] = \"z\"; s[5..6] = \"y\"; return s;",
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
