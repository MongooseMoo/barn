package vm

import (
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/MongooseMoo/barn/compiler"
	"github.com/MongooseMoo/barn/sourcekey"
	"github.com/MongooseMoo/barn/types"
)

// Every task running a program shares its call sites. Concurrent first uses
// agree on one site per slot, and concurrent calls of verbs whose code differs
// each get the program compiled from their own code.
func TestVerbCallSiteSharedAcrossGoroutines(t *testing.T) {
	c := compiler.New(map[string]int{})
	caller, diagnostics := c.CompileMOO([]string{"return x:v();"})
	if len(diagnostics) != 0 {
		t.Fatalf("compile caller: %v", diagnostics)
	}
	index := slices.IndexFunc(caller.Constants, func(v types.Value) bool { return v.Type() == types.TYPE_STR && v.Str() == "v" })
	if index < 0 {
		t.Fatalf("no constant names v: %v", caller.Constants)
	}
	sources := make([][]string, 4)
	for i := range sources {
		sources[i] = []string{fmt.Sprintf("return %d;", i)}
	}

	var wg sync.WaitGroup
	sites := make([]*verbCallSite, 8)
	for g := range sites {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			site := callSiteAt(caller, index)
			sites[g] = site
			for i := 0; i < 200; i++ {
				code := sources[(g+i)%len(sources)]
				program, diagnostics := site.compile(c, code, sourcekey.Of(code))
				if len(diagnostics) != 0 || !slices.Equal(program.Source, code) {
					t.Errorf("goroutine %d call %d: source %v, want %v (%v)", g, i, program.Source, code, diagnostics)
					return
				}
			}
		}(g)
	}
	wg.Wait()
	for g, site := range sites {
		if site == nil || site != sites[0] {
			t.Fatalf("goroutine %d got site %p, want %p", g, site, sites[0])
		}
	}
}

// A program from another compiler is never reused, even for the same source.
func TestVerbCallSiteProgramBelongsToItsCompiler(t *testing.T) {
	code := []string{"return 1;"}
	key := sourcekey.Of(code)
	site := &verbCallSite{}
	first, second := compiler.New(map[string]int{}), compiler.New(map[string]int{})
	p1, _ := site.compile(first, code, key)
	if again, _ := site.compile(first, code, key); again != p1 {
		t.Fatal("same compiler and key did not reuse the site's program")
	}
	if p2, _ := site.compile(second, code, key); p2 == p1 {
		t.Fatal("another compiler was handed the first compiler's program")
	}
}
