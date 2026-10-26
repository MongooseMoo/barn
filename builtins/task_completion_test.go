package builtins

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func TestQueueInfoReportsLastInputTaskID(t *testing.T) {
	ctx := ctxWithConnManager(&stubConnManager{conn: &stubConn{lastInputTaskID: 913}})
	ctx.IsWizard = true
	ctx.Player = 2
	wireTestTaskHost(ctx)
	result := admitTestBuiltin("queue_info", ctx, []types.Value{types.NewObj(2)})
	if !result.IsNormal() {
		t.Fatal(result.Error)
	}
	got, ok := result.Val.MapGet(types.NewStr("last_input_task_id"))
	if !ok || got.Type() != types.TYPE_INT || got.Int() != 913 {
		t.Fatalf("queue_info last_input_task_id = %v, %t; want 913", got, ok)
	}
}
