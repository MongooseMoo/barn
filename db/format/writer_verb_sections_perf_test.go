package format

import (
	"bytes"
	"fmt"
	"io"
	"testing"

	"github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

func TestVerbSectionSerializationBytes(t *testing.T) {
	snapshot := store.Snapshot{
		AllObjects: []*store.SnapshotObject{
			nil,
			{ID: 2, Recycled: true, VerbList: []store.VerbView{{HasProgram: true}}},
			{ID: 7, VerbList: []store.VerbView{
				{},
				{HasProgram: true, Code: []string{"return 7;"}},
				{HasProgram: true},
			}},
		},
		AnonymousObjects: []*store.SnapshotObject{
			{ID: 12, Anonymous: true, VerbList: []store.VerbView{
				{HasProgram: true},
				{},
				{HasProgram: true, Code: []string{"return 12;", ""}},
			}},
		},
	}
	var output bytes.Buffer
	w := NewWriter(&output, snapshot)
	if err := w.writeVerbCodeSections(); err != nil {
		t.Fatal(err)
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	const want = "4\n#7:1\nreturn 7;\n.\n#7:2\n.\n#12:0\n.\n#12:2\nreturn 12;\n\n.\n"
	if got := output.String(); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// Measures verb-section serialization alone, including the writer's buffer.
// The synthetic snapshots have 10% anonymous objects and three verbs per
// object: one absent, one empty-but-present, and one two-line program.
func BenchmarkVerbSectionSerialization(b *testing.B) {
	for _, count := range []int{100, 10000} {
		b.Run(fmt.Sprintf("objects=%d", count), func(b *testing.B) {
			snapshot := store.Snapshot{}
			for i := 0; i < count; i++ {
				obj := &store.SnapshotObject{
					ID: types.ObjID(i), Anonymous: i >= count*9/10,
					VerbList: []store.VerbView{
						{}, {HasProgram: true},
						{HasProgram: true, Code: []string{"x = 1;", "return x;"}},
					},
				}
				if obj.Anonymous {
					snapshot.AnonymousObjects = append(snapshot.AnonymousObjects, obj)
				} else {
					snapshot.AllObjects = append(snapshot.AllObjects, obj)
				}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				w := NewWriter(io.Discard, snapshot)
				if err := w.writeVerbCodeSections(); err != nil {
					b.Fatal(err)
				}
				if err := w.Flush(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
