package builtins

import (
	"bytes"
	cryptorand "crypto/rand"
	"errors"
	rand "math/rand/v2"
	"sync"
	"testing"

	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/types"
)

type failingEntropyReader struct {
	err error
}

func (r failingEntropyReader) Read(_ []byte) (int, error) {
	return 0, r.err
}

func randomTestSeed(offset byte) [32]byte {
	var seed [32]byte
	for i := range seed {
		seed[i] = byte(i) + offset
	}
	return seed
}

func installSharedRandomForTest(t *testing.T, generator *randomGenerator) {
	t.Helper()
	original := sharedRandom
	sharedRandom = generator
	t.Cleanup(func() {
		sharedRandom = original
	})
}

func requireRandomInt(t *testing.T, ctx *kernel.TaskContext, max int64) int64 {
	t.Helper()
	result := builtinRandom(ctx, []types.Value{types.NewInt(max)})
	if result.IsError() {
		t.Fatalf("random(%d) failed: %v", max, result.Error)
	}
	return result.Val.Int()
}

func TestReseedRandomReturnsZeroAndReseedsIntegerStream(t *testing.T) {
	initialSeed := randomTestSeed(0)
	reseededSeed := randomTestSeed(32)
	installSharedRandomForTest(t, newRandomGenerator(initialSeed, bytes.NewReader(reseededSeed[:])))

	ctx := kernel.NewTaskContext()
	ctx.IsWizard = true
	result := builtinReseedRandom(ctx, nil)
	if result.IsError() {
		t.Fatalf("reseed_random() failed: %v", result.Error)
	}
	if result.Val.Type() != types.TYPE_INT || result.Val.Int() != 0 {
		t.Fatalf("reseed_random() = %#v, want integer 0", result.Val)
	}

	reference := rand.New(rand.NewChaCha8(reseededSeed))
	for i := 0; i < 3; i++ {
		if got, want := requireRandomInt(t, ctx, 1_000_000), reference.Int64N(1_000_000)+1; got != want {
			t.Fatalf("random() draw %d = %d, want %d from reseeded integer stream", i, got, want)
		}
	}
}

func TestReseedRandomValidatesArityAndAuthority(t *testing.T) {
	wizard := kernel.NewTaskContext()
	wizard.IsWizard = true
	if result := builtinReseedRandom(wizard, []types.Value{types.NewInt(1)}); !result.IsError() || result.Error != types.E_ARGS {
		t.Fatalf("reseed_random(1) = %#v, want E_ARGS", result)
	}

	nonWizard := kernel.NewTaskContext()
	if result := builtinReseedRandom(nonWizard, nil); !result.IsError() || result.Error != types.E_PERM {
		t.Fatalf("non-wizard reseed_random() = %#v, want E_PERM", result)
	}
}

func TestReseedRandomEntropyFailurePreservesIntegerState(t *testing.T) {
	seed := randomTestSeed(7)
	entropyErr := errors.New("entropy unavailable")
	installSharedRandomForTest(t, newRandomGenerator(seed, failingEntropyReader{err: entropyErr}))

	ctx := kernel.NewTaskContext()
	ctx.IsWizard = true
	reference := rand.New(rand.NewChaCha8(seed))

	if got, want := requireRandomInt(t, ctx, 1_000_000), reference.Int64N(1_000_000)+1; got != want {
		t.Fatalf("random() before failed reseed = %d, want %d", got, want)
	}
	if result := builtinReseedRandom(ctx, nil); !result.IsError() || result.Error != types.E_INVARG {
		t.Fatalf("reseed_random() with failed entropy = %#v, want E_INVARG", result)
	}
	if got, want := requireRandomInt(t, ctx, 1_000_000), reference.Int64N(1_000_000)+1; got != want {
		t.Fatalf("random() after failed reseed = %d, want unchanged-stream draw %d", got, want)
	}
}

func TestReseedRandomLeavesOtherRandomStreamsIndependent(t *testing.T) {
	initialSeed := randomTestSeed(0)
	reseededSeed := randomTestSeed(64)
	installSharedRandomForTest(t, newRandomGenerator(initialSeed, bytes.NewReader(reseededSeed[:])))

	cryptoBytes := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	originalCryptoReader := cryptorand.Reader
	cryptorand.Reader = bytes.NewReader(cryptoBytes)
	t.Cleanup(func() {
		cryptorand.Reader = originalCryptoReader
	})

	ctx := kernel.NewTaskContext()
	ctx.IsWizard = true
	if result := builtinReseedRandom(ctx, nil); result.IsError() {
		t.Fatalf("reseed_random() failed: %v", result.Error)
	}

	reference := rand.New(rand.NewChaCha8(reseededSeed))
	if got, want := requireRandomInt(t, ctx, 1_000_000), reference.Int64N(1_000_000)+1; got != want {
		t.Fatalf("first random() = %d, want %d", got, want)
	}

	floating := builtinFrandom(ctx, []types.Value{types.NewFloat(1)})
	if floating.IsError() {
		t.Fatalf("frandom() failed: %v", floating.Error)
	}
	if got := floating.Val.Float(); got < 0 || got >= 1 {
		t.Fatalf("frandom(1.0) = %v, want [0, 1)", got)
	}

	randomBytes := builtinRandomBytes(ctx, []types.Value{types.NewInt(int64(len(cryptoBytes)))})
	if randomBytes.IsError() {
		t.Fatalf("random_bytes() failed: %v", randomBytes.Error)
	}
	gotBytes, invalid := decodeBinaryString(randomBytes.Val.Str())
	if invalid {
		t.Fatalf("random_bytes() returned invalid binary string %q", randomBytes.Val.Str())
	}
	if !bytes.Equal(gotBytes, cryptoBytes) {
		t.Fatalf("random_bytes() = %x, want crypto/rand bytes %x", gotBytes, cryptoBytes)
	}

	if got, want := requireRandomInt(t, ctx, 1_000_000), reference.Int64N(1_000_000)+1; got != want {
		t.Fatalf("random() after frandom/random_bytes = %d, want independent-stream draw %d", got, want)
	}
}

func TestRandomAndReseedRandomConcurrentUse(t *testing.T) {
	installSharedRandomForTest(t, newRandomGenerator(randomTestSeed(0), cryptorand.Reader))

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
				if result := builtinRandom(ctx, []types.Value{types.NewInt(1_000_000)}); result.IsError() {
					errors <- result.Error
					return
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
			if result := builtinReseedRandom(ctx, nil); result.IsError() {
				errors <- result.Error
				return
			}
		}
	}()

	close(start)
	wg.Wait()
	close(errors)

	for errCode := range errors {
		t.Fatalf("concurrent random builtin failed: %v", errCode)
	}
}
