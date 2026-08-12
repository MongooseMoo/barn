package format

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"
	"testing"
)

func TestReadValueRejectsUntrustedCollectionCounts(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	tests := []struct {
		name      string
		version   int
		input     string
		wantError string
	}{
		{name: "negative list", version: 4, input: "4\n-1\n", wantError: "invalid list count -1"},
		{name: "negative map", version: 17, input: "10\n-1\n", wantError: "invalid map count -1"},
		{name: "absurd list", version: 4, input: "4\n" + strconv.Itoa(maxInt) + "\n", wantError: "read list element 0"},
		{name: "absurd map", version: 17, input: "10\n" + strconv.Itoa(maxInt) + "\n", wantError: "read map pair 0"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			database := &Database{Version: test.version}
			_, err := database.readValue(bufio.NewReader(strings.NewReader(test.input)))
			if err == nil {
				t.Fatal("readValue() error = nil, want an invalid database error")
			}
			if !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("readValue() error = %q, want it to contain %q", err, test.wantError)
			}
		})
	}
}

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
