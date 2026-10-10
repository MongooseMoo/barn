package format

import (
	"bufio"
	"io"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// smallReader has a buffer far shorter than the long lines below, so those
// take readLineBytes's path for a line that does not fit.
func smallReader(text string) *bufio.Reader {
	return bufio.NewReaderSize(strings.NewReader(text), 16)
}

func TestReadLineBytesReturnsWhatReadStringWould(t *testing.T) {
	long := strings.Repeat("abcdefghij", 20)
	text := "7\n\n" + long + "\n#42\r\nlast line without a newline"

	want := bufio.NewReader(strings.NewReader(text))
	got := smallReader(text)
	for line := 0; ; line++ {
		wantLine, wantErr := want.ReadString('\n')
		gotLine, gotErr := readLineBytes(got)
		if string(gotLine) != wantLine || gotErr != wantErr {
			t.Fatalf("line %d = %q, %v; ReadString gives %q, %v", line, gotLine, gotErr, wantLine, wantErr)
		}
		if wantErr == io.EOF {
			return
		}
	}
}

func TestReadIntAndObjIDParseLinesOfAnyLength(t *testing.T) {
	padded := strings.Repeat(" ", 40) + "123" + strings.Repeat(" ", 40)
	r := smallReader("5\n-17\r\n" + padded + "\n#9\n-1\n" + strings.Repeat(" ", 40) + "#77\n")

	for _, want := range []int{5, -17, 123} {
		got, err := readInt(r)
		if err != nil || got != want {
			t.Fatalf("readInt = %d, %v, want %d", got, err, want)
		}
	}
	for _, want := range []types.ObjID{9, -1, 77} {
		got, err := readObjID(r)
		if err != nil || got != want {
			t.Fatalf("readObjID = %d, %v, want %d", got, err, want)
		}
	}
	if _, err := readInt(r); err != io.EOF {
		t.Fatalf("readInt at end of input: %v, want io.EOF", err)
	}
}

func TestReadIntReportsWhatItCouldNotParse(t *testing.T) {
	_, err := readInt(smallReader("twelve\n"))
	if err == nil || !strings.Contains(err.Error(), `parse int: strconv.Atoi: parsing "twelve"`) {
		t.Fatalf("readInt error = %v", err)
	}
	_, err = readObjID(smallReader("#x\n"))
	if err == nil || !strings.Contains(err.Error(), `parse objid: strconv.ParseInt: parsing "x"`) {
		t.Fatalf("readObjID error = %v", err)
	}
}

func TestReadValueStringsKeepTheirTextAfterLaterReads(t *testing.T) {
	long := strings.Repeat("0123456789", 30)
	database := &Database{Version: 17}
	r := smallReader("2\nfirst\n2\n" + long + "\n2\nfirst\n2\ntrailing space \r\n0\n5\n")

	var values []types.Value
	for i := 0; i < 5; i++ {
		value, err := database.readValue(r)
		if err != nil {
			t.Fatalf("value %d: %v", i, err)
		}
		values = append(values, value)
	}
	// Every string is checked after all the reads: a value that still pointed
	// into the reader's buffer would have been overwritten by now.
	for i, want := range []string{"first", long, "first", "trailing space "} {
		if values[i].Str() != want {
			t.Fatalf("string %d = %q, want %q", i, values[i].Str(), want)
		}
	}
	if values[4].Int() != 5 {
		t.Fatalf("int after the strings = %v, want 5", values[4])
	}
}

func TestReadIntDoesNotAllocate(t *testing.T) {
	text := strings.Repeat("123456\n", 2000)
	r := bufio.NewReader(strings.NewReader(text))
	allocs := testing.AllocsPerRun(1000, func() {
		if _, err := readInt(r); err != nil {
			t.Fatal(err)
		}
	})
	if allocs != 0 {
		t.Fatalf("readInt allocated %v times per line, want 0", allocs)
	}
}
