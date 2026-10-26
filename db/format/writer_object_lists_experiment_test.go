package format

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"errors"
	"math"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func TestWriterObjectListsExperiment(t *testing.T) {
	oldList := func(w *Writer, ids []types.ObjID) error {
		elements := make([]types.Value, len(ids))
		for i, id := range ids {
			elements[i] = types.NewObj(id)
		}
		return w.writeValue(types.NewList(elements))
	}
	cases := [][]types.ObjID{nil, {}, {-1}, {0}, {62, 15}, {math.MinInt64, -1, 0, math.MaxInt64}}
	for _, ids := range cases {
		for _, parents := range []bool{false, true} {
			old := func(w *Writer) error {
				if parents && len(ids) == 0 {
					return w.writeValue(types.NewObj(-1))
				}
				if parents && len(ids) == 1 {
					return w.writeValue(types.NewObj(ids[0]))
				}
				return oldList(w, ids)
			}
			current := func(w *Writer) error {
				if parents {
					return w.writeParents(ids)
				}
				return w.writeObjectList(ids)
			}
			var expected bytes.Buffer
			ref := &Writer{w: bufio.NewWriter(&expected)}
			if err := old(ref); err != nil {
				t.Fatal(err)
			}
			if err := ref.Flush(); err != nil {
				t.Fatal(err)
			}
			for _, size := range []int{1, 16, 4096} {
				for limit := 0; limit <= expected.Len(); limit++ {
					gotSink := &primitiveExperimentSink{remaining: limit}
					wantSink := &primitiveExperimentSink{remaining: limit}
					gotWriter := &Writer{w: bufio.NewWriterSize(gotSink, size)}
					wantWriter := &Writer{w: bufio.NewWriterSize(wantSink, size)}
					got, want := current(gotWriter), old(wantWriter)
					if got == nil {
						got = gotWriter.Flush()
					}
					if want == nil {
						want = wantWriter.Flush()
					}
					if !errors.Is(got, want) || !bytes.Equal(gotSink.Bytes(), wantSink.Bytes()) {
						t.Fatalf("ids=%v parents=%t buffer=%d limit=%d: got error %v, want %v; bytes equal=%t", ids, parents, size, limit, got, want, bytes.Equal(gotSink.Bytes(), wantSink.Bytes()))
					}
				}
			}
		}
	}
	snapshot := primitiveExperimentSnapshot(t)
	var output bytes.Buffer
	if err := NewWriter(&output, snapshot).WriteDatabase(); err != nil {
		t.Fatal(err)
	}
	t.Logf("toastcore snapshot: objects=%d bytes=%d sha256=%x", len(snapshot.Objects), output.Len(), sha256.Sum256(output.Bytes()))
}
