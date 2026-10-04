package format

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDatabaseRetainsDeclaredProgramCountAcrossVersions(t *testing.T) {
	// This legacy dump declares one empty program for a missing object. The
	// existing loader consumes it without attaching it; an audit must still see
	// the declared count rather than silently deriving zero from loaded verbs.
	v4 := filepath.Join(t.TempDir(), "program-count.db")
	const legacyDump = "** LambdaMOO Database, Format Version 4 **\n0\n1\n0\n0\n#9:0\n.\n0 clocks\n0 queued tasks\n0 suspended tasks\n"
	if err := os.WriteFile(v4, []byte(legacyDump), 0600); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct {
		name     string
		path     string
		version  int
		programs int
	}{
		{"v4", v4, 4, 1},
		{"v5", filepath.Join("testdata", "Broken5.db"), 5, 1},
		{"v17", filepath.Join("testdata", "toastcore.db"), 17, 1950},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			database, err := LoadDatabase(fixture.path)
			if err != nil {
				t.Fatal(err)
			}
			if database.Version != fixture.version || database.DeclaredPrograms != fixture.programs {
				t.Fatalf("version=%d declared programs=%d, want version=%d programs=%d", database.Version, database.DeclaredPrograms, fixture.version, fixture.programs)
			}
		})
	}
}
