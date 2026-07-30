package builtins

import (
	cryptorand "crypto/rand"
	rand "math/rand/v2"
	"sync"
)

type randomGenerator struct {
	mu     sync.Mutex
	source *rand.ChaCha8
	random *rand.Rand
}

var sharedRandom = newRandomGenerator(mustRandomSeed())

func newRandomGenerator(seed [32]byte) *randomGenerator {
	source := rand.NewChaCha8(seed)
	return &randomGenerator{
		source: source,
		random: rand.New(source),
	}
}

func mustRandomSeed() [32]byte {
	seed, err := randomSeed()
	if err != nil {
		panic(err)
	}
	return seed
}

func randomSeed() ([32]byte, error) {
	var seed [32]byte
	_, err := cryptorand.Read(seed[:])
	return seed, err
}

func (r *randomGenerator) reseed() error {
	seed, err := randomSeed()
	if err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.source.Seed(seed)
	return nil
}

func (r *randomGenerator) int64N(n int64) int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.random.Int64N(n)
}

func (r *randomGenerator) float64() float64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.random.Float64()
}

func (r *randomGenerator) read(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.source.Read(p)
}
