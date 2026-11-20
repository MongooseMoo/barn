package builtins

import (
	"context"
	"database/sql"
	"testing"
)

// SQLite, not the statement text, decides which inline statements may skip the
// irreversible-effect boundary. The Mongoose say path's cache-version probe
// (#6128:lookup_cache_version) must be shareable; anything that writes, or that
// changes connection state a retried attempt would change again, must not.
func TestSQLiteStatementShareable(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.ExecContext(context.Background(), "CREATE TABLE t(x INTEGER)"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, sql string
		want      bool
	}{
		{"select", "SELECT x FROM t", true},
		{"mongoose_cache_version", "SELECT total_changes(), (SELECT data_version FROM pragma_data_version), (SELECT schema_version FROM pragma_schema_version)", true},
		{"select_cte", "WITH v(x) AS (SELECT 7) SELECT x FROM v", true},
		{"two_selects", "SELECT 1; SELECT 2", true},
		{"insert", "INSERT INTO t VALUES (1)", false},
		{"with_insert", "WITH v(x) AS (SELECT 7) INSERT INTO t SELECT x FROM v", false},
		{"select_then_insert", "SELECT 1; INSERT INTO t VALUES (1)", false},
		{"pragma_write", "PRAGMA user_version = 17", false},
		{"pragma_setting", "PRAGMA cache_size = 100", false},
		{"pragma_read", "PRAGMA user_version", false},
		{"begin", "BEGIN", false},
		{"commit", "COMMIT", false},
		{"savepoint", "SAVEPOINT s", false},
		{"attach", "ATTACH DATABASE ':memory:' AS other", false},
		{"detach", "DETACH DATABASE other", false},
		{"create_if_not_exists", "CREATE TABLE IF NOT EXISTS t(x INTEGER)", false},
		{"syntax_error", "SELEC 1", false},
		{"missing_table", "SELECT * FROM missing", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := sqliteStatementShareable(conn, tc.sql); got != tc.want {
				t.Fatalf("sqliteStatementShareable(%q) = %v, want %v", tc.sql, got, tc.want)
			}
		})
	}

	// Inside an open transaction a read can see rows a later ROLLBACK removes.
	if _, err := conn.ExecContext(context.Background(), "BEGIN"); err != nil {
		t.Fatal(err)
	}
	if sqliteStatementShareable(conn, "SELECT x FROM t") {
		t.Fatal("a SELECT inside an open transaction was classified shareable")
	}
	if _, err := conn.ExecContext(context.Background(), "ROLLBACK"); err != nil {
		t.Fatal(err)
	}
	// Classification leaves no authorizer behind: an ATTACH still runs.
	if _, err := conn.ExecContext(context.Background(), "ATTACH DATABASE ':memory:' AS other"); err != nil {
		t.Fatalf("ATTACH after classification: %v", err)
	}
}
