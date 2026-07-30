package builtins

import (
	"bytes"
	cryptorand "crypto/rand"
	rand "math/rand/v2"
	"sync"
	"testing"

	"barn/kernel"
	"barn/types"
)

func TestRandomFamilySharesReseededGenerator(t *testing.T) {
	var seed [32]byte
	for i := range seed {
		seed[i] = byte(i)
	}
	entropy := bytes.NewReader(append(seed[:], seed[:]...))
	originalReader := cryptorand.Reader
	cryptorand.Reader = entropy
	t.Cleanup(func() {
		cryptorand.Reader = originalReader
	})

	ctx := kernel.NewTaskContext()
	ctx.IsWizard = true

	assertReseededSequence := func() {
		t.Helper()

		entropyBefore := entropy.Len()
		result := builtinReseedRandom(ctx, nil)
		if result.IsError() {
			t.Fatalf("reseed_random() failed: %v", result.Error)
		}
		if result.Val.Type() != types.TYPE_INT || result.Val.Int() != 0 {
			t.Fatalf("reseed_random() = %#v, want integer 0", result.Val)
		}

		consumed := entropyBefore - entropy.Len()
		if consumed != 32 {
			t.Fatalf("reseed_random() consumed %d entropy bytes, want a 32-byte seed", consumed)
		}

		source := rand.NewChaCha8(seed)
		reference := rand.New(source)

		integer := builtinRandom(ctx, []types.Value{types.NewInt(1_000_000)})
		if integer.IsError() {
			t.Fatalf("random() failed: %v", integer.Error)
		}
		if got, want := integer.Val.Int(), reference.Int64N(1_000_000)+1; got != want {
			t.Fatalf("random() = %d, want %d from reseeded shared stream", got, want)
		}

		floating := builtinFrandom(ctx, []types.Value{types.NewFloat(1)})
		if floating.IsError() {
			t.Fatalf("frandom() failed: %v", floating.Error)
		}
		if got, want := floating.Val.Float(), reference.Float64(); got != want {
			t.Fatalf("frandom() = %v, want %v from reseeded shared stream", got, want)
		}

		randomBytes := builtinRandomBytes(ctx, []types.Value{types.NewInt(16)})
		if randomBytes.IsError() {
			t.Fatalf("random_bytes() failed: %v", randomBytes.Error)
		}
		gotBytes, invalid := decodeBinaryString(randomBytes.Val.Str())
		if invalid {
			t.Fatalf("random_bytes() returned invalid binary string %q", randomBytes.Val.Str())
		}
		wantBytes := make([]byte, 16)
		if _, err := source.Read(wantBytes); err != nil {
			t.Fatalf("reference random byte draw failed: %v", err)
		}
		if !bytes.Equal(gotBytes, wantBytes) {
			t.Fatalf("random_bytes() = %x, want %x from reseeded shared stream", gotBytes, wantBytes)
		}

		integer = builtinRandom(ctx, []types.Value{types.NewInt(1_000_000)})
		if integer.IsError() {
			t.Fatalf("second random() failed: %v", integer.Error)
		}
		if got, want := integer.Val.Int(), reference.Int64N(1_000_000)+1; got != want {
			t.Fatalf("random() after random_bytes() = %d, want %d from shared stream", got, want)
		}
	}

	assertReseededSequence()
	assertReseededSequence()
}

func TestReseedRandomRejectsNonWizard(t *testing.T) {
	ctx := kernel.NewTaskContext()
	result := builtinReseedRandom(ctx, nil)
	if !result.IsError() || result.Error != types.E_PERM {
		t.Fatalf("non-wizard reseed_random() = %#v, want E_PERM", result)
	}
}

func TestRandomFamilyConcurrentUse(t *testing.T) {
	const workers = 8
	const iterations = 100

	start := make(chan struct{})
	errors := make(chan types.ErrorCode, workers+1)
	var wg sync.WaitGroup

	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start

			ctx := kernel.NewTaskContext()
			for range iterations {
				results := []types.Result{
					builtinRandom(ctx, []types.Value{types.NewInt(1_000_000)}),
					builtinFrandom(ctx, []types.Value{types.NewFloat(1)}),
					builtinRandomBytes(ctx, []types.Value{types.NewInt(16)}),
				}
				for _, result := range results {
					if result.IsError() {
						errors <- result.Error
						return
					}
				}
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start

		ctx := kernel.NewTaskContext()
		ctx.IsWizard = true
		for range iterations {
			result := builtinReseedRandom(ctx, nil)
			if result.IsError() {
				errors <- result.Error
				return
			}
		}
	}()

	close(start)
	wg.Wait()
	close(errors)

	for err := range errors {
		t.Fatalf("concurrent random builtin failed: %v", err)
	}
}
