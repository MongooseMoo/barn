package builtins

import (
	"barn/types"
	"testing"
)

func TestSortWithKeyList(t *testing.T) {
	ctx := types.NewTaskContext()
	items := types.NewList([]types.Value{
		types.NewStr("beta"),
		types.NewStr("alpha"),
		types.NewStr("gamma"),
	})
	keys := types.NewList([]types.Value{
		types.NewInt(2),
		types.NewInt(1),
		types.NewInt(3),
	})

	res := builtinSort(ctx, []types.Value{items, keys})
	if res.IsError() {
		t.Fatalf("unexpected error: %v", res.Error)
	}

	got := res.Val.(types.ListValue)
	want := []string{"alpha", "beta", "gamma"}
	for i := 1; i <= got.Len(); i++ {
		s, ok := got.Get(i).(types.StrValue)
		if !ok {
			t.Fatalf("expected string at index %d, got %T", i, got.Get(i))
		}
		if s.Value() != want[i-1] {
			t.Fatalf("index %d: got %q, want %q", i, s.Value(), want[i-1])
		}
	}
}

func TestSortRejectsMismatchedKeyLength(t *testing.T) {
	ctx := types.NewTaskContext()
	items := types.NewList([]types.Value{
		types.NewStr("a"),
		types.NewStr("b"),
	})
	keys := types.NewList([]types.Value{
		types.NewInt(1),
	})

	res := builtinSort(ctx, []types.Value{items, keys})
	if !res.IsError() || res.Error != types.E_INVARG {
		t.Fatalf("expected E_INVARG for key length mismatch, got %+v", res)
	}
}

func TestSortNaturalAndReverse(t *testing.T) {
	ctx := types.NewTaskContext()
	items := types.NewList([]types.Value{
		types.NewStr("file10"),
		types.NewStr("file2"),
		types.NewStr("file1"),
	})

	asc := builtinSort(ctx, []types.Value{items, items, types.NewInt(1), types.NewInt(0)})
	if asc.IsError() {
		t.Fatalf("unexpected natural sort error: %v", asc.Error)
	}
	ascList := asc.Val.(types.ListValue)
	ascWant := []string{"file1", "file2", "file10"}
	for i := 1; i <= ascList.Len(); i++ {
		got := ascList.Get(i).(types.StrValue).Value()
		if got != ascWant[i-1] {
			t.Fatalf("natural asc index %d: got %q, want %q", i, got, ascWant[i-1])
		}
	}

	desc := builtinSort(ctx, []types.Value{items, items, types.NewInt(1), types.NewInt(1)})
	if desc.IsError() {
		t.Fatalf("unexpected reverse natural sort error: %v", desc.Error)
	}
	descList := desc.Val.(types.ListValue)
	descWant := []string{"file10", "file2", "file1"}
	for i := 1; i <= descList.Len(); i++ {
		got := descList.Get(i).(types.StrValue).Value()
		if got != descWant[i-1] {
			t.Fatalf("natural desc index %d: got %q, want %q", i, got, descWant[i-1])
		}
	}
}

func TestSortStableWhenKeysAreEqual(t *testing.T) {
	ctx := types.NewTaskContext()
	items := types.NewList([]types.Value{
		types.NewStr("first"),
		types.NewStr("second"),
		types.NewStr("third"),
	})
	keys := types.NewList([]types.Value{
		types.NewInt(1),
		types.NewInt(1),
		types.NewInt(2),
	})

	res := builtinSort(ctx, []types.Value{items, keys})
	if res.IsError() {
		t.Fatalf("unexpected error: %v", res.Error)
	}

	got := res.Val.(types.ListValue)
	want := []string{"first", "second", "third"}
	for i := 1; i <= got.Len(); i++ {
		s := got.Get(i).(types.StrValue).Value()
		if s != want[i-1] {
			t.Fatalf("stable order mismatch at index %d: got %q, want %q", i, s, want[i-1])
		}
	}
}
