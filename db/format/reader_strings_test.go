package format

import (
	"bufio"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func TestReadValueSharesEqualStringsWithinALoad(t *testing.T) {
	database := &Database{Version: 17}
	stream := bufio.NewReader(strings.NewReader("2\nlamp\n2\nlamp\n2\ndesk\n2\n\n2\nlamp\n"))
	want := []string{"lamp", "lamp", "desk", "", "lamp"}

	for i, text := range want {
		value, err := database.readValue(stream)
		if err != nil {
			t.Fatalf("value %d: %v", i, err)
		}
		if !value.Identical(types.NewStr(text)) {
			t.Fatalf("value %d = %v, want %q", i, value, text)
		}
	}

	if got := len(database.loadedStrings); got != 3 {
		t.Fatalf("distinct strings remembered = %d, want 3 (lamp, desk and the empty string)", got)
	}
}

func TestLoadedStrDoesNotMixDifferentStrings(t *testing.T) {
	database := &Database{}

	first := database.loadedStr("Lamp")
	second := database.loadedStr("lamp")

	if !first.Identical(types.NewStr("Lamp")) || !second.Identical(types.NewStr("lamp")) {
		t.Fatalf("loadedStr folded case: %v, %v", first, second)
	}
}
