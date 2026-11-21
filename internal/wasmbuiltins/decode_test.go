package wasmbuiltins

import (
	"context"
	"encoding/binary"
	"io"
	"strings"
	"testing"
)

func wasmSection(id byte, data []byte) []byte {
	out := []byte{id}
	out = binary.AppendUvarint(out, uint64(len(data)))
	return append(out, data...)
}
func coreDeclarationComponent(section []byte) []byte {
	core := append([]byte{0, 97, 115, 109, 1, 0, 0, 0}, section...)
	return append([]byte{0, 97, 115, 109, 13, 0, 1, 0}, wasmSection(1, core)...)
}
func TestCoreAllocationBombsRejectedBeforeRuntime(t *testing.T) {
	// A short binary can declare billions of locals. The runtime expands the
	// declaration into a Go slice, so validation must precede its decoder.
	body := []byte{1}
	body = binary.AppendUvarint(body, 0xffffffff)
	body = append(body, 0x7f, 0x0b)
	code := []byte{1}
	code = binary.AppendUvarint(code, uint64(len(body)))
	code = append(code, body...)
	for _, section := range [][]byte{
		wasmSection(10, code),
		wasmSection(3, binary.AppendUvarint(nil, 0xffffffff)),
		wasmSection(1, []byte{1, 0x60, 0xff, 0xff, 0xff, 0xff, 0x0f}),
	} {
		data := coreDeclarationComponent(section)
		if _, err := prepare(context.Background(), data); err == nil || !strings.Contains(err.Error(), "core") {
			t.Fatalf("core allocation bomb reached runtime: %v", err)
		}
	}
}

func TestParserOutputBoundIncludesCopyFastPaths(t *testing.T) {
	b := &boundedBuffer{limit: 4}
	if _, err := io.Copy(b, strings.NewReader("too much output")); err == nil || len(b.Bytes()) > 4 {
		t.Fatalf("unbounded parser output: %d, %v", len(b.Bytes()), err)
	}
	if _, err := io.Copy(b, strings.NewReader("okay")); err != nil || b.String() != "okay" {
		t.Fatalf("bounded output: %q, %v", b.String(), err)
	}
}
