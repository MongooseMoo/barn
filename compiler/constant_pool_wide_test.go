package compiler

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/types"
)

// emitDistinctConstants pushes count distinct string constants.
func emitDistinctConstants(c *lowerer, count int) {
	for i := range count {
		c.emitConstant(types.NewStr(fmt.Sprintf("k%d", i)))
	}
}

// Programs with at most 256 constants must keep their bytes: a suspended task
// is restored only when recompiling its source gives the code it ran.
func TestConstantPushKeepsOneByteIndexThrough255(t *testing.T) {
	c := newLowerer(nil)
	emitDistinctConstants(c, 257)
	if c.err != nil {
		t.Fatalf("emitting 257 constants failed: %v", c.err)
	}
	var want []byte
	for i := range 256 {
		want = append(want, byte(bytecode.OP_PUSH), byte(i))
	}
	want = append(want, byte(bytecode.OP_PUSH_WIDE), 1, 0)
	if !bytes.Equal(c.program.Code, want) {
		t.Fatalf("code tail = %v, want %v", c.program.Code[len(c.program.Code)-7:], want[len(want)-7:])
	}
	if got := len(c.program.Constants); got != 257 {
		t.Fatalf("constant pool holds %d entries, want 257", got)
	}
}

func TestConstantPoolEndsAtTheSixteenBitIndex(t *testing.T) {
	c := newLowerer(nil)
	emitDistinctConstants(c, 65536)
	if c.err != nil {
		t.Fatalf("emitting 65536 constants failed: %v", c.err)
	}
	if tail := c.program.Code[len(c.program.Code)-3:]; !bytes.Equal(tail, []byte{byte(bytecode.OP_PUSH_WIDE), 0xff, 0xff}) {
		t.Fatalf("constant 65535 encoded as %v, want PUSH_WIDE 255 255", tail)
	}
	emitDistinctConstants(c, 65537)
	if c.err == nil || !strings.Contains(c.err.Error(), "too many constants (max 65536)") {
		t.Fatalf("constant 65536 error = %v, want too many constants (max 65536)", c.err)
	}
}

// The native-checkpoint import follows the lowerer's symbolic operand stack; a
// literal pushed through the wide form is still that literal.
func TestNativeImportLayoutReadsWidePushLiteral(t *testing.T) {
	c := newLowerer(nil)
	c.importLayout = newBarnImportLayout()
	emitDistinctConstants(c, 257)
	c.importLayout.flush(c)
	if c.err != nil || c.importLayout.err != nil {
		t.Fatalf("emitting 257 constants failed: %v, %v", c.err, c.importLayout.err)
	}
	stack := c.importLayout.stack
	if len(stack) != 257 {
		t.Fatalf("symbolic stack holds %d values, want 257", len(stack))
	}
	for _, index := range []int{255, 256} {
		value := stack[index]
		if want := fmt.Sprintf("k%d", index); !value.isLiteral || value.literal.Type() != types.TYPE_STR || value.literal.Str() != want {
			t.Fatalf("symbolic stack[%d] = %+v, want literal %q", index, value, want)
		}
	}
}
