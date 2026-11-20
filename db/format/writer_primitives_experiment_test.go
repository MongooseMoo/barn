package format

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/db/store"
)

// This experiment compares encoding with the previous fmt primitives and measures
// serialization only. Loading, snapshot capture and disk I/O are outside timing.
func TestWriterPrimitivesExperiment(t *testing.T) {
	for _, size := range []int{1, 16, 21, 4096} {
		var actual, expected bytes.Buffer
		w := &Writer{w: bufio.NewWriterSize(&actual, size)}
		for _, value := range []int64{0, 1, -1, 255, 256, math.MinInt64, math.MaxInt64} {
			if err := w.writeInt64(value); err != nil {
				t.Fatal(err)
			}
			fmt.Fprintf(&expected, "%d\n", value)
		}
		for _, value := range []int{0, -1, math.MinInt, math.MaxInt} {
			if err := w.writeInt(value); err != nil {
				t.Fatal(err)
			}
			fmt.Fprintf(&expected, "%d\n", value)
		}
		for _, value := range []string{"", "ordinary", "a\x00b\r\n", "MOO \u03bb", strings.Repeat("long", 4096)} {
			if err := w.writeString(value); err != nil {
				t.Fatal(err)
			}
			fmt.Fprintf(&expected, "%s\n", value)
		}
		if err := w.Flush(); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(actual.Bytes(), expected.Bytes()) {
			t.Fatalf("buffer size %d: bytes differ", size)
		}
	}
	// Compare the complete buffered operation (write then flush) and committed
	// prefix at each failure position, including failure on the newline.
	for _, value := range []string{"", "abc", strings.Repeat("long", 20)} {
		for limit := 0; limit <= len(value)+1; limit++ {
			actual := &primitiveExperimentSink{remaining: limit}
			expected := &primitiveExperimentSink{remaining: limit}
			w := &Writer{w: bufio.NewWriterSize(actual, 16)}
			old := bufio.NewWriterSize(expected, 16)
			got := w.writeString(value)
			_, want := fmt.Fprintf(old, "%s\n", value)
			if got == nil {
				got = w.Flush()
			}
			if want == nil {
				want = old.Flush()
			}
			if !errors.Is(got, want) || !bytes.Equal(actual.Bytes(), expected.Bytes()) {
				t.Fatalf("string length %d failure after %d bytes differs", len(value), limit)
			}
		}
	}
	for limit := 0; limit <= 21; limit++ {
		actual := &primitiveExperimentSink{remaining: limit}
		expected := &primitiveExperimentSink{remaining: limit}
		w := &Writer{w: bufio.NewWriterSize(actual, 16)}
		old := bufio.NewWriterSize(expected, 16)
		got := w.writeInt64(math.MinInt64)
		_, want := fmt.Fprintf(old, "%d\n", int64(math.MinInt64))
		if !errors.Is(got, want) || !errors.Is(w.Flush(), old.Flush()) || !bytes.Equal(actual.Bytes(), expected.Bytes()) {
			t.Fatalf("int64 failure after %d bytes differs", limit)
		}
	}
	snapshot := primitiveExperimentSnapshot(t)
	var output bytes.Buffer
	if err := NewWriter(&output, snapshot).WriteDatabase(); err != nil {
		t.Fatal(err)
	}
	t.Logf("toastcore snapshot: objects=%d bytes=%d sha256=%x", len(snapshot.Objects), output.Len(), sha256.Sum256(output.Bytes()))
}

type primitiveExperimentSink struct {
	bytes.Buffer
	remaining int
}

func (w *primitiveExperimentSink) Write(p []byte) (int, error) {
	if len(p) > w.remaining {
		n, _ := w.Buffer.Write(p[:w.remaining])
		w.remaining = 0
		return n, io.ErrClosedPipe
	}
	w.remaining -= len(p)
	return w.Buffer.Write(p)
}

// Override bytes.Buffer.WriteString so bufio cannot bypass the injected error.
func (w *primitiveExperimentSink) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}

func primitiveExperimentSnapshot(tb testing.TB) store.Snapshot {
	tb.Helper()
	database, err := LoadDatabaseReadOnly("testdata/toastcore.db")
	if err != nil {
		tb.Fatal(err)
	}
	s, err := database.NewStoreFromDatabase()
	if err != nil {
		tb.Fatal(err)
	}
	return s.Snapshot()
}

func BenchmarkWriterPrimitivesExperiment(b *testing.B) {
	b.Run("Int64", func(b *testing.B) {
		w := NewWriter(io.Discard, store.Snapshot{})
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if err := w.writeInt64(int64(i) - 100000); err != nil {
				b.Fatal(err)
			}
		}
		if err := w.Flush(); err != nil {
			b.Fatal(err)
		}
	})
	b.Run("String", func(b *testing.B) {
		w := NewWriter(io.Discard, store.Snapshot{})
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if err := w.writeString("a representative property name"); err != nil {
				b.Fatal(err)
			}
		}
		if err := w.Flush(); err != nil {
			b.Fatal(err)
		}
	})
	b.Run("ToastcoreSnapshot", func(b *testing.B) {
		snapshot := primitiveExperimentSnapshot(b)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if err := NewWriter(io.Discard, snapshot).WriteDatabase(); err != nil {
				b.Fatal(err)
			}
		}
	})
}
