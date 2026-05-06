package builtins

import (
	"testing"
)

func TestCryptDES(t *testing.T) {
	// Both Unix (via libcrypt) and Windows (via github.com/digitive/crypt)
	// should produce the same traditional DES crypt(3) hash for a given
	// password and salt. Test against a known good value from ToastStunt.
	result, err := cryptDES("foobar", "SA")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	expected := "SAEmC5UwrAl2A"
	if result != expected {
		t.Errorf("crypt('foobar', 'SA') = %q, expected %q", result, expected)
	}
}
