package vm

import (
	"errors"
	"fmt"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

type countingStringer struct {
	calls *int
}

func (s countingStringer) String() string {
	*s.calls++
	return "dynamic detail"
}

func TestMooErrorDefersDetailFormatting(t *testing.T) {
	calls := 0
	err := newMooErrorf(types.E_TYPE, "bad %s", countingStringer{calls: &calls})

	if calls != 0 {
		t.Fatalf("detail formatted while constructing error: calls = %d", calls)
	}
	if got, want := err.Error(), "E_TYPE: bad dynamic detail"; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
	if calls != 1 {
		t.Fatalf("detail formatting calls = %d, want 1", calls)
	}
}

func TestErrorCodeRecognizesWrappedMooErrorWithoutFormatting(t *testing.T) {
	calls := 0
	err := (&VM{}).annotateError(newMooErrorf(types.E_RANGE, "bad %s", countingStringer{calls: &calls}), 7)

	if got := errorCode(err); got != types.E_RANGE {
		t.Fatalf("errorCode() = %s, want E_RANGE", got)
	}
	if calls != 0 {
		t.Fatalf("detail formatted while extracting code: calls = %d", calls)
	}

	var mooErr MooError
	if !errors.As(err, &mooErr) {
		t.Fatal("wrapped error does not expose MooError")
	}
	if got, want := err.Error(), "E_RANGE: bad dynamic detail (line 7)"; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

func TestErrorCodeReadsStructuredErrorsWithoutAllocating(t *testing.T) {
	machine := &VM{}
	tests := []struct {
		name string
		err  error
		want types.ErrorCode
	}{
		{name: "moo error", err: newMooError(types.E_PERM, "no"), want: types.E_PERM},
		{name: "exception", err: VMException{Code: types.E_INVARG, Value: types.NewInt(1)}, want: types.E_INVARG},
		{name: "annotated moo error", err: machine.annotateError(newMooError(types.E_RANGE, ""), 3), want: types.E_RANGE},
		{name: "twice annotated exception", err: machine.annotateError(machine.annotateError(VMException{Code: types.E_DIV}, 3), 9), want: types.E_DIV},
		{name: "wrapped by fmt", err: fmt.Errorf("context: %w", newMooError(types.E_TYPE, "")), want: types.E_TYPE},
		{name: "text only", err: errors.New("E_VERBNF: nothing there"), want: types.E_VERBNF},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := errorCode(tt.err); got != tt.want {
				t.Fatalf("errorCode() = %s, want %s", got, tt.want)
			}
		})
	}
	for _, tt := range tests[:4] {
		if allocs := testing.AllocsPerRun(100, func() { _ = errorCode(tt.err) }); allocs != 0 {
			t.Fatalf("%s: errorCode allocated %v times", tt.name, allocs)
		}
	}
}

func TestMooErrorFormattingMatchesLegacyErrors(t *testing.T) {
	tests := []struct {
		name string
		err  MooError
		want string
	}{
		{name: "code only", err: newMooError(types.E_DIV, ""), want: "E_DIV"},
		{name: "static detail", err: newMooError(types.E_DIV, "division by zero"), want: "E_DIV: division by zero"},
		{name: "formatted detail", err: newMooErrorf(types.E_INVIND, "invalid object #%d", 17), want: "E_INVIND: invalid object #17"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Fatalf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

var benchmarkError error

func BenchmarkMooErrorRaise(b *testing.B) {
	b.Run("typed", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			benchmarkError = newMooError(types.E_DIV, "division by zero")
		}
	})
	b.Run("legacy-formatted", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			benchmarkError = fmt.Errorf("E_DIV: division by zero")
		}
	})
}
