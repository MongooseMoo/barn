package store

import (
	"sync/atomic"
	"testing"
)

// Concurrent transactions registering and releasing their read timestamps,
// spread across the live-readTS shards. Each goroutine starts on its own shard
// and steps one timestamp per transaction, as tasks beginning at a clock other
// commits keep advancing do. Run with -cpu to vary the worker count.
func BenchmarkReadTSRegistryParallel(b *testing.B) {
	b.Run("BeginRelease", func(b *testing.B) {
		s := NewStore()
		var workers atomic.Uint64
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			g := workers.Add(1)
			ts := g<<32 | g
			for pb.Next() {
				s.BeginSnapshot(ts).Release()
				ts++
			}
		})
	})
	b.Run("RegisterDeregister", func(b *testing.B) {
		s := NewStore()
		var workers atomic.Uint64
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			g := workers.Add(1)
			ts := g<<32 | g
			for pb.Next() {
				s.registerReadTS(ts)
				s.deregisterReadTS(ts)
				ts++
			}
		})
	})
}
