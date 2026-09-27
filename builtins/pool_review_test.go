package builtins

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func TestPoolReviewUniqueBounds(t *testing.T) {
	for _, n := range []int{0, 8, 9, 16384, 16385} {
		scratch, pool := borrowUniqueScratch(n)
		if len(scratch.heads) != 0 || cap(scratch.previous) < n {
			t.Fatalf("n=%d: dirty or undersized scratch", n)
		}
		if (pool == nil) != (n > 16384) {
			t.Fatalf("n=%d: wrong pool eligibility", n)
		}
		if pool != nil {
			if cap(scratch.previous) > 16384 {
				t.Fatal("oversized retained slice")
			}
			pool.Put(scratch)
		}
	}
}

func TestPoolReviewUniqueConcurrentOwnership(t *testing.T) {
	for worker := range 8 {
		t.Run(fmt.Sprint(worker), func(t *testing.T) {
			t.Parallel()
			ctx := newTestExecution()
			var held types.Value
			var first types.Value
			for iteration := range 100 {
				values := make([]types.Value, 128)
				want := make([]types.Value, 64)
				for i := range want {
					want[i] = types.NewStr(fmt.Sprintf("worker%d-round%d-value%d", worker, iteration, i))
					values[i], values[i+64] = want[i], want[i]
				}
				result := builtinUnique(ctx, []types.Value{types.NewList(values)})
				expected := types.NewList(want)
				if !result.IsNormal() || !result.Val.Equal(expected) {
					t.Fatal("foreign, stale, or unordered result")
				}
				if iteration == 0 {
					held, first = result.Val, expected
				}
				if !held.Equal(first) {
					t.Fatal("earlier result changed on same-class reuse")
				}
			}
		})
	}
}

func TestPoolReviewFileBufferBoundsAndConversionOwnership(t *testing.T) {
	for _, n := range []int{0, 1, 64, 65, 65536, 65537} {
		buf, owner, pool := borrowFileReadBuffer(n)
		if len(buf) != n || ((pool == nil) != (n == 0 || n > 65536)) {
			t.Fatalf("n=%d: wrong buffer or pool", n)
		}
		if pool != nil {
			if cap(buf) > 65536 || owner == nil {
				t.Fatal("invalid retained buffer")
			}
			copy(buf, bytes.Repeat([]byte{'A'}, n))
			text, binary := filterTextMode(buf), encodeBinaryBytes(buf)
			for i := range buf {
				buf[i] = 'B'
			}
			if text != strings.Repeat("A", n) || binary != text {
				t.Fatal("conversion aliases borrowed storage")
			}
			pool.Put(owner)
		}
	}
}

func TestPoolReviewFileConcurrentOwnership(t *testing.T) {
	for worker := range 8 {
		t.Run(fmt.Sprint(worker), func(t *testing.T) {
			t.Parallel()
			file, err := os.CreateTemp(t.TempDir(), "read")
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			ctx := newTestExecution()
			ctx.IsWizard = true
			h := &mooFileHandle{id: 1, file: file, mode: "r-tf", binary: worker%2 == 0}
			ctx.Session.runtime.files.handles[1] = h
			var held string
			var first string
			for iteration := range 30 {
				// Both 63 and 33 bytes use the same 64-byte pool. Shorter reads
				// must never expose bytes left over from the preceding longer file.
				count := 21
				if iteration%2 != 0 {
					count = 11
				}
				letter := string(rune('A' + worker))
				if iteration%2 != 0 {
					letter = strings.ToLower(letter)
				}
				data := []byte(strings.Repeat(letter+"~\x00", count))
				if err := file.Truncate(0); err != nil {
					t.Fatal(err)
				}
				if _, err := file.WriteAt(data, 0); err != nil {
					t.Fatal(err)
				}
				if _, err := file.Seek(0, 0); err != nil {
					t.Fatal(err)
				}
				want := strings.Repeat(letter+"~", count)
				if h.binary {
					want = strings.Repeat(letter+"~7E~00", count)
				}
				result := builtinFileRead(ctx, []types.Value{types.NewInt(1), types.NewInt(64)})
				if !result.IsNormal() || result.Val.Str() != want {
					t.Fatal("foreign, stale, or malformed file result", result)
				}
				if iteration == 0 {
					held, first = result.Val.Str(), want
				}
				if held != first {
					t.Fatal("earlier file result changed")
				}
			}
		})
	}
}
