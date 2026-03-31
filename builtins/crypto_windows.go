//go:build windows
// +build windows

package builtins

import "fmt"

// cryptDESPlatform implements traditional Unix DES crypt on Windows
// ToastStunt compatibility: DES crypt is not supported on Windows.
func cryptDESPlatform(password, salt string) (string, error) {
	return "", fmt.Errorf("DES crypt is unavailable on Windows")
}
