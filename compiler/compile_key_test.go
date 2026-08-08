package compiler

import (
	"testing"

	"github.com/MongooseMoo/barn/sourcekey"
)

func TestCompileMOOWithKeyHitsTheSameCacheEntryAsCompileMOO(t *testing.T) {
	lines := []string{"return 41 + 1;"}
	viaHash, diagnostics := CompileMOO(lines, testRegistry{})
	if len(diagnostics) > 0 {
		t.Fatalf("CompileMOO: %v", diagnostics[0])
	}
	viaKey, diagnostics := CompileMOOWithKey(lines, sourcekey.Of(lines), testRegistry{})
	if len(diagnostics) > 0 {
		t.Fatalf("CompileMOOWithKey: %v", diagnostics[0])
	}
	if viaKey != viaHash {
		t.Fatalf("precomputed key missed the cache: got %p, want %p", viaKey, viaHash)
	}
	// And the reverse direction: a program first compiled through the key path is
	// served to the hashing path.
	fresh := []string{"return \"key-first\";"}
	first, diagnostics := CompileMOOWithKey(fresh, sourcekey.Of(fresh), testRegistry{})
	if len(diagnostics) > 0 {
		t.Fatalf("CompileMOOWithKey fresh: %v", diagnostics[0])
	}
	second, diagnostics := CompileMOO(fresh, testRegistry{})
	if len(diagnostics) > 0 {
		t.Fatalf("CompileMOO fresh: %v", diagnostics[0])
	}
	if second != first {
		t.Fatalf("hash path missed the key-path entry: got %p, want %p", second, first)
	}
}

func TestCompileMOOWithKeyFallsBackWhenKeyIsUnset(t *testing.T) {
	lines := []string{"return \"unset-key\";"}
	var unset sourcekey.Key
	program, diagnostics := CompileMOOWithKey(lines, unset, testRegistry{})
	if len(diagnostics) > 0 {
		t.Fatalf("CompileMOOWithKey unset: %v", diagnostics[0])
	}
	if program == nil {
		t.Fatalf("CompileMOOWithKey returned no program for an unset key")
	}
	hashed, diagnostics := CompileMOO(lines, testRegistry{})
	if len(diagnostics) > 0 {
		t.Fatalf("CompileMOO: %v", diagnostics[0])
	}
	if hashed != program {
		t.Fatalf("unset key did not fall back to the source hash: got %p, want %p", hashed, program)
	}
}

func TestCompileMOOWithKeyReportsDiagnosticsLikeCompileMOO(t *testing.T) {
	lines := []string{"if (1)", "return 1;"}
	keyed, keyedDiagnostics := CompileMOOWithKey(lines, sourcekey.Of(lines), testRegistry{})
	hashed, hashedDiagnostics := CompileMOO(lines, testRegistry{})
	if len(keyedDiagnostics) == 0 || len(hashedDiagnostics) == 0 {
		t.Fatalf("expected diagnostics from both paths, got %d and %d", len(keyedDiagnostics), len(hashedDiagnostics))
	}
	if keyed != nil || hashed != nil {
		t.Fatalf("a failed compile must return no program")
	}
	if keyedDiagnostics[0].Error() != hashedDiagnostics[0].Error() {
		t.Fatalf("diagnostics differ: %q vs %q", keyedDiagnostics[0].Error(), hashedDiagnostics[0].Error())
	}
}

// The verb-call hot path takes this branch on every call: a cache hit with a
// precomputed key must not allocate (the whole point of carrying the key is to
// stop hashing the source, which allocated ~1.5GB per 28s on real-mongoose).
func TestCachedCompileWithKeyDoesNotAllocate(t *testing.T) {
	lines := []string{"x = 1;", "return x + 2;"}
	key := sourcekey.Of(lines)
	if _, diagnostics := CompileMOOWithKey(lines, key, testRegistry{}); len(diagnostics) > 0 {
		t.Fatalf("warm-up compile: %v", diagnostics[0])
	}
	allocs := testing.AllocsPerRun(100, func() {
		CompileMOOWithKey(lines, key, testRegistry{})
	})
	if allocs != 0 {
		t.Fatalf("cache hit with a precomputed key allocated %v times per call", allocs)
	}
}

func TestDistinctSourcesCompileToDistinctPrograms(t *testing.T) {
	before := []string{"return 1;"}
	after := []string{"return 2;"}
	beforeProgram, _ := CompileMOOWithKey(before, sourcekey.Of(before), testRegistry{})
	afterProgram, _ := CompileMOOWithKey(after, sourcekey.Of(after), testRegistry{})
	if beforeProgram == afterProgram {
		t.Fatalf("edited source served the stale cached program")
	}
}
