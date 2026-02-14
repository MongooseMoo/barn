package main

import (
	"fmt"

	"github.com/digitive/crypt"
)

func cryptDES(password, salt string) (string, error) {
	return crypt.Crypt(password, salt)
}

func main() {
	// Test with explicit 2-char salt extraction
	testPw := "testpass"
	result, _ := cryptDES(testPw, "AB")
	fmt.Printf("New hash for '%s': %s\n", testPw, result)

	// Verify using first 2 chars of result as salt
	verify, _ := cryptDES(testPw, result[:2])
	fmt.Printf("Verification: %s\n", verify)
	fmt.Printf("Match: %v\n", result == verify)

	// Now test stored mongoose password
	stored := "FD8gBC5iN39ps"
	salt := stored[:2]
	fmt.Printf("\n--- Testing mongoose wizard hash ---\n")
	fmt.Printf("Stored: %s, Salt: %s\n", stored, salt)

	// Test some common passwords
	for _, pw := range []string{"potrzebie", "potrzeb", "wizard", "mongoose"} {
		h, _ := cryptDES(pw, salt)
		match := ""
		if h == stored {
			match = " <-- MATCH"
		}
		fmt.Printf("crypt('%s', '%s') = %s%s\n", pw, salt, h, match)
	}
}
