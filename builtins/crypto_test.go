package builtins

import (
	"runtime"
	"testing"
)

func TestCryptDES(t *testing.T) {
	if runtime.GOOS == "windows" {
		// On Windows, DES crypt should fail with error
		_, err := cryptDES("foobar", "SA")
		if err == nil {
			t.Fatal("Expected error on Windows, got nil")
		}
		t.Log("Windows correctly rejects DES crypt:", err)
		return
	}

	// On Unix, test against known good value from ToastStunt
	result, err := cryptDES("foobar", "SA")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	expected := "SAEmC5UwrAl2A"
	if result != expected {
		t.Errorf("crypt('foobar', 'SA') = %q, expected %q", result, expected)
	}
}
