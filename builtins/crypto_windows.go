//go:build windows
// +build windows

package builtins

import (
	"github.com/sergeymakinen/go-crypt/des"
	crypthash "github.com/sergeymakinen/go-crypt/hash"
)

// cryptDESPlatform implements traditional Unix DES crypt on Windows
// Uses pure Go implementation from github.com/sergeymakinen/go-crypt
func cryptDESPlatform(password, salt string) (string, error) {
	// Get the 8-byte key from the DES algorithm
	key, err := des.Key([]byte(password), []byte(salt))
	if err != nil {
		return "", err
	}

	// Encode the key using the same encoding as the library
	// The sum is 11 characters of the hash
	var sum [11]byte
	crypthash.BigEndianEncoding.Encode(sum[:], key)

	// Return salt (2 chars) + hash (11 chars) = 13 chars total
	return salt + string(sum[:]), nil
}
