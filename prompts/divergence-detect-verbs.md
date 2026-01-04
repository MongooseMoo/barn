# Task: Detect Divergences in Verb Builtins

## Context

We need to verify Barn's verb builtin implementations match Toast (the reference) before updating the spec.

## Objective

Find behavioral differences between Barn and Toast for all verb builtins.

## Files to Read

- `spec/builtins/verbs.md` - expected behavior specification
- `builtins/verbs.go` - Barn implementation

## Reference

See `prompts/divergence-detect-template.md` for full instructions on report format and testing methodology.

## Key Builtins to Test

### Verb Queries
- `verbs()` - list verbs on object
- `verb_info()` - get verb metadata {owner, perms, names}
- `verb_args()` - get argument specifiers {dobj, prep, iobj}
- `verb_code()` - get verb source code

### Verb Definition
- `add_verb()` - create new verb
- `delete_verb()` - remove verb
- `set_verb_info()` - modify metadata
- `set_verb_args()` - modify arg specifiers
- `set_verb_code()` - set verb source

### Verb Invocation
- `call_function()` - call function by name
- `callers()` - get call stack
- `verb_location()` - where verb is defined

### Argument Specifiers
- Direct object: "this", "any", "none"
- Preposition: "with", "at", "to", etc.
- Indirect object: "this", "any", "none"

## Edge Cases to Test

- Non-existent verbs
- Permission violations
- Invalid verb names
- Verb inheritance
- Wildcard verb patterns (*verb)
- Empty verb code

## Testing Commands

```bash
# Toast oracle
./toast_oracle.exe 'verbs(#0)'

# Barn
./moo_client.exe -port 9500 -cmd "connect wizard" -cmd "; return verbs(#0);"

# Check conformance tests
grep -r "verbs\|verb_info\|add_verb\|verb_code" ~/code/moo-conformance-tests/src/moo_conformance/_tests/
```

## Output

Write your report to: `reports/divergence-verbs.md`

## CRITICAL

- Do NOT fix anything - only detect and report
- Do NOT edit spec - only report findings
- Test EVERY major verb builtin
- Pay special attention to permission checks and inheritance
- Flag behaviors with NO conformance test coverage
- Include exact test expressions and outputs
