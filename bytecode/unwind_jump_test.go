package bytecode_test

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/compiler"
)

func TestUnwindJumpForkRebasingAndValidation(t *testing.T) {
	program, diagnostics := compiler.New(map[string]int{}).CompileMOO([]string{
		"try fork(0) for i in [1..2] try continue; finally x=1; endtry endfor endfork finally y=1; endtry return 0;",
	})
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	if err := bytecode.VerifyProgram(program); err != nil {
		t.Fatal(err)
	}
	var body *bytecode.Program
	for ip, op := range program.Code {
		if bytecode.OpCode(op) == bytecode.OP_FORK_LOCAL_WIDE && program.IsInstructionBoundary(ip) {
			body = program.ExtractForkBody(ip+7, int(binary.BigEndian.Uint32(program.Code[ip+3:ip+7])))
			break
		}
	}
	if body == nil {
		t.Fatal("fork body missing")
	}
	if err := bytecode.VerifyProgram(body); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(bytecode.Disassemble(body), "\n"), "UNWIND_JUMP") {
		t.Fatal("unwind jump not disassembled")
	}
	for ip, op := range body.Code {
		if bytecode.OpCode(op) == bytecode.OP_UNWIND_JUMP && body.IsInstructionBoundary(ip) {
			invalid := *body
			invalid.Code = append([]byte(nil), body.Code...)
			binary.BigEndian.PutUint32(invalid.Code[ip+1:ip+5], uint32(ip+1))
			if err := bytecode.VerifyProgram(&invalid); err == nil {
				t.Fatal("operand target accepted")
			}
			return
		}
	}
	t.Fatal("no unwind jump in extracted body")
}

func TestUnwindJumpCannotCreateTickFreeCycle(t *testing.T) {
	program := &bytecode.Program{Code: []byte{byte(bytecode.OP_UNWIND_JUMP), 0, 0, 0, 0, 0, 0, byte(bytecode.OP_RETURN_NONE)}}
	if err := bytecode.VerifyProgram(program); err == nil || !strings.Contains(err.Error(), "charges no tick") {
		t.Fatalf("tick-free unwind loop result: %v", err)
	}
}
