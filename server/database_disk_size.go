package server

import (
	"fmt"
	"os"
)

// databaseDiskSize reports the latest ordinary checkpoint once this process
// has attempted a dump, falling back to the input database if it is unavailable.
// Before the first dump, a checkpoint left by an earlier process is ignored.
func (s *Server) databaseDiskSize() (int64, error) {
	if s.ordinaryDumpStarted.Load() {
		if info, err := os.Stat(s.dbPath + ".new"); err == nil {
			return info.Size(), nil
		}
	}
	info, err := os.Stat(s.dbPath)
	if err != nil {
		return 0, fmt.Errorf("no database file available: %w", err)
	}
	return info.Size(), nil
}
