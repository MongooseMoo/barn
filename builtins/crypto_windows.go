//go:build windows
// +build windows

package builtins

import (
	"github.com/digitive/crypt"
)

// cryptDESPlatform implements traditional Unix DES crypt on Windows
// Uses pure Go implementation from github.com/digitive/crypt
func cryptDESPlatform(password, salt string) (string, error) {
	// Extract 2-char salt from potentially longer input (stored hash)
	if len(salt) > 2 {
		salt = salt[:2]
	}
	return crypt.Crypt(password, salt)
}
