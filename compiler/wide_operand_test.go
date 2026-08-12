package compiler

import (
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/MongooseMoo/barn/bytecode"
)

func TestWideControlFlowOperandBoundaryRoundTrips(t *testing.T) {
	for _, value := range []int{65535, 65536} {
		t.Run(fmt.Sprint(value), func(t *testing.T) {
			lowerer := &lowerer{program: &bytecode.Program{}}
			lowerer.emitWideInt(value)
			if lowerer.err != nil {
				t.Fatalf("emitWideInt(%d) failed: %v", value, lowerer.err)
			}
			if got := int(binary.BigEndian.Uint32(lowerer.program.Code)); got != value {
				t.Errorf("wide operand = %d, want %d", got, value)
			}
		})
	}
}

func TestWideForkBodyLengthBoundaryExtractsCompleteBody(t *testing.T) {
	for _, bodyLen := range []int{65535, 65536} {
		t.Run(fmt.Sprint(bodyLen), func(t *testing.T) {
			lowerer := &lowerer{program: &bytecode.Program{Code: []byte{byte(bytecode.OP_FORK_WIDE), 0}}}
			lowerer.emitWideInt(bodyLen)
			padding := make([]byte, bodyLen)
			for index := range padding {
				padding[index] = byte(bytecode.OP_RETURN_NONE)
			}
			lowerer.program.Code = append(lowerer.program.Code, padding...)
			if lowerer.err != nil {
				t.Fatalf("encoding fork length %d failed: %v", bodyLen, lowerer.err)
			}

			const bodyIP = 6 // opcode + var index + uint32 body length
			extracted := lowerer.program.ExtractForkBody(bodyIP, bodyLen)
			if extracted == nil {
				t.Fatalf("ExtractForkBody rejected body length %d", bodyLen)
			}
			if got := len(extracted.Code); got != bodyLen+1 {
				t.Errorf("extracted code length = %d, want %d", got, bodyLen+1)
			}
		})
	}
}
