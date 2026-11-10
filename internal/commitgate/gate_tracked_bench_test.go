package commitgate

import (
	"context"
	"testing"
)

// BenchmarkExclusiveBehindWaitersTracked is BenchmarkExclusiveBehindWaiters
// through AcquireTracked: the cost tracking adds.
func BenchmarkExclusiveBehindWaitersTracked(b *testing.B) {
	g, owner := gateBehindWaiters()
	defer owner.Release()
	label := "#2:say by retry budget spent"
	b.ReportAllocs()
	for range b.N {
		ctx := &cancelAfterEnqueue{Context: context.Background()}
		if _, err := g.AcquireTracked(ctx, Exclusive, label); err == nil {
			b.Fatal("granted behind a shared owner")
		}
	}
}
