package server

import (
	"context"
	"sync"
)

const maxPendingTransportInputs = 16

// transportInputBuffer applies backpressure only to this connection's reader.
// The decoder may hold one additional input while waiting. Forced input never
// waits on this budget, so a running task cannot block its own dispatcher.
type transportInputBuffer struct {
	mu           sync.Mutex
	count, bytes int
	changed      chan struct{}
}

func newTransportInputBuffer() *transportInputBuffer {
	return &transportInputBuffer{changed: make(chan struct{}, 1)}
}

func (b *transportInputBuffer) acquire(ctx, connection context.Context, size int) bool {
	for {
		if ctx.Err() != nil || connection.Err() != nil {
			return false
		}
		b.mu.Lock()
		// An already-decoded oversized transport message may enter alone. This
		// budget must not introduce a new protocol framing/line-size policy.
		if b.count < maxPendingTransportInputs && (b.bytes == 0 || size <= maxLineBytes-b.bytes) {
			b.count++
			b.bytes += size
			b.mu.Unlock()
			return true
		}
		b.mu.Unlock()
		select {
		case <-ctx.Done():
			return false
		case <-connection.Done():
			return false
		case <-b.changed:
		}
	}
}

func (b *transportInputBuffer) release(size int) {
	b.mu.Lock()
	b.count--
	b.bytes -= size
	b.mu.Unlock()
	select {
	case b.changed <- struct{}{}:
	default:
	}
}
