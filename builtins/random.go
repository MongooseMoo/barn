package builtins

import (
	cryptorand "crypto/rand"
	"io"
	rand "math/rand/v2"
	"sync"
)

type randomGenerator struct {
	mu      sync.Mutex
	source  *rand.ChaCha8
	random  *rand.Rand
	entropy io.Reader
}

var sharedRandom = newRandomGenerator(mustRandomSeed(cryptorand.Reader), cryptorand.Reader)

func newRandomGenerator(seed [32]byte, entropy io.Reader) *randomGenerator {
	source := rand.NewChaCha8(seed)
	return &randomGenerator{
		source:  source,
		random:  rand.New(source),
		entropy: entropy,
	}
}

func mustRandomSeed(entropy io.Reader) [32]byte {
	seed, err := randomSeed(entropy)
	if err != nil {
		panic(err)
	}
	return seed
}

func randomSeed(entropy io.Reader) ([32]byte, error) {
	var seed [32]byte
	_, err := io.ReadFull(entropy, seed[:])
	return seed, err
}

func (r *randomGenerator) reseed() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	seed, err := randomSeed(r.entropy)
	if err != nil {
		return err
	}
	r.source.Seed(seed)
	return nil
}

func (r *randomGenerator) int64N(n int64) int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.random.Int64N(n)
}

func (r *randomGenerator) uint64N(n uint64) uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.random.Uint64N(n)
}

func (r *randomGenerator) uint64() uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.random.Uint64()
}
