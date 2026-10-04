//go:build windows

package format

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// Windows has no Unix directory-fsync equivalent in this package. Write-through
// moves request durable metadata publication in addition to each file's Sync.
func renameCheckpointFile(from, to string) error {
	oldPath, err := checkpointWindowsPath(from)
	if err != nil {
		return err
	}
	newPath, err := checkpointWindowsPath(to)
	if err != nil {
		return err
	}
	if err := windows.MoveFileEx(oldPath, newPath, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH); err != nil {
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: err}
	}
	return nil
}

func checkpointWindowsPath(path string) (*uint16, error) {
	full, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(full, `\\?\`) {
		if strings.HasPrefix(full, `\\`) {
			full = `\\?\UNC\` + full[2:]
		} else {
			full = `\\?\` + full
		}
	}
	return windows.UTF16PtrFromString(full)
}
