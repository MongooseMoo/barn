package format

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

func TestReadNativeNonePreservesUnbound(t *testing.T) {
	for _, version := range []int{4, 17} {
		database := &Database{Version: version}
		stream := bufio.NewReader(strings.NewReader("6\n0\n0\n5\n0\n73\n"))
		for index, expected := range []types.Value{types.Unbound, types.NewInt(0), types.None, types.NewInt(73)} {
			got, err := database.readValue(stream)
			if err != nil {
				t.Fatalf("version %d value %d: %v", version, index, err)
			}
			if !got.Equal(expected) {
				t.Fatalf("version %d value %d: got %v (unbound=%v, clear=%v), want %v", version, index, got, got.IsUnbound(), got.IsNone(), expected)
			}
		}
	}
}

func TestWriteUnboundUsesNativeNone(t *testing.T) {
	for _, test := range []struct {
		name  string
		value types.Value
		wire  string
	}{
		{"uninitialized", types.Unbound, "6\n"},
		{"integer zero", types.NewInt(0), "0\n0\n"},
		{"clear property", types.None, "5\n"},
		{"nested sentinels", types.NewList([]types.Value{types.Unbound, types.NewInt(0), types.None}), "4\n3\n6\n0\n0\n5\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var saved bytes.Buffer
			writer := NewWriter(&saved, store.Snapshot{})
			if err := writer.writeValue(test.value); err != nil {
				t.Fatal(err)
			}
			if err := writer.Flush(); err != nil {
				t.Fatal(err)
			}
			if got := saved.String(); got != test.wire {
				t.Fatalf("wire = %q, want native %q", got, test.wire)
			}
			database := &Database{Version: 17}
			got, err := database.readValue(bufio.NewReader(&saved))
			if err != nil || !got.Equal(test.value) {
				t.Fatalf("round trip = %v, error %v; want %v", got, err, test.value)
			}
		})
	}
}

func TestUnboundRuntimeEnvironmentRoundTrip(t *testing.T) {
	names := []string{"missing", "zero", "used"}
	variables := map[string]types.Value{"missing": types.Unbound, "zero": types.NewInt(0), "used": types.NewInt(73)}
	for round := 0; round < 2; round++ {
		var saved bytes.Buffer
		writer := NewWriter(&saved, store.Snapshot{})
		if err := writer.writeRtEnv(names, variables); err != nil {
			t.Fatal(err)
		}
		if err := writer.Flush(); err != nil {
			t.Fatal(err)
		}
		const nativeEnvironment = "3 variables\nmissing\n6\nzero\n0\n0\nused\n0\n73\n"
		if got := saved.String(); got != nativeEnvironment {
			t.Errorf("round %d environment = %q, want %q", round, got, nativeEnvironment)
		}
		database := &Database{Version: 17}
		loaded, err := database.readRtEnv(bufio.NewReader(&saved))
		if err != nil {
			t.Fatal(err)
		}
		if len(loaded) != 3 || !loaded["missing"].IsUnbound() || loaded["zero"].IsUnbound() || !loaded["zero"].Equal(types.NewInt(0)) || !loaded["used"].Equal(types.NewInt(73)) {
			t.Fatalf("round %d: missing unbound=%v, zero unbound=%v, named values=%d", round, loaded["missing"].IsUnbound(), loaded["zero"].IsUnbound(), len(loaded))
		}
		variables = loaded
	}
}
