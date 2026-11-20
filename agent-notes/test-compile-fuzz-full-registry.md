# Compiler fuzz with the full builtin registry

## Goal
Item 3 of the fuzzing expansion Q approved 2026-11-01 (compile-time limits;
plan in barn master checkout agent-notes/fuzzing-expansion.md).

## State
compiler/FuzzCompileVerifiable now compiles with
`builtins.NewRegistryFromDescriptors(config.Core, vm.Descriptors())`.
Seeds plus the local 11,006-verb corpus (BARN_FUZZ_CORPUS) replay and pass in
87 s.

## Evidence
- With `builtins.NewRegistry()` (the old target), 639 corpus verbs fail on
  "Unknown built-in function: pass" and 23 on "eval": they were skipped before
  verification. With the VM descriptors, only optional-capability builtins
  (network, sqlite, curl; excluded from config.Core) and 14 real syntax
  failures remain.
- Compile time is about linear: a throwaway probe compiled 2,000 / 8,000 /
  32,000-unit inputs of eight shapes; 4x input took 1.3-3.4x time; slowest
  352 KB of statements in 151 ms.
- Toast has no program-size limit (code_gen.cc grows buffers; parser.y limits
  only EXCEPT clause count and scatter targets), so Barn adds none.
- Go's fuzzer already reports a hung input, so no per-input time bound is
  added.

## Surprises
- 14 Mongoose verbs fail to compile on Barn but use syntax Toast's grammar
  accepts: `except e ("STRING")` (parser.y:667 codes = ne_arglist), a catch
  default that is an assignment (parser.y:674 default = tARROW expr), and two
  "Illegal expression on left side of assignment" cases. Reported to Q as a
  separate conformance item; not changed here.
- Barn caps constants at 255 (compiler/lower_emit.go:114) and locals at 256
  (bytecode/program.go:10); Toast has neither cap. No corpus verb reaches them.

## Next step
PR CI.
