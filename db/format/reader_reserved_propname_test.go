package format

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Toast's object reader (src/db_file.cc, where propdefs are read) logs
// "DB_WARNING: Property #N.x has a reserved name" for each property definition
// whose name is a built-in property name in any case, and keeps the definition
// (issue #1091).
func TestLoadDatabaseWarnsOfReservedPropertyName(t *testing.T) {
	raw := "** LambdaMOO Database, Format Version 4 **\n1\n0\n0\n0\n" +
		"#0\nroot\n\n0\n-1\n-1\n-1\n-1\n-1\n-1\n-1\n0\n2\nLocation\nkept\n2\n" +
		"0\n1\n-1\n5\n0\n2\n-1\n5\n" +
		"0 clocks\n0 queued tasks\n0 suspended tasks\n"
	path := filepath.Join(t.TempDir(), "reserved.db")
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	prior := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(prior)

	database, err := LoadDatabase(path)
	if err != nil {
		t.Fatalf("LoadDatabase failed: %v", err)
	}
	logOutput := buf.String()
	if got := strings.Count(logOutput, "has a reserved name"); got != 1 {
		t.Fatalf("reserved-name warnings = %d, want 1\nfull log:\n%s", got, logOutput)
	}
	if !strings.Contains(logOutput, "DB_WARNING: Property #0.Location has a reserved name") {
		t.Fatalf("log missing the reserved-name warning\nfull log:\n%s", logOutput)
	}
	if names := database.Objects[0].PropOrder(); len(names) != 2 || names[0] != "Location" || names[1] != "kept" {
		t.Fatalf("property definitions = %q, want [Location kept]", names)
	}
}
