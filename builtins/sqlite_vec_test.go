package builtins

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func TestSqliteVectorSearch(t *testing.T) {
	ctx := sqliteWizardCtx()
	handle := sqliteMustResult(t, builtinSqliteOpen(ctx, []types.Value{types.NewStr(":memory:")}))
	t.Cleanup(func() { sqliteMustResult(t, builtinSqliteClose(ctx, []types.Value{handle})) })

	created := sqliteMustResult(t, builtinSqliteQuery(ctx, []types.Value{
		handle, types.NewStr("CREATE VIRTUAL TABLE embeddings USING vec0(embedding float[2])"),
	}))
	if created.Type() != types.TYPE_LIST || created.Len() != 0 {
		t.Fatalf("creating vec0 table returned %v, want an empty list", created)
	}
	inserted := sqliteMustResult(t, builtinSqliteExecute(ctx, []types.Value{
		handle,
		types.NewStr("INSERT INTO embeddings(rowid, embedding) VALUES (1, ?), (2, ?)"),
		types.NewList([]types.Value{types.NewStr("[3,4]"), types.NewStr("[0,0]")}),
	}))
	if inserted.Type() != types.TYPE_LIST || inserted.Len() != 0 {
		t.Fatalf("inserting vectors returned %v, want an empty list", inserted)
	}
	rows := sqliteMustList(t, sqliteMustResult(t, builtinSqliteExecute(ctx, []types.Value{
		handle,
		types.NewStr("SELECT rowid, distance FROM embeddings WHERE embedding MATCH ? AND k = 1 ORDER BY distance"),
		types.NewList([]types.Value{types.NewStr("[0,0]")}),
	})))
	if rows.Len() != 1 {
		t.Fatalf("vector search returned %v, want one row", rows)
	}
	row := sqliteMustList(t, rows.Get(1))
	if row.Len() != 2 || sqliteMustInt(t, row.Get(1)) != 2 || row.Get(2).Type() != types.TYPE_FLOAT || row.Get(2).Float() != 0 {
		t.Fatalf("vector search returned row %v, want rowid 2 and distance 0.0", row)
	}
}
