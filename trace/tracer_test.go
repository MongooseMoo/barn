package trace

import (
	"bytes"
	"errors"
	"io"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

type forbiddenWriter struct{}

func TestInitEmptyFilterTracesAllVerbs(t *testing.T) {
	previous := globalTracer
	t.Cleanup(func() { globalTracer = previous })
	for _, filters := range [][]string{nil, {""}, {" ", ""}} {
		var output bytes.Buffer
		Init(true, filters, &output)
		VerbCall(2, "echo", 1, 2, 0)
		if !IsEnabled() || !strings.Contains(output.String(), `CALL #2:"echo" argc=1`) {
			t.Fatalf("empty CLI filter hid verb events: %q", output.String())
		}
	}
	var output bytes.Buffer
	Init(true, []string{" echo ", ""}, &output)
	VerbCall(2, "excluded", 0, 2, 0)
	VerbCall(2, "echo", 0, 2, 0)
	if strings.Contains(output.String(), "excluded") || !strings.Contains(output.String(), `CALL #2:"echo"`) {
		t.Fatalf("filter normalization: %q", output.String())
	}
}

func (forbiddenWriter) Write([]byte) (int, error) { panic("disabled tracer wrote output") }

func TestDisabledTraceDoesNoFormattingOrIO(t *testing.T) {
	for _, filters := range [][]string{nil, {"[invalid"}} {
		tracer := &Tracer{filters: filters, writer: forbiddenWriter{}}
		if allocations := testing.AllocsPerRun(100, func() {
			tracer.VerbCall(2, "echo", 4096, 2, 0)
			tracer.VerbReturn(2, "echo", types.TYPE_MAP)
			tracer.Exception(2, "echo", types.E_INVARG)
			tracer.Notify(2)
			tracer.Connection("NEW", 1, -1)
		}); allocations != 0 {
			t.Fatalf("disabled trace allocated %g times", allocations)
		}
	}
}

func TestTraceStructuralFormatAndFilters(t *testing.T) {
	var output bytes.Buffer
	tracer := &Tracer{enabled: true, filters: []string{"echo"}, writer: &output}
	tracer.VerbCall(2, "echo", 128, 2, 0)
	tracer.VerbReturn(2, "echo", types.TYPE_MAP)
	tracer.Exception(2, "echo", types.E_INVARG)
	tracer.VerbCall(2, "excluded", 1, 2, 0)
	tracer.VerbReturn(2, "excluded", types.TYPE_STR)
	tracer.Exception(2, "excluded", types.E_TYPE)
	tracer.Notify(2)
	tracer.Connection("NEW", 1, -1)
	want := "[TRACE] CALL #2:\"echo\" argc=128 player=#2 caller=#0\n" +
		"[TRACE] RETURN #2:\"echo\" type=MAP\n" +
		"[TRACE] EXCEPTION #2:\"echo\" E_INVARG\n" +
		"[TRACE]   NOTIFY #2\n" +
		"[TRACE] CONN NEW conn=1 player=#-1\n"
	if got := output.String(); got != want {
		t.Fatalf("trace = %q, want %q", got, want)
	}
	output.Reset()
	tracer.filters = nil
	tracer.VerbReturn(2, "echo", types.TypeCode(-1))
	tracer.Exception(2, "echo", types.ErrorCode(-1))
	if got := output.String(); !strings.Contains(got, "type=UNKNOWN") || !strings.Contains(got, "E_UNKNOWN") {
		t.Fatalf("unknown codes escaped bounded format: %s", got)
	}
}

func TestTraceQuotesVerbIdentityAndSerializesRecords(t *testing.T) {
	var output bytes.Buffer
	tracer := &Tracer{enabled: true, writer: &output}
	var writers sync.WaitGroup
	for range 8 {
		writers.Add(1)
		go func() {
			defer writers.Done()
			for range 64 {
				tracer.VerbCall(2, "verb\n[TRACE] forged", 1, 2, 0)
			}
		}()
	}
	writers.Wait()
	lines := strings.Split(strings.TrimSuffix(output.String(), "\n"), "\n")
	if len(lines) != 512 {
		t.Fatalf("record count = %d", len(lines))
	}
	for _, line := range lines {
		if line != `[TRACE] CALL #2:"verb\n[TRACE] forged" argc=1 player=#2 caller=#0` {
			t.Fatalf("record was injected or interleaved: %q", line)
		}
	}
}

type failingWriter struct{ calls int }

func (w *failingWriter) Write([]byte) (int, error) {
	w.calls++
	return 0, errors.New("WRITER_ERROR_SECRET_209")
}

func TestTraceWriteErrorDoesNotEmitDebugFallback(t *testing.T) {
	writer := &failingWriter{}
	tracer := &Tracer{enabled: true, writer: writer}
	tracer.VerbCall(2, "echo", 1, 2, 0)
	tracer.VerbReturn(2, "echo", types.TYPE_STR)
	tracer.Exception(2, "echo", types.E_INVARG)
	if writer.calls != 3 {
		t.Fatalf("unexpected retry/fallback writes: %d", writer.calls)
	}
}

func TestTraceAllocationsDoNotGrowPerArgument(t *testing.T) {
	tracer := &Tracer{enabled: true, writer: io.Discard}
	small := testing.AllocsPerRun(100, func() { tracer.VerbCall(2, "echo", 1, 2, 0) })
	large := testing.AllocsPerRun(100, func() { tracer.VerbCall(2, "echo", 4096, 2, 0) })
	// fmt may box the large integer count; it cannot allocate per argument.
	if large > small+1 {
		t.Fatalf("call allocations scaled with argument count: 1=%g 4096=%g", small, large)
	}
}

func BenchmarkTraceMetadata(b *testing.B) {
	for _, count := range []int{0, 1, 128, 4096} {
		b.Run("argc="+strconv.Itoa(count), func(b *testing.B) {
			tracer := &Tracer{enabled: true, writer: io.Discard}
			b.ReportAllocs()
			for b.Loop() {
				tracer.VerbCall(2, "echo", count, 2, 0)
			}
		})
	}
	for _, kind := range []types.TypeCode{types.TYPE_STR, types.TYPE_LIST, types.TYPE_MAP, types.TYPE_WAIF} {
		b.Run("return="+kind.String(), func(b *testing.B) {
			tracer := &Tracer{enabled: true, writer: io.Discard}
			b.ReportAllocs()
			for b.Loop() {
				tracer.VerbReturn(2, "echo", kind)
			}
		})
	}
}
