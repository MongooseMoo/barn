package vm

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// Raw reported input (#1085): m = ["a" -> 1]; return m["a"..1]; Toast returns
// [] because compare() orders "a" (STR) after 1 (INT); Barn raised E_RANGE.
// Toast execute.cc OP_RANGE_REF checks list/map bounds first, then compares
// the bounds and returns [] when from sorts after to, whatever the keys.
func TestMapRangeReadOrdersBoundsByToastCompare(t *testing.T) {
	cases := []struct {
		code string
		want string
	}{
		{`m = ["a" -> 1]; return m["a"..1];`, `[]`},
		{`m = ["a" -> 1]; return m["z"..1];`, `[]`},
		{`m = ["a" -> 1]; return m["a"..#1];`, `[]`},
		{`return [0.5 -> 1]["b"..1];`, `[]`},
		{`return ["q" -> 0][true..1.5];`, `[]`},
		{`return ["q" -> 0][1.5..E_PERM];`, `[]`},
		{`return ["q" -> 0][E_PERM..#1];`, `[]`},
		{`return ["a" -> 1]["z".."b"];`, `[]`},
		{`return [1.5 -> 1][2.5..0.5];`, `[]`},
		{`return [#1 -> 1][#5..#2];`, `[]`},
		{`return [E_PERM -> 1][E_RANGE..E_TYPE];`, `[]`},
		{`return []["b".."a"];`, `[]`},
		{`return [][#1..1];`, `[]`},
		{`m = [1 -> "i", #1 -> "o", E_PERM -> "e", 1.5 -> "f", true -> "b", "a" -> "s"]; return m[#1..true];`, `[#1 -> "o", E_PERM -> "e", 1.5 -> "f", true -> "b"]`},
		{`m = ["a" -> 1, "B" -> 2]; return m["A".."b"];`, `["a" -> 1, "B" -> 2]`},
		{`m = ["a" -> 1]; return m["A".."A"];`, `["a" -> 1]`},
	}
	for _, tc := range cases {
		result := runBytecodeProgram(t, tc.code, nil, nil)
		if result.Flow != types.FlowReturn {
			t.Errorf("%s: flow = %v error %s, want %s", tc.code, result.Flow, result.Error, tc.want)
			continue
		}
		if got := result.Val.String(); got != tc.want {
			t.Errorf("%s = %s, want %s", tc.code, got, tc.want)
		}
	}
}

func TestMapRangeReadCheckOrder(t *testing.T) {
	cases := []struct {
		code string
		want types.ErrorCode
	}{
		{`m = ["a" -> 1]; return m[1.."a"];`, types.E_RANGE},
		{`return ["a" -> 1]["b".."z"];`, types.E_RANGE},
		{`return []["a".."b"];`, types.E_RANGE},
		{`return [][1..#1];`, types.E_RANGE},
		{`m = ["a" -> 1]; return m["b".."b"];`, types.E_RANGE},
		// A list or map bound raises E_TYPE before any key lookup.
		{`m = ["a" -> 1]; return m["zz"..{}];`, types.E_TYPE},
		{`m = ["a" -> 1]; return m["zz"..[]];`, types.E_TYPE},
		{`m = ["a" -> 1]; return m[5..{}];`, types.E_TYPE},
		{`return []["zz"..{}];`, types.E_TYPE},
	}
	for _, tc := range cases {
		result := runBytecodeProgram(t, tc.code, nil, nil)
		if result.Flow != types.FlowException || result.Error != tc.want {
			t.Errorf("%s: flow = %v error %s val %v, want %s", tc.code, result.Flow, result.Error, result.Val, tc.want)
		}
	}
}

// Raw reported input (#1085): m = ["a" -> 1]; m["a"..1] = 3; Toast raises
// E_TYPE: EOP_RANGESET checks the value type against the base type, then the
// bound types, before looking up either key. Barn raised E_RANGE.
func TestMapRangeAssignmentCheckOrder(t *testing.T) {
	cases := []struct {
		code string
		want types.ErrorCode
	}{
		{`m = ["a" -> 1]; m["a"..1] = 3;`, types.E_TYPE},
		{`m = ["a" -> 1]; m["a".."z"] = 3;`, types.E_TYPE},
		{`m = ["a" -> 1]; m["zz".."yy"] = {};`, types.E_TYPE},
		{`m = ["a" -> 1]; m["zz"..[]] = 5;`, types.E_TYPE},
		{`e = []; e[1..0] = 5;`, types.E_TYPE},
		{`m = ["a" -> 1]; m["zz"..{}] = ["k" -> 1];`, types.E_TYPE},
		{`m = ["a" -> 1]; m["zz"..[]] = ["k" -> 1];`, types.E_TYPE},
		{`m = ["a" -> 1]; m["zz".."a"] = ["k" -> 1];`, types.E_RANGE},
		{`m = ["a" -> 1]; m["a"..1] = ["k" -> 1];`, types.E_RANGE},
		{`e = []; e[1..0] = ["k" -> 1];`, types.E_RANGE},
	}
	for _, tc := range cases {
		result := runBytecodeProgram(t, tc.code, nil, nil)
		if result.Flow != types.FlowException || result.Error != tc.want {
			t.Errorf("%s: flow = %v error %s val %v, want %s", tc.code, result.Flow, result.Error, result.Val, tc.want)
		}
	}
}
