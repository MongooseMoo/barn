package format

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func sidecarHashFixture(t testing.TB, size int) (string, []types.WaifIdentity, string) {
	t.Helper()
	payload := make([]byte, size)
	for i := range payload {
		payload[i] = byte(i*31 + 7)
	}
	path := filepath.Join(t.TempDir(), "hash.db")
	if err := os.WriteFile(path, payload, 0600); err != nil {
		t.Fatal(err)
	}
	identity, err := types.ParseWaifIdentity("00112233445566778899aabbccddeeff")
	if err != nil {
		t.Fatal(err)
	}
	identities := []types.WaifIdentity{identity}
	header := fmt.Sprintf("barn-waif-identities-v1 %x\n%s\n", sha256.Sum256(payload), identities[0])
	return path, identities, header
}

func TestWaifIdentitySidecarPreservesExactHashBytes(t *testing.T) {
	for _, size := range []int{0, 17, 8 << 20} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			path, identities, expected := sidecarHashFixture(t, size)
			if err := writeWaifIdentitySidecar(path+waifIdentitySidecarSuffix, path, identities); err != nil {
				t.Fatal(err)
			}
			actual, err := os.ReadFile(path + waifIdentitySidecarSuffix)
			if err != nil || string(actual) != expected {
				t.Fatalf("sidecar bytes = %q, error = %v, want %q", actual, err, expected)
			}
			got, err := readWaifIdentitySidecar(path)
			if err != nil || len(got) != 1 || got[0] != identities[0] {
				t.Fatalf("identities = %v, error = %v, want %v", got, err, identities)
			}
			if err := os.WriteFile(path, []byte("changed"), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := readWaifIdentitySidecar(path); err == nil || !strings.Contains(err.Error(), "does not match database") {
				t.Fatalf("mismatching database error = %v", err)
			}
		})
	}
}

func TestWaifIdentitySidecarHashReadFailures(t *testing.T) {
	for _, kind := range []string{"missing", "directory"} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "database")
			if kind == "directory" {
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(path+waifIdentitySidecarSuffix, []byte("header\n"), 0600); err != nil {
				t.Fatal(err)
			}
			_, readErr := readWaifIdentitySidecar(path)
			writeErr := writeWaifIdentitySidecar(path+".output", path, nil)
			for _, err := range []error{readErr, writeErr} {
				if err == nil || !strings.Contains(err.Error(), "hash database for WAIF identity sidecar") {
					t.Fatalf("hash failure context = %v", err)
				}
				if kind == "missing" && !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("missing file error = %v", err)
				}
			}
			if _, err := os.Stat(path + ".output"); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("failed hash created sidecar output: %v", err)
			}
		})
	}
}

func TestWaifIdentitySidecarHashMemoryIsBounded(t *testing.T) {
	for _, operation := range []string{"Read", "Write"} {
		t.Run(operation, func(t *testing.T) {
			path, identities, header := sidecarHashFixture(t, 8<<20)
			if err := os.WriteFile(path+waifIdentitySidecarSuffix, []byte(header), 0600); err != nil {
				t.Fatal(err)
			}
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			for i := 0; i < 4; i++ {
				var err error
				if operation == "Read" {
					_, err = readWaifIdentitySidecar(path)
				} else {
					err = writeWaifIdentitySidecar(path+waifIdentitySidecarSuffix, path, identities)
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			runtime.ReadMemStats(&after)
			// A loose budget admits streaming buffers and incidental runtime work
			// but rejects an 8 MiB temporary on every hash.
			if bytes := (after.TotalAlloc - before.TotalAlloc) / 4; bytes > 512<<10 {
				t.Fatalf("sidecar hash allocated %d bytes/op, want <= 512 KiB", bytes)
			}
		})
	}
}

var sidecarHashIdentitySink []types.WaifIdentity

func BenchmarkWaifIdentitySidecarHash(b *testing.B) {
	for _, size := range []int{1 << 20, 8 << 20, 32 << 20} {
		for _, operation := range []string{"Read", "Write"} {
			b.Run(fmt.Sprintf("%s/%dMiB", operation, size>>20), func(b *testing.B) {
				path, identities, expected := sidecarHashFixture(b, size)
				if err := os.WriteFile(path+waifIdentitySidecarSuffix, []byte(expected), 0600); err != nil {
					b.Fatal(err)
				}
				b.SetBytes(int64(size))
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if operation == "Read" {
						var err error
						sidecarHashIdentitySink, err = readWaifIdentitySidecar(path)
						if err != nil {
							b.Fatal(err)
						}
					} else if err := writeWaifIdentitySidecar(path+waifIdentitySidecarSuffix, path, identities); err != nil {
						b.Fatal(err)
					}
				}
				b.StopTimer()
				actual, err := os.ReadFile(path + waifIdentitySidecarSuffix)
				if err != nil || string(actual) != expected {
					b.Fatal("sidecar workload changed")
				}
				if operation == "Read" && (len(sidecarHashIdentitySink) != 1 || sidecarHashIdentitySink[0] != identities[0]) {
					b.Fatal("identity workload changed")
				}
			})
		}
	}
}
