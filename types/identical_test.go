package types

import (
	"math"
	"testing"
)

func TestIdentical(t *testing.T) {
	l := func(vs ...Value) Value { return NewList(vs) }
	m := func(pairs ...[2]Value) Value {
		v := NewMap(nil)
		for _, p := range pairs {
			v = v.MapSet(p[0], p[1])
		}
		return v
	}
	cases := []struct {
		name string
		a, b Value
		want bool
	}{
		{"int", NewInt(1), NewInt(1), true},
		{"int differs", NewInt(1), NewInt(2), false},
		{"int vs float", NewInt(1), NewFloat(1), false},
		{"float bits", NewFloat(1.5), NewFloat(1.5), true},
		{"neg zero", NewFloat(0), NewFloat(math.Copysign(0, -1)), false},
		{"str", NewStr("abc"), NewStr("abc"), true},
		{"str case", NewStr("abc"), NewStr("ABC"), false},
		{"obj", NewObj(5), NewObj(5), true},
		{"list", l(NewStr("a"), NewInt(1)), l(NewStr("a"), NewInt(1)), true},
		{"list case", l(NewStr("a")), l(NewStr("A")), false},
		{"list len", l(NewStr("a")), l(NewStr("a"), NewStr("a")), false},
		{"map", m([2]Value{NewStr("k"), NewInt(1)}), m([2]Value{NewStr("k"), NewInt(1)}), true},
		{"map val", m([2]Value{NewStr("k"), NewInt(1)}), m([2]Value{NewStr("k"), NewInt(2)}), false},
		{"map order", m([2]Value{NewStr("a"), NewInt(1)}, [2]Value{NewStr("b"), NewInt(2)}), m([2]Value{NewStr("b"), NewInt(2)}, [2]Value{NewStr("a"), NewInt(1)}), false},
	}
	for _, c := range cases {
		if got := c.a.Identical(c.b); got != c.want {
			t.Errorf("%s: Identical(%v, %v) = %v, want %v", c.name, c.a, c.b, got, c.want)
		}
		if c.want && !c.a.Equal(c.b) {
			t.Errorf("%s: Identical but not Equal", c.name)
		}
	}
}
