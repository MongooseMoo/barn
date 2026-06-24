//go:build !windows && !cgo
// +build !windows,!cgo

package builtins

import "github.com/digitive/crypt"

// cryptDESPlatform implements traditional Unix DES crypt for non-cgo builds.
func cryptDESPlatform(password, salt string) (string, error) {
	if len(salt) > 2 {
		salt = salt[:2]
	}
	return crypt.Crypt(password, salt)
}
