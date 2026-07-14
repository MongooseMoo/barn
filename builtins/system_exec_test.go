package builtins

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateAndResolvePathUsesServerExecutablesDirectory(t *testing.T) {
	serverDir := t.TempDir()
	execDir := filepath.Join(serverDir, "executables")
	if err := os.Mkdir(execDir, 0o755); err != nil {
		t.Fatalf("create executables directory: %v", err)
	}
	fixture := filepath.Join(execDir, "sleep")
	if err := os.WriteFile(fixture, nil, 0o755); err != nil {
		t.Fatalf("create executable fixture: %v", err)
	}
	t.Chdir(serverDir)

	got, err := validateAndResolvePath("sleep")
	if err != nil {
		t.Fatalf("resolve executables/sleep: %v", err)
	}
	want := filepath.Join("executables", "sleep")
	if got != want {
		t.Fatalf("resolved path = %q, want %q", got, want)
	}
}
