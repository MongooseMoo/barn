package server

import (
	"fmt"
	"os"
)

// databaseDiskSize reports the storage occupied by the server's loaded input
// database and its latest ordinary checkpoint. Both remain active restart
// candidates until an operator adopts the checkpoint.
func (s *Server) databaseDiskSize() (int64, error) {
	var total int64
	found := false
	for _, path := range []string{s.dbPath, s.dbPath + ".new"} {
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		total += info.Size()
		found = true
	}
	if !found {
		return 0, fmt.Errorf("no database file(s) available")
	}
	return total, nil
}
