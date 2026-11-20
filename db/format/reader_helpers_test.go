package format

import (
	"slices"
	"testing"
)

// A name an object defines again over its ancestor's definition names only
// the first slot; the shadowed slot keeps its position under a placeholder.
func TestDistinctSlotNamesRenamesShadowedSlots(t *testing.T) {
	got := distinctSlotNames([]string{"0", "0", "Name", "name", "_inherited_4"})
	want := []string{"0", "_inherited_1", "Name", "_inherited_3", "_inherited_4"}
	if !slices.Equal(got, want) {
		t.Fatalf("distinctSlotNames = %q, want %q", got, want)
	}
	names := []string{"a", "b"}
	if got := distinctSlotNames(names); !slices.Equal(got, names) {
		t.Fatalf("distinct names changed: %q", got)
	}
}
