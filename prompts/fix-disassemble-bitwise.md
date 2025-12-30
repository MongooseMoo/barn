# Task: Fix disassemble() to show actual bytecode opcodes

## Context
Barn's `disassemble()` builtin currently returns source code lines instead of actual bytecode disassembly. The tests expect opcode names like "BITAND", "BITOR", etc.

## Current Implementation (BROKEN)
`builtins/verbs.go` line ~580:
```go
// Return a list of disassembly lines
// For now, return the source code prefixed with line numbers
// A real implementation would show bytecode
lines := make([]types.Value, len(verb.Code))
for i, line := range verb.Code {
    lines[i] = types.NewStr(line)
}
```

## What's Available
- `vm/opcodes.go`: Has `OpcodeName` map with all opcode names (BITAND, BITOR, BITXOR, BITNOT, SHL, SHR, etc.)
- `vm/compiler.go`: Has `Compile()` function that compiles AST to bytecode
- `parser/parser.go`: Has `ParseProgram()` to parse source code

## Required Fix
1. Parse the verb source code to AST
2. Compile AST to bytecode using the compiler
3. Walk through bytecode and emit opcode names with operands
4. Return list of strings like: `{"PUSH 1", "PUSH 2", "BITAND", "POP"}`

## Test Commands
```bash
cd /c/Users/Q/code/barn
go build -o barn_test.exe ./cmd/barn/

./barn_test.exe -db Test.db -port 9650 > server_9650.log 2>&1 &

# Manual test
./moo_client.exe -port 9650 -timeout 5 -cmd "connect wizard" -cmd "; o = create(\$nothing); add_verb(o, {player, \"xd\", \"and\"}, {\"this\", \"none\", \"this\"}); set_verb_code(o, \"and\", {\"1 &. 2;\"}); return disassemble(o, \"and\");"

# Run conformance tests
cd /c/Users/Q/code/cow_py
uv run pytest tests/conformance/ --transport socket --moo-port 9650 -v -k "disassemble" -x
```

## Expected Output Format
Tests check: `expect: match: "BITAND"` - the output just needs to CONTAIN the opcode name.

## Key Files
- `builtins/verbs.go` - builtinDisassemble function
- `vm/opcodes.go` - OpcodeName map
- `vm/compiler.go` - Compiler and Compile function

## Output
Write findings and implementation to `./reports/fix-disassemble-bitwise.md`

## CRITICAL: File Modified Error Workaround
If Edit/Write fails with "file unexpectedly modified":
1. Read the file again with Read tool
2. Retry the Edit
3. Try path formats: `./relative`, `C:/forward/slashes`, `C:\back\slashes`
4. NEVER use cat, sed, echo - always Read/Edit/Write
