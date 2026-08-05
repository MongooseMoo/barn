package builtins

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"

	cryptbcrypt "github.com/go-crypt/x/bcrypt"
)

const bcrypt2xPrefix = "$2x$"

// cryptBcrypt2x implements Openwall's $2x$ compatibility variant. The variant
// deliberately reproduces the historical signed-char key schedule bug; it is
// not corrected bcrypt with a different prefix.
//
// Provenance: Openwall crypt_blowfish, tag CRYPT_BLOWFISH_1_3, commit
// 3354bb81eea489e972b0a7c63231514ab34f73a0, crypt_blowfish.c BF_set_key().
func cryptBcrypt2x(password, setting string) (string, error) {
	if len(setting) < 7 || !strings.HasPrefix(setting, bcrypt2xPrefix) {
		return "", fmt.Errorf("invalid bcrypt 2x setting")
	}
	if setting[4] < '0' || setting[4] > '9' ||
		setting[5] < '0' || setting[5] > '9' || setting[6] != '$' {
		return "", fmt.Errorf("invalid bcrypt 2x cost field")
	}
	cost, err := strconv.Atoi(setting[4:6])
	if err != nil || cost < 4 || cost > 31 {
		return "", fmt.Errorf("invalid bcrypt 2x cost")
	}

	saltPortion := setting[7:]
	var saltEncoded string
	switch {
	case len(saltPortion) == 16:
		saltEncoded = bcryptBase64Encode([]byte(saltPortion))
	case len(saltPortion) >= cryptbcrypt.EncodedSaltSize:
		saltEncoded = saltPortion[:cryptbcrypt.EncodedSaltSize]
	default:
		return "", fmt.Errorf("invalid bcrypt 2x salt length")
	}

	rawSalt, err := cryptbcrypt.Base64Decode([]byte(saltEncoded))
	if err != nil || len(rawSalt) != 16 {
		return "", fmt.Errorf("invalid bcrypt 2x salt")
	}

	// The existing bcrypt implementation reads 18 cyclic big-endian words
	// during each password-key expansion. Supplying these exact 72 bytes makes
	// every expansion consume the Openwall $2x$ signed-char words.
	schedule := bcrypt2xSchedule([]byte(password))
	hash, err := cryptbcrypt.GenerateFromPasswordSalt(schedule, rawSalt, cost)
	if err != nil {
		return "", err
	}
	if len(hash) != 60 || !bytes.HasPrefix(hash, []byte("$2a$")) {
		return "", fmt.Errorf("invalid bcrypt output shape")
	}

	hash[2] = 'x'
	return string(hash), nil
}

// bcrypt2xSchedule synthesizes the 18 big-endian words produced by Openwall's
// buggy signed-char BF_set_key path. Openwall consumes a C string and bcrypt's
// key schedule consumes at most 72 bytes, so an embedded NUL terminates the
// password and bytes after the first 72 never participate.
func bcrypt2xSchedule(password []byte) []byte {
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
