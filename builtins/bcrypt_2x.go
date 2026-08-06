package builtins

import (
	"bytes"
	"encoding/binary"
)

// bcrypt2xSchedule synthesizes the 18 big-endian words produced by the
// historical signed-char bcrypt key setup bug. Feeding these 72 bytes to the
// corrected backend makes each password-key expansion consume those exact
// words while leaving its salt expansion and block encryption unchanged.
func bcrypt2xSchedule(password []byte) []byte {
	// The historical implementation consumes a C string, and bcrypt uses at
	// most 72 password bytes.
	if nul := bytes.IndexByte(password, 0); nul >= 0 {
		password = password[:nul]
	}
	if len(password) > 72 {
		password = password[:72]
	}

	key := make([]byte, len(password)+1)
	copy(key, password)

	schedule := make([]byte, 18*4)
	pos := 0
	for wordIndex := 0; wordIndex < 18; wordIndex++ {
		var word uint32
		for byteIndex := 0; byteIndex < 4; byteIndex++ {
			b := key[pos]
			word <<= 8
			word |= uint32(int32(int8(b)))
			if b == 0 {
				pos = 0
			} else {
				pos++
			}
		}
		binary.BigEndian.PutUint32(schedule[wordIndex*4:], word)
	}
	return schedule
}
