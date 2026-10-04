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
					want = encodeBinaryBytes([]byte(line))
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
