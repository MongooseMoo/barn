package wasmbuiltins

import (
	"bytes"
	"encoding/binary"
)

// Wazy's one-instance fast path bypasses CompileCache and registers the core
// module's custom name. An inert standard inline core instance selects its
// cached, anonymous graph path. Existing indices, imports and exports are
// unchanged. Remove this workaround when both paths honor CompileCache.
// The caller must first validate the section envelopes with singleCore.
func withGraphInstantiation(data []byte) []byte {
	instances := uint64(0)
	imports := uint64(0)
	for pos := 8; pos < len(data); {
		id := data[pos]
		pos++
		n, size := binary.Uvarint(data[pos:])
		pos += size
		if id == 2 {
			count, _ := binary.Uvarint(data[pos : pos+int(n)])
			instances += count
		}
		if id == 10 {
			count, _ := binary.Uvarint(data[pos : pos+int(n)])
			imports += count
		}
		pos += int(n)
	}
	if instances == 1 && imports == 0 {
		// A passthrough of the existing core function zero has no observable
		// export at component level. Wazy rejects empty inline instances.
		return append(bytes.Clone(data), 2, 10, 1, 1, 1, 4, 'b', 'a', 'r', 'n', 0, 0)
	}
	return data
}
