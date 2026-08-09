package format

import (
	"bufio"
	"fmt"
	"strings"
	"testing"
)

func TestSkipValueAfterTypeMatchesWaifReader(t *testing.T) {
	for _, propdefsLen := range []int{2, 34} {
		t.Run(fmt.Sprintf("propdefs_%d", propdefsLen), func(t *testing.T) {
			lastProp := propdefsLen - 1
			input := fmt.Sprintf("c 0\n#1\n#2\n%d\n%d\n0\n123\n-1\n.\n0\n999\n", propdefsLen, lastProp)

			readDB := &Database{Version: 17}
			readStream := bufio.NewReader(strings.NewReader(input))
			if _, err := readDB.readValueAfterType(readStream, TypeWaif); err != nil {
				t.Fatalf("read WAIF: %v", err)
			}
			readNext, err := readDB.readValue(readStream)
			if err != nil {
				t.Fatalf("read value following WAIF: %v", err)
			}

			skipDB := &Database{Version: 17}
			skipStream := bufio.NewReader(strings.NewReader(input))
			if err := skipDB.skipValueAfterType(skipStream, TypeWaif); err != nil {
				t.Fatalf("skip WAIF: %v", err)
			}
			skipNext, err := skipDB.readValue(skipStream)
			if err != nil {
				t.Fatalf("read value following skipped WAIF: %v", err)
			}

			if got, want := readNext.Int(), int64(999); got != want {
				t.Fatalf("value following read WAIF = %d, want %d", got, want)
			}
			if got, want := skipNext.Int(), readNext.Int(); got != want {
				t.Fatalf("value following skipped WAIF = %d, want %d", got, want)
			}
			if got, want := skipDB.savedWaifs[0].propsByIndex[lastProp].Int(), int64(123); got != want {
				t.Fatalf("skipped WAIF property %d = %d, want %d", lastProp, got, want)
			}
		})
	}
}
