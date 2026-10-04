package compiler

import (
	"testing"
	"time"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/sourcekey"
)

func ownCompilation(t *testing.T, cache *programCache, key sourcekey.Key) *compilation {
	t.Helper()
	program, call, owner := cache.beginCompile(key)
	if program != nil || call == nil || !owner {
		t.Fatal("expected ownership of an uncached compilation")
	}
	return call
}

func TestProgramCacheSharesActiveCompilationAndRechecksCompletedEntry(t *testing.T) {
	cache := newProgramCache(2)
	key := sourcekey.Of([]string{"return 1;"})
	owner := ownCompilation(t, cache, key)
	program, waiter, owns := cache.beginCompile(key)
	if program != nil || waiter != owner || owns {
		t.Fatal("same-key miss did not join the active compilation")
	}
	returned := make(chan compileResult, 1)
	go func() {
		program, diagnostics := waiter.wait()
		returned <- compileResult{program, diagnostics}
	}()
	want := &bytecode.Program{Code: []byte{byte(bytecode.OP_RETURN_NONE)}}
	owner.program = want
	cache.endCompile(key, owner)
	select {
	case got := <-returned:
		if got.program != want || len(got.diagnostics) != 0 {
			t.Fatal("waiter received a different compilation result")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("completed compilation did not release its waiter")
	}
	// A caller whose original get missed may reach beginCompile after completion.
	program, call, owns := cache.beginCompile(key)
	if program != want || call != nil || owns || cache.flights != nil {
		t.Fatal("late cold lookup started duplicate work or retained flight state")
	}
}

func TestProgramCacheAllowsIndependentCompilations(t *testing.T) {
	cache := newProgramCache(2)
	firstKey := sourcekey.Of([]string{"return 1;"})
	secondKey := sourcekey.Of([]string{"return 2;"})
	first := ownCompilation(t, cache, firstKey)
	started := make(chan *compilation, 1)
	go func() {
		_, call, owner := cache.beginCompile(secondKey)
		if !owner {
			started <- nil
			return
		}
		started <- call
	}()
	var second *compilation
	select {
	case second = <-started:
		if second == nil || second == first {
			t.Fatal("different keys did not receive independent ownership")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("unfinished compilation blocked an unrelated key")
	}
	first.program = &bytecode.Program{}
	cache.endCompile(firstKey, first)
	if cache.flights[secondKey] != second {
		t.Fatal("finishing one key removed another key's active compilation")
	}
	second.program = &bytecode.Program{}
	cache.endCompile(secondKey, second)
	if cache.flights != nil {
		t.Fatal("completed independent compilations retained flight state")
	}
}

func TestProgramCacheDiagnosticWaitersAreReleasedAndCanRetry(t *testing.T) {
	cache := newProgramCache(2)
	key := sourcekey.Of([]string{"return absent();"})
	owner := ownCompilation(t, cache, key)
	const waiters = 8
	returned := make(chan compileResult, waiters)
	for range waiters {
		_, call, owns := cache.beginCompile(key)
		if owns || call != owner {
			t.Fatal("diagnostic waiter did not join the existing compilation")
		}
		go func() {
			program, diagnostics := call.wait()
			returned <- compileResult{program, diagnostics}
		}()
	}
	owner.diagnostics = []Diagnostic{{Stage: BytecodeStage, Message: "Unknown built-in function: absent"}}
	cache.endCompile(key, owner)
	for range waiters {
		select {
		case result := <-returned:
			if result.program != nil || len(result.diagnostics) != 1 || result.diagnostics[0].Message != owner.diagnostics[0].Message {
				t.Fatal("waiter did not receive the original diagnostic")
			}
			result.diagnostics[0].Message = "caller edit"
		case <-time.After(5 * time.Second):
			t.Fatal("diagnostic completion stranded a waiter")
		}
	}
	if _, ok := cache.get(key); ok || cache.flights != nil {
		t.Fatal("failed compilation retained a cache entry or active flight")
	}
	retried := ownCompilation(t, cache, key)
	if retried == owner {
		t.Fatal("diagnostic result was permanently cached")
	}
	retried.program = &bytecode.Program{}
	cache.endCompile(key, retried)
}

func TestProgramCacheRetainsBoundedLRUAndCompletedFlightResult(t *testing.T) {
	cache := newProgramCache(2)
	keys := []sourcekey.Key{
		sourcekey.Of([]string{"return 1;"}),
		sourcekey.Of([]string{"return 2;"}),
		sourcekey.Of([]string{"return 3;"}),
	}
	first := ownCompilation(t, cache, keys[0])
	first.program = &bytecode.Program{}
	cache.endCompile(keys[0], first)
	for _, key := range keys[1:] {
		call := ownCompilation(t, cache, key)
		call.program = &bytecode.Program{}
		cache.endCompile(key, call)
	}
	if _, ok := cache.get(keys[0]); ok || len(cache.entries) != 2 || cache.lru.Len() != 2 {
		t.Fatal("completed flights changed bounded LRU eviction")
	}
	if got, _ := first.wait(); got != first.program {
		t.Fatal("eviction changed a completed flight's result")
	}
	// Touch the second entry before reloading the first; the third must be evicted.
	cache.get(keys[1])
	reloaded := ownCompilation(t, cache, keys[0])
	reloaded.program = &bytecode.Program{}
	cache.endCompile(keys[0], reloaded)
	if _, ok := cache.get(keys[2]); ok {
		t.Fatal("recompilation ignored LRU recency")
	}
}

func TestProgramCachePanicReleasesWaitersAndFlight(t *testing.T) {
	cache := newProgramCache(2)
	key := sourcekey.Of([]string{"return 1;"})
	owner := ownCompilation(t, cache, key)
	_, waiter, _ := cache.beginCompile(key)
	recovered := make(chan any, 2)
	go func() {
		defer func() { recovered <- recover() }()
		waiter.wait()
	}()
	go func() {
		defer func() { recovered <- recover() }()
		cache.runCompile(key, owner, func() (*bytecode.Program, []Diagnostic) {
			panic("compile panic")
		})
	}()
	for range 2 {
		select {
		case got := <-recovered:
			if got != "compile panic" {
				t.Fatalf("caller panic=%v, want compile panic", got)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("panic completion stranded a caller")
		}
	}
	if cache.flights != nil {
		t.Fatal("panic retained flight state")
	}
	retried := ownCompilation(t, cache, key)
	retried.program = &bytecode.Program{}
	cache.endCompile(key, retried)
}
