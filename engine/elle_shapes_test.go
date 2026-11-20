package engine

import (
	"fmt"
	"testing"
)

// Each first attempt reads the row its predecessor will append to, before any
// append can begin. If all four commit without validating those reads, their
// observations form a four-transaction anti-dependency cycle.
func elleRing(t *testing.T, rt *Runtime, id int64) int64 {
	t.Helper()
	var setup, final string
	for row := 0; row < 4; row++ {
		setup += fmt.Sprintf(`add_property(#0,"elle_ring_%d",{},{#0,"rw"});`, row)
		final += fmt.Sprintf(`elle_observe("r","ring-%d",#0.("elle_ring_%d"));`, row, row)
	}
	elleDrain(t, rt, []*Task{elleTask(t, rt, id, setup)})
	id++
	var batch []*Task
	for row := 0; row < 4; row++ {
		source := fmt.Sprintf(`elle_observe("r","ring-%d",#0.("elle_ring_%d")); elle_barrier(100); v=#0.("elle_ring_%d"); elle_observe("r","ring-%d",v); #0.("elle_ring_%d")={@v,%d}; elle_observe("append","ring-%d",%d);`, (row+3)%4, (row+3)%4, row, row, row, 800+row, row, 800+row)
		batch = append(batch, elleTask(t, rt, id, source))
		id++
	}
	elleDrain(t, rt, batch)
	elleDrain(t, rt, []*Task{elleTask(t, rt, id, final)})
	return id + 1
}

func elleRegisterRead(kind int) string {
	switch kind {
	case 0:
		return `elle_observe("r","property",#30.data);`
	case 1:
		return `elle_observe("r","name",toint(#32.name));`
	default:
		return `lines=verb_code(#31,1); line=lines[1]; elle_observe("r","verb-code",toint(line[1..$-1]));`
	}
}

func elleRegisterWrite(kind, token int) string {
	switch kind {
	case 0:
		return fmt.Sprintf(`#30.data=%d; elle_observe("w","property",%d);`, token, token)
	case 1:
		return fmt.Sprintf(`#32.name="%d"; elle_observe("w","name",%d);`, token, token)
	default:
		return fmt.Sprintf(`set_verb_code(#31,1,{"%d;"}); elle_observe("w","verb-code",%d);`, token, token)
	}
}
