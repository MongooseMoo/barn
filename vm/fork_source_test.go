package vm

import (
	"barn/parser"
	"testing"
)

func TestBytecodeForkSourceLines(t *testing.T) {
	source := "fork (4)\n  #0.audit_restart_task_seen = \"after\";\nendfork\nreturn 0;"
	p := parser.NewParser(source)
	stmts, err := p.ParseProgram()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	c := NewCompilerWithRegistry(BuildVMRegistry())
	prog, err := c.CompileStatements(stmts)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	prog.Source = []string{
		"fork (4)",
		"  #0.audit_restart_task_seen = \"after\";",
		"endfork",
		"return 0;",
	}

	for ip := 0; ip < len(prog.Code); ip++ {
		if OpCode(prog.Code[ip]) != OP_FORK {
			continue
		}
		bodyLen := int(prog.Code[ip+2])<<8 | int(prog.Code[ip+3])
		lines := prog.ForkSourceLines(ip+4, bodyLen)
		if len(lines) != 1 || lines[0] != "#0.audit_restart_task_seen = \"after\";" {
			t.Fatalf("source lines = %#v", lines)
		}
		return
	}
	t.Fatal("OP_FORK not found")
}

func TestBytecodeForkSourceLinesFromFlattenedEval(t *testing.T) {
	source := "try add_property(#0, \"audit_restart_task_seen\", \"before\", {#0, \"rw\"}); except (E_INVARG) #0.audit_restart_task_seen = \"before\"; endtry fork (4) #0.audit_restart_task_seen = \"after\"; endfork return #0.audit_restart_task_seen;"
	p := parser.NewParser(source)
	stmts, err := p.ParseProgram()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	c := NewCompilerWithRegistry(BuildVMRegistry())
	prog, err := c.CompileStatements(stmts)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	prog.Source = []string{source}

	for ip := 0; ip < len(prog.Code); ip++ {
		if OpCode(prog.Code[ip]) != OP_FORK {
			continue
		}
		bodyLen := int(prog.Code[ip+2])<<8 | int(prog.Code[ip+3])
		lines := prog.ForkSourceLines(ip+4, bodyLen)
		if len(lines) != 1 || lines[0] != "#0.audit_restart_task_seen = \"after\";" {
			t.Fatalf("source lines = %#v", lines)
		}
		return
	}
	t.Fatal("OP_FORK not found")
}
