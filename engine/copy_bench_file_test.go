package engine

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestCopyBenchFile(t *testing.T) {
	content := []byte("sound catalog bytes")

	t.Run("copies to new path", func(t *testing.T) {
		dir := t.TempDir()
		src := filepath.Join(dir, "src.sqlite")
		dst := filepath.Join(dir, "files", "sqlite", "sound.sqlite")
		if err := os.WriteFile(src, content, 0o644); err != nil {
			t.Fatal(err)
		}
		copied, err := copyBenchFile(src, dst)
		if err != nil {
			t.Fatal(err)
		}
		if !copied {
			t.Fatal("copied = false, want true")
		}
		got, err := os.ReadFile(dst)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, content) {
			t.Fatalf("dst = %q, want %q", got, content)
		}
	})

	t.Run("same file through two spellings is left intact", func(t *testing.T) {
		dir := t.TempDir()
		abs := filepath.Join(dir, "files", "sqlite", "sound.sqlite")
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, content, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Chdir(dir)
		copied, err := copyBenchFile(abs, "files/sqlite/sound.sqlite")
		if err != nil {
			t.Fatal(err)
		}
		if copied {
			t.Fatal("copied = true, want false")
		}
		got, err := os.ReadFile(abs)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, content) {
			t.Fatalf("file = %q, want unchanged %q", got, content)
		}
	})
}
