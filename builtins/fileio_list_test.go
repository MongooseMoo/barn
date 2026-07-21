package builtins

import (
	"testing"

	"barn/kernel"
	"barn/types"
)

func TestFileListReturnsEmptyListWhenSandboxDirectoryIsMissing(t *testing.T) {
	t.Chdir(t.TempDir())
	ctx := kernel.NewTaskContext()
	ctx.IsWizard = true

	result := builtinFileList(ctx, []types.Value{types.NewStr("."), types.NewFloat(1.5)})
	if result.IsError() {
		t.Fatalf("file_list on missing sandbox returned %s, want empty list", result.Error)
	}
	if result.Val.Type() != types.TYPE_LIST || result.Val.Len() != 0 {
		t.Fatalf("file_list on missing sandbox = %s, want {}", result.Val.String())
	}
}
