package format

import (
	"bufio"
	"strconv"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// listText is the database text of a list of the ints 0 to count-1.
func listText(count int) string {
	var text strings.Builder
	text.WriteString("4\n" + strconv.Itoa(count) + "\n")
	for i := 0; i < count; i++ {
		text.WriteString("0\n" + strconv.Itoa(i) + "\n")
	}
	return text.String()
}

func TestReadValueListsCarryNoSpareCapacity(t *testing.T) {
	for _, count := range []int{1, 3, 5, 33, maxTrustedListCount, maxTrustedListCount + 1, 3 * maxTrustedListCount} {
		database := &Database{Version: 17}
		value, err := database.readValue(bufio.NewReader(strings.NewReader(listText(count))))
		if err != nil {
			t.Fatalf("list of %d: %v", count, err)
		}
		elements := value.Elements()
		if len(elements) != count {
			t.Fatalf("list of %d read as %d elements", count, len(elements))
		}
		if cap(elements) != len(elements) {
			t.Errorf("list of %d holds capacity for %d", count, cap(elements))
		}
		for i, element := range elements {
			if element.Int() != int64(i) {
				t.Fatalf("list of %d: element %d = %v", count, i, element)
			}
		}
	}
}

func TestReadValueDoesNotAllocateATruncatedListsClaimedLength(t *testing.T) {
	database := &Database{Version: 17}
	_, err := database.readValue(bufio.NewReader(strings.NewReader("4\n1000000000000\n0\n7\n")))
	if err == nil {
		t.Fatal("a list cut short after one element was read without error")
	}
}

func TestReadValueSharesEmptyListsAndMaps(t *testing.T) {
	database := &Database{Version: 17}
	stream := bufio.NewReader(strings.NewReader("4\n0\n4\n0\n10\n0\n10\n0\n"))

	var values []types.Value
	for i := 0; i < 4; i++ {
		value, err := database.readValue(stream)
		if err != nil {
			t.Fatalf("value %d: %v", i, err)
		}
		values = append(values, value)
	}
	if values[0].Type() != types.TYPE_LIST || values[0].Len() != 0 || values[1].Len() != 0 {
		t.Fatalf("empty lists read as %v and %v", values[0], values[1])
	}
	if values[2].Type() != types.TYPE_MAP || values[2].Len() != 0 || values[3].Len() != 0 {
		t.Fatalf("empty maps read as %v and %v", values[2], values[3])
	}

	// Building on one loaded empty value must leave the others empty.
	grownList := values[0].Append(types.NewInt(1))
	grownMap := values[2].MapSet(types.NewStr("k"), types.NewInt(1))
	if grownList.Len() != 1 || grownMap.Len() != 1 {
		t.Fatalf("append and set on loaded empties gave %v and %v", grownList, grownMap)
	}
	if values[0].Len() != 0 || values[1].Len() != 0 || values[2].Len() != 0 || values[3].Len() != 0 {
		t.Fatalf("a loaded empty value changed: %v %v %v %v", values[0], values[1], values[2], values[3])
	}
}
