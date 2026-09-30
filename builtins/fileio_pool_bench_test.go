package builtins

import (
	"bytes"
	"github.com/MongooseMoo/barn/types"
	"os"
	"path/filepath"
	"testing"
)

func poolReadFixture(t testing.TB, size int, binary bool) (*Execution, *mooFileHandle, types.Value, string) {
	t.Helper()
	pattern := []byte("abc DEF~\x00\n012345")
	data := bytes.Repeat(pattern, size/len(pattern)+1)[:size]
	path := filepath.Join(t.TempDir(), "read.dat")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { file.Close() })
	ctx := newTestExecution()
	ctx.IsWizard = true
	h := &mooFileHandle{id: 1, file: file, mode: "r-tf", binary: binary}
	ctx.Session.runtime.files.handles[1] = h
	want := filterTextMode(data)
	if binary {
		want = encodeBinaryBytes(data)
	}
	return ctx, h, types.NewInt(1), want
}

func benchPoolRead(b *testing.B, size int, binary bool) {
	ctx, h, id, want := poolReadFixture(b, size, binary)
	args := []types.Value{id, types.NewInt(int64(size))}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := h.file.Seek(0, 0); err != nil {
			b.Fatal(err)
		}
		r := builtinFileRead(ctx, args)
		if !r.IsNormal() || r.Val.Str() != want {
			b.Fatal("read result mismatch", r)
		}
	}
}

func BenchmarkPoolFileRead(b *testing.B) {
	for _, tc := range []struct {
		name   string
		size   int
		binary bool
	}{
		{"Text64", 64, false}, {"Text4096", 4096, false}, {"Binary4096", 4096, true}, {"Text65536", 65536, false},
	} {
		b.Run(tc.name, func(b *testing.B) { benchPoolRead(b, tc.size, tc.binary) })
	}
}

// Reserved for the promotion verifier.
func BenchmarkPoolFileReadHoldout(b *testing.B) { benchPoolRead(b, 8192, true) }

func TestFileReadResultOwnershipAndErrors(t *testing.T) {
	for _, binary := range []bool{false, true} {
		ctx, h, id, want := poolReadFixture(t, 4096, binary)
		args := []types.Value{id, types.NewInt(4096)}
		first := builtinFileRead(ctx, args)
		for range 5 {
			if _, err := h.file.Seek(0, 0); err != nil {
				t.Fatal(err)
			}
			if r := builtinFileRead(ctx, args); !r.IsNormal() {
				t.Fatal(r)
			}
		}
		if !first.IsNormal() || first.Val.Str() != want {
			t.Fatal("read string changed after subsequent reads")
		}
		if r := builtinFileRead(ctx, args); r.Error != types.E_FILE {
			t.Fatal("EOF result", r)
		}
		if _, err := h.file.Seek(0, 0); err != nil {
			t.Fatal(err)
		}
		if r := builtinFileRead(ctx, []types.Value{id, types.NewInt(0)}); r.Error != types.E_FILE {
			t.Fatal("empty result", r)
		}
	}
}
