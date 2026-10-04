package format

import (
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

type checkpointPathLock struct {
	mu    sync.Mutex
	users int
}

var checkpointPathLocks = struct {
	sync.Mutex
	paths map[string]*checkpointPathLock
}{paths: make(map[string]*checkpointPathLock)}

// Entries exist only while a writer, loader, or recovery operation uses a path.
// Distinct ordinary and panic checkpoint paths retain independent ownership.
func lockCheckpointPath(path string) (func(), error) {
	key, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if runtime.GOOS == "windows" {
		key = strings.ToLower(key)
	}
	checkpointPathLocks.Lock()
	entry := checkpointPathLocks.paths[key]
	if entry == nil {
		entry = &checkpointPathLock{}
		checkpointPathLocks.paths[key] = entry
	}
	entry.users++
	checkpointPathLocks.Unlock()
	entry.mu.Lock()
	return func() {
		entry.mu.Unlock()
		checkpointPathLocks.Lock()
		entry.users--
		if entry.users == 0 {
			delete(checkpointPathLocks.paths, key)
		}
		checkpointPathLocks.Unlock()
	}, nil
}
