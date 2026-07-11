# Toast Disassemble Oracle

Verified against the documented WSL oracle on 2026-07-11:

- Binary: `/root/src/toaststunt/build-release/moo` (ToastStunt 2.7.3_5)
- Database: `/root/src/toastcore/toastcore.db`
- Expression: `disassemble(#0, 1)` and the equivalent
  `disassemble(#0, "do_login_command")`
- Both forms returned identical lists.

The returned shape is:

1. `Language version number: 17`
2. `First line number: 1`
3. An empty separator string
4. `Main code vector:`
5. `=================`
6. Bytecode metadata lines
7. One row per decoded instruction containing the byte offset, encoded opcode
   and operand bytes, and an opcode mnemonic

Representative rows from Toast:

```text
  0: 133 000               PUSH_LITERAL "...This code should only be run as a server task..."
  2: 144                   POP
  4: 012 018             * CALL_FUNC callers
 10: 141                   RETURN
```

Barn uses a different bytecode instruction set, so conformance means returning
this metadata-and-decoded-row shape over Barn's actual compiled
`bytecode.Program`. It does not mean relabeling Barn instructions as Toast
opcodes or walking semantic IR to synthesize pseudo-instructions.

## Boundary-context follow-up

After the final managed suite exposed missing operator mnemonics, the same WSL
oracle was queried for map boundary behavior using the repository's disposable
`cmd/toast_oracle` flow. Toast returned:

```text
["b" -> 2, "a" -> 1][^..$]  => ["a" -> 1, "b" -> 2]
["b" -> 2, "a" -> 1][^]     => 1
["b" -> 2, "a" -> 1][$]     => 2
[10 -> "a", 20 -> "b"][^..$] => [10 -> "a", 20 -> "b"]
```

This distinguishes two bytecode contexts: map indexing resolves first/last to
map keys, while map ranges resolve first/last to positional `1` and map length.
Barn therefore retains the context in the operand of its actual index-marker
instruction; disassembly renders both operand forms as `FIRST` or `LAST`.
