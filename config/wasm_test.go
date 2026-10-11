package config

import (
	"strings"
	"testing"
)

func TestWASMDirectoryOption(t *testing.T) {
	o, err := Parse(strings.NewReader("WASM_DIRECTORY = /srv/barn/builtins\n"), "test.conf")
	if err != nil || o.WASMDirectory != "/srv/barn/builtins" {
		t.Fatalf("options: %+v, %v", o, err)
	}
}
