// Package sourcekey computes the content identity of one MOO verb body.
//
// It is a leaf package on purpose: both the compiler (which keys its program
// cache by source content) and db/store (which carries the key alongside a
// verb's source so the hot call path never rehashes) must produce the SAME key
// for the same lines. Two sources that differ in any byte, in any line, or in
// line count must never share a key — a collision would serve a cached program
// compiled from other source.
package sourcekey

import (
	"crypto/sha256"
	"encoding/binary"
)

// Key identifies source content. The zero value means "not computed"; it is
// never produced by Of, so a caller holding a Key can always tell a real key
// from an absent one and fall back to hashing. Key is comparable and is used
// directly as a map key.
type Key struct {
	hash [sha256.Size]byte
	set  bool
}

// IsSet reports whether this key was computed from actual source. An unset key
// carries no information about any source and must never be used for a lookup.
func (k Key) IsSet() bool { return k.set }

// Of returns the key for the given source lines. Line lengths are hashed along
// with the bytes so that no regrouping of the same bytes across lines (["ab"]
// vs ["a","b"]) can collide.
func Of(lines []string) Key {
	hash := sha256.New()
	var length [8]byte
	for _, line := range lines {
		binary.LittleEndian.PutUint64(length[:], uint64(len(line)))
		hash.Write(length[:])
		hash.Write([]byte(line))
	}
	key := Key{set: true}
	copy(key.hash[:], hash.Sum(nil))
	return key
}
