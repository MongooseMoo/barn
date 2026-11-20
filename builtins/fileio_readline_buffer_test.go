package builtins

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/types"
)

func readlineFixture(t testing.TB, data, mode string) (*Execution, types.Value) {
	t.Helper()
	t.Chdir(t.TempDir())
	if err := os.Mkdir("files", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("files/line", []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	ctx := newTestExecution()
	ctx.IsWizard = true
	r := builtinFileOpen(ctx, []types.Value{types.NewStr("line"), types.NewStr(mode)})
	if r.IsError() {
		t.Fatal(r)
	}
	t.Cleanup(func() { builtinFileClose(ctx, []types.Value{r.Val}) })
	return ctx, r.Val
}

func requireFileString(t testing.TB, r types.Result, want string) {
	t.Helper()
	if !r.IsNormal() || r.Val.Str() != want {
		t.Fatalf("result = %v, want %q", r, want)
	}
}

func requireFileInt(t testing.TB, r types.Result, want int64) {
	t.Helper()
	if !r.IsNormal() || r.Val.Int() != want {
		t.Fatalf("result = %v, want %d", r, want)
	}
}

func TestFileGrepFirstMatchPreservesNextLine(t *testing.T) {
	for _, test := range []struct{ mode, wantNext string }{
		{"r-tf", "omega"},
		{"r-bf", "omega~0A"},
	} {
		t.Run(test.mode, func(t *testing.T) {
			ctx, id := readlineFixture(t, "alpha\nneedle\nomega\n", test.mode)
			requireFileInt(t, builtinFileSeek(ctx, []types.Value{id, types.NewInt(15)}), 15)
			result := admitTestBuiltin("file_grep", ctx, []types.Value{id, types.NewStr("needle")})
			if !result.IsNormal() || result.Val.String() != `{{"needle", 2}}` {
				t.Fatalf("file_grep = %+v, want {{\"needle\", 2}}", result)
			}
			requireFileInt(t, builtinFileTell(ctx, []types.Value{id}), 13)
			requireFileString(t, builtinFileReadline(ctx, []types.Value{id}), test.wantNext)
		})
	}
}

func TestFileScansRewindAndConsumeToEOF(t *testing.T) {
	for _, test := range []struct {
		name, builtin, needle, want string
	}{
		{"count", "file_count_lines", "", "3"},
		{"all matches", "file_grep", "a", `{{"alpha", 1}, {"omega", 3}}`},
		{"no matches", "file_grep", "missing", "{}"},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, id := readlineFixture(t, "alpha\nneedle\nomega\n", "r-tf")
			requireFileInt(t, builtinFileSeek(ctx, []types.Value{id, types.NewInt(15)}), 15)
			args := []types.Value{id}
			if test.builtin == "file_grep" {
				args = append(args, types.NewStr(test.needle), types.NewInt(1))
			}
			result := admitTestBuiltin(test.builtin, ctx, args)
			if !result.IsNormal() || result.Val.String() != test.want {
				t.Fatalf("%s = %+v, want %s", test.builtin, result, test.want)
			}
			requireFileInt(t, builtinFileTell(ctx, []types.Value{id}), 19)
			requireFileInt(t, builtinFileEOF(ctx, []types.Value{id}), 1)
		})
	}
}

func TestFileCountLinesHandlesFragmentsAndFinalLines(t *testing.T) {
	for _, test := range []struct {
		name, data string
		want       int64
	}{
		{"empty", "", 0},
		{"empty line", "\n", 1},
		{"final unterminated", "alpha\nlast", 2},
		{"exact reader buffer", strings.Repeat("x", 4096), 1},
		{"long unterminated", strings.Repeat("x", 65536), 1},
		{"long terminated", strings.Repeat("x", 65536) + "\n", 1},
		{"long with tail", strings.Repeat("x", 65536) + "\ntail\n", 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, id := readlineFixture(t, test.data, "r-tf")
			requireFileInt(t, admitTestBuiltin("file_count_lines", ctx, []types.Value{id}), test.want)
			requireFileInt(t, builtinFileTell(ctx, []types.Value{id}), int64(len(test.data)))
		})
	}
}

func TestFileGrepReadsLongMatchedLineWithoutConsumingTail(t *testing.T) {
	line := strings.Repeat("x", 65536)
	ctx, id := readlineFixture(t, line+"\ntail\n", "r-tf")
	result := admitTestBuiltin("file_grep", ctx, []types.Value{id, types.NewStr("x")})
	if !result.IsNormal() || len(result.Val.Elements()) != 1 {
		t.Fatalf("long-line file_grep = %+v, want one match", result)
	}
	match := result.Val.Elements()[0].Elements()
	if len(match) != 2 || match[0].Str() != line || match[1].Int() != 1 {
		t.Fatal("long-line file_grep did not return the complete first line")
	}
	requireFileInt(t, builtinFileTell(ctx, []types.Value{id}), 65537)
	requireFileString(t, builtinFileReadline(ctx, []types.Value{id}), "tail")
}

func TestFileScansDoNotRunWhenEffectBoundaryRefuses(t *testing.T) {
	for _, name := range []string{"file_count_lines", "file_grep"} {
		t.Run(name, func(t *testing.T) {
			ctx, id := readlineFixture(t, "alpha\nneedle\nomega\n", "r-tf")
			requireFileInt(t, builtinFileSeek(ctx, []types.Value{id, types.NewInt(13)}), 13)
			crossings := 0
			ctx.BeforeIrreversibleEffect = func(string) bool {
				crossings++
				ctx.ConflictRetryRequested = true
				return true
			}
			args := []types.Value{id}
			if name == "file_grep" {
				args = append(args, types.NewStr("needle"))
			}
			result := admitTestBuiltin(name, ctx, args)
			if result.Flow != types.FlowAbortAttempt || crossings != 1 || ctx.IrreversibleSideEffect {
				t.Errorf("%s = %+v, crossings=%d irreversible=%v, want aborted before effect", name, result, crossings, ctx.IrreversibleSideEffect)
			}
			requireFileInt(t, builtinFileTell(ctx, []types.Value{id}), 13)
			requireFileString(t, builtinFileReadline(ctx, []types.Value{id}), "omega")
		})
	}
}

func TestReadlineBufferedValuesAndPosition(t *testing.T) {
	for _, binary := range []bool{false, true} {
		t.Run(fmt.Sprint(binary), func(t *testing.T) {
			long := strings.Repeat("x", 65537)
			lines := []string{"\n", "a\r\n", "b\x00~\n", long + "\n", long + "tail"}
			mode := "r-tf"
			if binary {
				mode = "r-bf"
			}
			ctx, id := readlineFixture(t, strings.Join(lines, ""), mode)
			var pos int64
			for _, line := range lines {
				want := filterTextMode([]byte(strings.TrimRight(line, "\r\n")))
				if binary {
					want = EncodeRawToBinary([]byte(line))
				}
				requireFileString(t, builtinFileReadline(ctx, []types.Value{id}), want)
				pos += int64(len(line))
				requireFileInt(t, builtinFileTell(ctx, []types.Value{id}), pos)
			}
			requireFileInt(t, builtinFileEOF(ctx, []types.Value{id}), 1)
			if r := builtinFileReadline(ctx, []types.Value{id}); r.Error != types.E_FILE {
				t.Fatal(r)
			}
		})
	}
}

func TestReadlineChunkBoundaries(t *testing.T) {
	for _, size := range []int{4095, 4096, 4097, 4098, 8193, 8194} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			line := strings.Repeat("x", size)
			ctx, id := readlineFixture(t, line+"\nnext\n", "r-tf")
			requireFileString(t, builtinFileReadline(ctx, []types.Value{id}), line)
			requireFileInt(t, builtinFileTell(ctx, []types.Value{id}), int64(size+1))
			requireFileString(t, builtinFileReadline(ctx, []types.Value{id}), "next")
		})
	}
}

func TestReadlineMixedOperations(t *testing.T) {
	ctx, id := readlineFixture(t, "first\nsecond\nthird\n", "r+tf")
	a := []types.Value{id}
	requireFileString(t, builtinFileReadline(ctx, a), "first")
	requireFileInt(t, builtinFileTell(ctx, a), 6)
	requireFileString(t, builtinFileRead(ctx, []types.Value{id, types.NewInt(2)}), "se")
	requireFileInt(t, builtinFileSeek(ctx, []types.Value{id, types.NewInt(-2), types.NewStr("cur")}), 6)
	requireFileInt(t, builtinFileWrite(ctx, []types.Value{id, types.NewStr("SECOND")}), 6)
	requireFileString(t, builtinFileReadline(ctx, a), "")
	requireFileInt(t, builtinFileTell(ctx, a), 13)
	// A readlines call temporarily seeks to the start and restores the consumed position.
	r := builtinFileReadlines(ctx, []types.Value{id, types.NewInt(1), types.NewInt(2)})
	if !r.IsNormal() || r.Val.Elements()[1].Str() != "SECOND" {
		t.Fatal(r)
	}
	requireFileInt(t, builtinFileTell(ctx, a), 13)
	requireFileInt(t, builtinFileWriteline(ctx, []types.Value{id, types.NewStr("THIRD")}), 0)
	requireFileInt(t, builtinFileSeek(ctx, []types.Value{id, types.NewInt(-6), types.NewStr("end")}), 13)
	requireFileString(t, builtinFileReadline(ctx, a), "THIRD")
	// Read-ahead must not hide subsequent writes through a different descriptor.
	requireFileInt(t, builtinFileSeek(ctx, []types.Value{id, types.NewInt(0)}), 0)
	requireFileString(t, builtinFileReadline(ctx, a), "first")
	f, err := os.OpenFile("files/line", os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteAt([]byte("changed"), 6)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	requireFileString(t, builtinFileReadline(ctx, a), "changedTHIRD")
}

func TestReadlineConcurrentConsumption(t *testing.T) {
	var data strings.Builder
	for i := range 128 {
		fmt.Fprintf(&data, "line-%03d\n", i)
	}
	ctx, id := readlineFixture(t, data.String(), "r-tf")
	results := make(chan string, 2048)
	errors := make(chan types.Result, 8)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				r := builtinFileReadline(ctx, []types.Value{id})
				if r.IsError() {
					if r.Error != types.E_FILE {
						errors <- r
					}
					return
				}
				results <- r.Val.Str()
			}
		}()
	}
	wg.Wait()
	close(results)
	close(errors)
	for r := range errors {
		t.Fatal(r)
	}
	seen := make(map[string]bool)
	for line := range results {
		if seen[line] {
			t.Fatalf("duplicate %q", line)
		}
		seen[line] = true
	}
	for i := range 128 {
		if !seen[fmt.Sprintf("line-%03d", i)] {
			t.Fatalf("missing line %d; got %v", i, seen)
		}
	}
	requireFileInt(t, builtinFileTell(ctx, []types.Value{id}), int64(data.Len()))
}

func TestReadlinePipeAndClose(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close(); w.Close() })
	ctx := newTestExecution()
	ctx.IsWizard = true
	ctx.Session.runtime.files.handles[1] = &mooFileHandle{id: 1, file: r, mode: "r-tf"}
	id := types.NewInt(1)
	go func() { _, _ = io.WriteString(w, "one\ntwo\n") }()
	requireFileString(t, builtinFileReadline(ctx, []types.Value{id}), "one")
	requireFileString(t, builtinFileReadline(ctx, []types.Value{id}), "two")
	done := make(chan types.Result, 1)
	go func() { done <- builtinFileReadline(ctx, []types.Value{id}) }()
	// Closing must interrupt a blocked pipe read, without waiting for its position lock.
	requireFileInt(t, builtinFileClose(ctx, []types.Value{id}), 0)
	select {
	case result := <-done:
		if !result.IsError() {
			t.Fatal(result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("close did not interrupt read")
	}
}

func BenchmarkReadlineBuffered(b *testing.B) {
	for _, workload := range []struct{ size, lines int }{{0, 1}, {1, 64}, {32, 1}, {32, 64}, {1024, 1}, {65536, 1}} {
		for _, binary := range []bool{false, true} {
			b.Run(fmt.Sprintf("bytes=%d/lines=%d/binary=%t", workload.size, workload.lines, binary), func(b *testing.B) {
				mode := "r-tf"
				if binary {
					mode = "r-bf"
				}
				line := strings.Repeat("x", workload.size) + "\n"
				ctx, id := readlineFixture(b, strings.Repeat(line, workload.lines), mode)
				want := strings.Repeat("x", workload.size)
				if binary {
					want += "~0A"
				}
				seek := []types.Value{id, types.NewInt(0)}
				b.ReportAllocs()
				b.SetBytes(int64(len(line) * workload.lines))
				b.ResetTimer()
				for b.Loop() {
					requireFileInt(b, builtinFileSeek(ctx, seek), 0)
					for range workload.lines {
						requireFileString(b, builtinFileReadline(ctx, []types.Value{id}), want)
					}
				}
			})
		}
	}
}
