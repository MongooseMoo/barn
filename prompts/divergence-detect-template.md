# Divergence Detection Template

This template is used to verify Barn's implementation matches Toast (the reference server) for a specific spec area. Unlike the blind-implementor audit, you HAVE access to implementation code and CAN run tests.

## Output Location

**CRITICAL:** Write your divergence report to:
```
reports/divergence-{spec-name}.md
```

## Your Access

You MAY:
- Read spec files in `spec/`
- Read Barn implementation files (Go code)
- Run Toast oracle: `./toast_oracle.exe 'expression'`
- Run moo_client against Barn: `./moo_client.exe -port 9500 -cmd "connect wizard" -cmd "; return expression;"`
- Search conformance tests in `~/code/moo-conformance-tests/`

## Your Task

For each behavior documented in the spec:
1. Identify testable behaviors (functions, edge cases, error conditions)
2. Test each against Toast oracle
3. Test same expression against Barn
4. Compare outputs
5. Classify divergences
6. Identify test coverage gaps

## Report Format

```markdown
# Divergence Report: {spec_name}

**Spec File**: {path}
**Barn Files**: {paths}
**Status**: divergences_found | clean | needs_investigation
**Date**: {date}

## Summary

{brief overview - number of behaviors tested, divergences found, gaps identified}

## Divergences

### 1. {builtin}({args}) - {description}

| Field | Value |
|-------|-------|
| Test | `{expression}` |
| Barn | {barn output} |
| Toast | {toast output} |
| Classification | likely_barn_bug / likely_spec_gap / needs_human_decision |
| Evidence | {why you classified it this way} |

### 2. ...

## Test Coverage Gaps

Behaviors documented in spec but NOT covered by conformance tests:

- `{function}({edge_case})` - {description}
- ...

## Behaviors Verified Correct

{List behaviors that match between Barn and Toast}
```

## Classification Guidelines

- **likely_barn_bug**: Toast is the reference, Barn clearly deviates from expected behavior
- **likely_spec_gap**: Both servers behave identically but spec doesn't document this behavior
- **needs_human_decision**: Ambiguous case, both behaviors could be valid, or unclear which is correct

## CRITICAL Rules

1. Do NOT fix anything - only detect and report
2. Do NOT edit spec - only report findings
3. Test EVERY testable behavior mentioned in spec
4. Flag behaviors with NO conformance test coverage
5. Be thorough - test edge cases (zero, negative, empty, max values, type mismatches)
6. Always show exact test expressions used

## Invoking the Oracles

### Toast Oracle (reference)
```bash
./toast_oracle.exe 'sqrt(-1)'
# Returns: E_INVARG
```

### Barn (implementation being tested)
```bash
./moo_client.exe -port 9500 -cmd "connect wizard" -cmd "; return sqrt(-1);"
# Returns output from Barn
```

### Finding Conformance Tests
```bash
# Check if a behavior has test coverage
grep -r "sqrt" ~/code/moo-conformance-tests/src/moo_conformance/_tests/
```

## Begin

When given a specific spec file to audit, follow this process thoroughly and write your complete report to the reports/ directory.
