package compiler

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/sourcekey"
)

func TestCompileCacheCorpusDigest(t *testing.T) {
	digest := sha256.New()
	for _, statements := range []int{128, 256, 512} {
		for initial := range 16 {
			lines := coldCompileLines(statements, initial)
			program, diagnostics := New(nil).CompileMOOWithKey(lines, sourcekey.Of(lines))
			if program == nil || len(diagnostics) != 0 {
				t.Fatal("corpus compilation failed")
			}
			fmt.Fprintf(digest, "%x/%v/%d/%v/%x/%v\n", program.Code, program.VarNames, program.NumLocals, program.LineInfo, program.BuiltinLayout, program.BuiltinSlots)
			for _, value := range program.Constants {
				fmt.Fprintf(digest, "%d/%s;", value.Type(), value.String())
			}
			fmt.Fprintln(digest, strings.Join(program.Source, "\n"))
		}
	}
	t.Logf("CORPUS_DIGEST=%x", digest.Sum(nil))
}

// Each cold operation creates a fresh compiler and releases a fixed burst of
// callers together. Source hashing and corpus setup are outside the timed loop.
func BenchmarkCompileCacheCold(b *testing.B) {
	for _, scenario := range []struct {
		name       string
		callers    int
		statements int
		distinct   bool
	}{
		{"Shared16", 16, 256, false},
		{"Distinct16", 16, 256, true},
		{"Single", 1, 256, false},
		{"HoldoutShared8", 8, 512, false},
		{"HoldoutShared32", 32, 128, false},
	} {
		b.Run(scenario.name, func(b *testing.B) {
			lines := make([][]string, scenario.callers)
			keys := make([]sourcekey.Key, scenario.callers)
			for i := range scenario.callers {
				initial := 0
				if scenario.distinct {
					initial = i
				}
				lines[i] = coldCompileLines(scenario.statements, initial)
				keys[i] = sourcekey.Of(lines[i])
			}
			compilers := make([]*Compiler, scenario.callers)
			b.ReportAllocs()
			for b.Loop() {
				c := New(nil)
				for i := range compilers {
					compilers[i] = c
				}
				results := compileBurst(compilers, lines, keys)
				for i, result := range results {
					if result.program == nil || len(result.diagnostics) != 0 || len(result.program.Source) != scenario.statements+2 {
						b.Fatalf("caller %d: invalid compilation result", i)
					}
				}
			}
		})
	}
}

func BenchmarkCompileCacheWarm(b *testing.B) {
	for _, parallel := range []bool{false, true} {
		b.Run(fmt.Sprintf("Parallel=%t", parallel), func(b *testing.B) {
			lines := coldCompileLines(256, 0)
			key := sourcekey.Of(lines)
			c := New(nil)
			want, diagnostics := c.CompileMOOWithKey(lines, key)
			if want == nil || len(diagnostics) != 0 {
				b.Fatal("warm-up compilation failed")
			}
			b.ReportAllocs()
			if parallel {
				b.ResetTimer()
				b.RunParallel(func(pb *testing.PB) {
					for pb.Next() {
						got, diagnostics := c.CompileMOOWithKey(lines, key)
						if got != want || len(diagnostics) != 0 {
							b.Error("warm hit changed")
							return
						}
					}
				})
				return
			}
			for b.Loop() {
				got, diagnostics := c.CompileMOOWithKey(lines, key)
				if got != want || len(diagnostics) != 0 {
					b.Fatal("warm hit changed")
				}
			}
		})
	}
}
