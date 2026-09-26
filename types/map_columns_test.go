package types

import "testing"

func TestMapColumnsPreserveTraversalAndIsolation(t *testing.T) {
	waif := NewWaif(0, 0)
	for _, entries := range [][][2]Value{
		nil,
		{{NewStr("z"), NewInt(1)}, {NewStr("a"), NewList([]Value{NewInt(2)})}},
		{{NewInt(1), NewStr("int")}, {NewFloat(1), NewStr("float")}, {NewObj(1), NewStr("obj")}},
		{{waif, NewInt(1)}, {NewBool(true), NewInt(2)}, {NewBool(false), NewInt(3)}},
	} {
		m := NewMap(entries)
		pairs := m.Pairs()
		values, keys := m.MapColumns()
		if len(values) != len(pairs) || len(keys) != len(pairs) {
			t.Fatal("column lengths differ")
		}
		for i, pair := range pairs {
			if !keys[i].Equal(pair[0]) || !values[i].Equal(pair[1]) {
				t.Fatalf("column %d differs from map traversal", i)
			}
			keys[i], values[i] = None, None
		}
		again := m.Pairs()
		for i, pair := range pairs {
			if !again[i][0].Equal(pair[0]) || !again[i][1].Equal(pair[1]) {
				t.Fatal("column mutation changed source map")
			}
		}
	}
}
