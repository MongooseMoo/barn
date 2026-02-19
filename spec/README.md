# Barn MOO Language Specification

This directory contains the formal specification for the MOO programming language as implemented in Barn (Go MOO server).

## Documents

| Document | Description | Status |
|----------|-------------|--------|
| [grammar.md](grammar.md) | EBNF grammar, operator precedence | ✅ Complete |
| [types.md](types.md) | Type system, coercion rules | ✅ Complete |
| [operators.md](operators.md) | Operator semantics | ✅ Complete |
| [statements.md](statements.md) | Control flow statements | ✅ Complete |
| [errors.md](errors.md) | Error codes and conditions | ✅ Complete |
| [objects.md](objects.md) | Object model, inheritance | ✅ Complete |
| [tasks.md](tasks.md) | Task model, concurrency | ✅ Complete |
| [vm.md](vm.md) | VM architecture, opcodes | ✅ Complete |
| [go-design.md](go-design.md) | Go implementation notes | ✅ Complete |
| [builtins/](builtins/) | Built-in function specs | ✅ Complete |

## Built-in Function Categories

| Category | File | Functions |
|----------|------|-----------|
| Type conversion | [builtins/types.md](builtins/types.md) | typeof, tostr, toint, etc. |
| Math | [builtins/math.md](builtins/math.md) | abs, min, max, random, sin, cos, sqrt, etc. |
| Strings | [builtins/strings.md](builtins/strings.md) | length, strsub, index, match, explode, etc. |
| Lists | [builtins/lists.md](builtins/lists.md) | listappend, listdelete, sort, setadd, etc. |
| Maps | [builtins/maps.md](builtins/maps.md) | mapkeys, mapvalues, mapdelete, etc. |
| Objects | [builtins/objects.md](builtins/objects.md) | create, recycle, valid, parent, move, etc. |
| Properties | [builtins/properties.md](builtins/properties.md) | properties, property_info, add_property, etc. |
| Verbs | [builtins/verbs.md](builtins/verbs.md) | verbs, verb_info, verb_code, add_verb, etc. |
| Tasks | [builtins/tasks.md](builtins/tasks.md) | task_id, suspend, resume, kill_task, etc. |
| Time | [builtins/time.md](builtins/time.md) | time, ctime, strftime, etc. |
| JSON | [builtins/json.md](builtins/json.md) | parse_json, generate_json |
| File I/O | [builtins/fileio.md](builtins/fileio.md) | file_open, file_read, file_write, etc. |
| Network | [builtins/network.md](builtins/network.md) | notify, read, curl, etc. |
| Crypto | [builtins/crypto.md](builtins/crypto.md) | string_hash, bcrypt, encrypt, etc. |
| Regex | [builtins/regex.md](builtins/regex.md) | pcre_match, pcre_replace, etc. |
| SQLite | [builtins/sqlite.md](builtins/sqlite.md) | sqlite_open, sqlite_execute, etc. |
| Exec | [builtins/exec.md](builtins/exec.md) | exec, exec_async |

## Principles

1. **Spec-first**: Every feature documented before implemented
2. **Test-driven**: Every spec item has corresponding tests
3. **Rigorous**: Formal enough to resolve ambiguities
4. **Practical**: Examples for every construct

## Conformance Tests

Conformance tests live in `../moo-conformance-tests/` and are executed via the
`moo-conformance` CLI.

```powershell
# Run full suite with managed server lifecycle (recommended)
go build -o barn.exe ./cmd/barn/
uv run --project ..\moo-conformance-tests moo-conformance --server-command "C:/Users/Q/code/barn/barn.exe -db {db} -port {port}"
```

## Sources

- [LambdaMOO Programmer's Manual](https://www.hayseed.net/MOO/manuals/ProgrammersManual.html)
- [ToastStunt Documentation](https://github.com/lisdude/toaststunt-documentation)
- [moo_interp](../moo_interp/) - Python reference implementation
- [ToastStunt](../mongoose/toaststunt/) - C++ reference implementation
