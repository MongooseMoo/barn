package format

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func sidecarRecordsFixture(t testing.TB, count int) ([]types.WaifIdentity, [sha256.Size]byte, []byte) {
	t.Helper()
	identities := make([]types.WaifIdentity, count)
	digest := sha256.Sum256([]byte("synthetic checkpoint"))
	var expected strings.Builder
	fmt.Fprintf(&expected, "barn-waif-identities-v1 %x\n", digest)
	for i := range identities {
		encoded := fmt.Sprintf("%032x", i+1)
		identity, err := types.ParseWaifIdentity(encoded)
		if err != nil {
			t.Fatal(err)
		}
		identities[i] = identity
		expected.WriteString(encoded)
		expected.WriteByte('\n')
	}
	return identities, digest, []byte(expected.String())
}

func TestWaifIdentitySidecarLargeRecordList(t *testing.T) {
	identities, digest, expected := sidecarRecordsFixture(t, 48000)
	path := filepath.Join(t.TempDir(), "large.waifids")
	if err := writeWaifIdentitySidecarFile(path, digest, identities); err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(actual, expected) {
		t.Fatalf("large sidecar bytes differ: got %d bytes, want %d, read error %v", len(actual), len(expected), err)
	}
	parsed, err := parseWaifIdentitySidecar(bytes.NewReader(actual), digest)
	if err != nil || len(parsed) != len(identities) {
		t.Fatalf("large sidecar round-trip count=%d, error=%v", len(parsed), err)
	}
	for i := range parsed {
		if parsed[i] != identities[i] {
			t.Fatalf("identity order changed at index %d", i)
		}
	}
}

// A small file fits inside a buffer. ENOSPC must still be surfaced before Sync
// or Close; this catches a missing Flush and a discarded deferred-write error.
func TestWaifIdentitySidecarDeferredWriteFailure(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("requires Linux /dev/full")
	}
	if _, err := os.Stat("/dev/full"); err != nil {
		t.Skip("/dev/full unavailable")
	}
	identities, digest, _ := sidecarRecordsFixture(t, 1)
	err := writeWaifIdentitySidecarFile("/dev/full", digest, identities)
	if !errors.Is(err, syscall.ENOSPC) || !strings.Contains(err.Error(), "WAIF identity sidecar") {
		t.Fatalf("write failure did not preserve ENOSPC and sidecar context: %v", err)
	}
}

// BARN_SIDECAR_BENCH_DIR selects an explicitly owned, existing empty directory.
// The driver runs the same test binary once on each filesystem with benchtime1x.
// Fixture setup and byte validation are outside the measured operation.
func BenchmarkWaifIdentitySidecarRecords(b *testing.B) {
	directory := os.Getenv("BARN_SIDECAR_BENCH_DIR")
	if directory == "" {
		directory = b.TempDir()
	}
	path := filepath.Join(directory, "records.waifids")
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		b.Fatal("benchmark output is not fresh")
	}
	identities, digest, expected := sidecarRecordsFixture(b, 48000)
	b.SetBytes(int64(len(expected)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := writeWaifIdentitySidecarFile(path, digest, identities); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	actual, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(actual, expected) {
		b.Fatalf("benchmark output changed: bytes=%d expected=%d error=%v", len(actual), len(expected), err)
	}
	checksum := sha256.Sum256(actual)
	b.Logf("records=48000 bytes=%d sha256=%s", len(actual), hex.EncodeToString(checksum[:]))
}
