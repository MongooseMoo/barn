package compiler

import (
	"fmt"
	"reflect"
	"runtime"
	"sync"
	"testing"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/sourcekey"
)

type compileResult struct {
	program     *bytecode.Program
	diagnostics []Diagnostic
}

func coldCompileLines(statements, initial int) []string {
	lines := make([]string, statements+2)
	lines[0] = fmt.Sprintf("value = %d;", initial)
	for i := 1; i <= statements; i++ {
		lines[i] = "value = value + 1;"
	}
	lines[len(lines)-1] = "return value;"
	return lines
}

func compileBurst(compilers []*Compiler, lines [][]string, keys []sourcekey.Key) []compileResult {
	results := make([]compileResult, len(compilers))
	start := make(chan struct{})
	var ready, done sync.WaitGroup
	ready.Add(len(compilers))
	done.Add(len(compilers))
	for i := range compilers {
		go func() {
			defer done.Done()
			ready.Done()
			<-start
			results[i].program, results[i].diagnostics = compilers[i].CompileMOOWithKey(lines[i], keys[i])
		}()
	}
	ready.Wait()
	close(start)
	done.Wait()
	return results
}

func TestCompileMOOWithKeyCoalescesColdBurst(t *testing.T) {
	previous := runtime.GOMAXPROCS(4)
	t.Cleanup(func() { runtime.GOMAXPROCS(previous) })
	const callers = 16
	lines := coldCompileLines(4096, 0)
	key := sourcekey.Of(lines)
	c := New(nil)
	compilers := make([]*Compiler, callers)
	sources := make([][]string, callers)
	keys := make([]sourcekey.Key, callers)
	for i := range callers {
		compilers[i], sources[i], keys[i] = c, lines, key
	}
	results := compileBurst(compilers, sources, keys)
	for i, result := range results {
		if result.program == nil || len(result.diagnostics) != 0 {
			t.Fatalf("caller %d: program=%p diagnostics=%v", i, result.program, result.diagnostics)
		}
		if result.program != results[0].program {
			t.Fatalf("cold callers compiled distinct programs: caller 0=%p caller %d=%p", results[0].program, i, result.program)
		}
	}
	reference, diagnostics := New(nil).CompileMOOWithKey(lines, key)
	if len(diagnostics) != 0 || !reflect.DeepEqual(results[0].program, reference) {
		t.Fatal("shared compilation differs from an independent compilation")
	}
}

func TestCompileMOOWithKeyConcurrentDiagnostics(t *testing.T) {
	for _, lines := range [][]string{{"if (1)", "return 1;"}, {"return absent_builtin();"}} {
		t.Run(lines[0], func(t *testing.T) {
			c := New(nil)
			key := sourcekey.Of(lines)
			compilers := []*Compiler{c, c, c, c}
			sources := [][]string{lines, lines, lines, lines}
			keys := []sourcekey.Key{key, key, key, key}
			results := compileBurst(compilers, sources, keys)
			for i, result := range results {
				if result.program != nil || len(result.diagnostics) != 1 {
					t.Fatalf("caller %d: program=%p diagnostics=%v", i, result.program, result.diagnostics)
				}
				if result.diagnostics[0].Error() != results[0].diagnostics[0].Error() {
					t.Fatalf("caller %d diagnostic differs", i)
				}
			}
			want := results[1].diagnostics[0].Error()
			results[0].diagnostics[0].Message = "caller-owned edit"
			if results[1].diagnostics[0].Error() != want {
				t.Fatal("callers share writable diagnostic storage")
			}
			_, retried := c.CompileMOOWithKey(lines, key)
			if len(retried) != 1 || retried[0].Error() != want {
				t.Fatal("diagnostic retry changed")
			}
			if _, ok := c.cache.get(key); ok {
				t.Fatal("failed compilation was cached")
			}
		})
	}
}

func TestConcurrentCompileRegistryIsolation(t *testing.T) {
	lines := []string{"return registry_target();"}
	key := sourcekey.Of(lines)
	first := New(map[string]int{"registry_target": 17})
	second := New(map[string]int{"registry_target": 23})
	results := compileBurst([]*Compiler{first, second}, [][]string{lines, lines}, []sourcekey.Key{key, key})
	for i, want := range []byte{17, 23} {
		if len(results[i].diagnostics) != 0 {
			t.Fatalf("compiler %d: %v", i, results[i].diagnostics)
		}
		if got := compiledBuiltinID(t, results[i].program); got != want {
			t.Fatalf("compiler %d builtin ID=%d, want %d", i, got, want)
		}
	}
	if results[0].program == results[1].program {
		t.Fatal("different compiler registries shared a program")
	}
}
