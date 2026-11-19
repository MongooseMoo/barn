package format

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/types"
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

// Toast stores an error value as the whole 64-bit number of its value union
// (db_io.cc dbio_write_var), but an error literal only ever assigns the 32-bit
// error code (parser.y), so the upper half is leftover memory. Toast reads the
// code back through the same 32-bit field. These numbers are the ones a Toast
// checkpoint held for E_NONE, E_RECMOVE and E_FILE.
func TestReadValueKeepsLow32BitsOfStoredErrorNumber(t *testing.T) {
	tests := []struct {
		stored string
		want   types.ErrorCode
	}{
		{stored: "0", want: types.E_NONE},
		{stored: "18", want: types.E_INTRPT},
		{stored: "8030606863672279040", want: types.E_NONE},
		{stored: "106734232272904", want: types.E_RECMOVE},
		{stored: "106734232272912", want: types.E_FILE},
		{stored: "-4294967295", want: types.E_TYPE},
	}
	for _, test := range tests {
		t.Run(test.stored, func(t *testing.T) {
			database := &Database{Version: 17}
			value, err := database.readValue(bufio.NewReader(strings.NewReader("3\n" + test.stored + "\n")))
			if err != nil {
				t.Fatalf("readValue() error = %v", err)
			}
			if value.Type() != types.TYPE_ERR || value.ErrCode() != test.want {
				t.Fatalf("readValue() = %v, want error code %d", value, test.want)
			}
		})
	}
}
