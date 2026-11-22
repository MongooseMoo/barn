package compiler

import (
	"testing"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/types"
)

// OP_CHECK_ARGS reads the arguments in place, so the native-checkpoint
// import's symbolic stack keeps each argument's origin across it.
func TestNativeImportLayoutKeepsArgumentsAcrossCheckArgs(t *testing.T) {
	c := newLowerer(nil)
	c.importLayout = newBarnImportLayout()
	emitDistinctConstants(c, 3)
	c.emit(bytecode.OP_CHECK_ARGS)
	c.emitByte(2)
	c.importLayout.flush(c)
	if c.err != nil || c.importLayout.err != nil {
		t.Fatalf("emitting failed: %v, %v", c.err, c.importLayout.err)
	}
	stack := c.importLayout.stack
	if len(stack) != 3 {
		t.Fatalf("symbolic stack holds %d values, want 3", len(stack))
	}
	for index, want := range []string{"k0", "k1", "k2"} {
		value := stack[index]
		if !value.isLiteral || value.literal.Type() != types.TYPE_STR || value.literal.Str() != want {
			t.Fatalf("symbolic stack[%d] = %+v, want literal %q", index, value, want)
		}
	}
}
