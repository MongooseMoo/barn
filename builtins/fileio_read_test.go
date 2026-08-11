package builtins

import (
	"math"
	"os"
	"testing"

	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/types"
)

func TestFileReadCapsAllocationAtBytesRemaining(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.Mkdir("files", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("files/small.txt", []byte("small file"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := kernel.NewTaskContext()
	ctx.IsWizard = true
	opened := builtinFileOpen(ctx, []types.Value{types.NewStr("small.txt"), types.NewStr("r-tf")})
	if opened.IsError() {
		t.Fatalf("file_open returned %s", opened.Error)
	}
	t.Cleanup(func() { builtinFileClose(ctx, []types.Value{opened.Val}) })

	result := builtinFileRead(ctx, []types.Value{opened.Val, types.NewInt(math.MaxInt64)})
	if result.IsError() {
		t.Fatalf("file_read returned %s", result.Error)
	}
	if got, want := result.Val.Str(), "small file"; got != want {
		t.Fatalf("file_read returned %q, want %q", got, want)
	}
}

func TestFileReadRejectsResultAboveStringLimit(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.Mkdir("files", 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create("files/large.txt")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(1024); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	ctx := kernel.NewTaskContext()
	ctx.IsWizard = true
	ctx.MaxStringConcat = 100
	opened := builtinFileOpen(ctx, []types.Value{types.NewStr("large.txt"), types.NewStr("r-tf")})
	if opened.IsError() {
		t.Fatalf("file_open returned %s", opened.Error)
	}
	t.Cleanup(func() { builtinFileClose(ctx, []types.Value{opened.Val}) })

	result := builtinFileRead(ctx, []types.Value{opened.Val, types.NewInt(math.MaxInt64)})
	if !result.IsError() || result.Error != types.E_QUOTA {
		t.Fatalf("file_read = flow %v, error %s; want E_QUOTA", result.Flow, result.Error)
	}
}
