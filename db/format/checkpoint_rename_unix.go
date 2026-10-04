//go:build !windows

package format

import "os"

func renameCheckpointFile(from, to string) error { return os.Rename(from, to) }
