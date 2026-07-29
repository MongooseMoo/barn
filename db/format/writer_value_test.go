package format

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"barn/db/store"
	"barn/types"
)

func TestWriteWaifPropertiesFollowClassIndexOrder(t *testing.T) {
	const propertyCount = 8
	classProperties := make([]string, propertyCount)
	waif := types.NewWaif(1, 2)
	for i := 0; i < propertyCount; i++ {
		name := fmt.Sprintf("p%d", i)
		classProperties[i] = ":" + name
		waif = waif.SetProperty(name, types.NewInt(int64(100+i)))
	}

	snapshot := store.Snapshot{
		Objects: map[types.ObjID]*store.SnapshotObject{
			1: {ID: 1},
		},
		PropertyNames: map[types.ObjID][]string{
			1: classProperties,
		},
	}

	var expected strings.Builder
	expected.WriteString("c 0\n1\n2\n8\n")
	for i := 0; i < propertyCount; i++ {
		fmt.Fprintf(&expected, "%d\n0\n%d\n", i, 100+i)
	}
	expected.WriteString("-1\n.\n")

	for i := 0; i < 64; i++ {
		var buf bytes.Buffer
		writer := NewWriter(&buf, snapshot)
		if err := writer.writeWaif(waif); err != nil {
			t.Fatalf("writeWaif iteration %d: %v", i, err)
		}
		if err := writer.Flush(); err != nil {
			t.Fatalf("Flush iteration %d: %v", i, err)
		}
		if got := buf.String(); got != expected.String() {
			t.Fatalf("iteration %d WAIF properties are not in class index order:\n%s", i, got)
		}
	}
}
