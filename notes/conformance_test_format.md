# Conformance Test Format Analysis

## Overview

The conformance tests use a **declarative YAML format** that is:
- Language/implementation agnostic
- Easy to port across implementations
- Self-documenting

Located at: `../cow_py/tests/conformance/`

## Structure

```
tests/conformance/
├── basic/          # Fundamental operations (arithmetic, string, list, etc.)
├── builtins/       # Built-in function tests
├── language/       # Language construct tests (loops, equality, etc.)
├── objects/        # Object system tests
└── server/         # Server-specific tests (exec, limits)
```

## YAML Schema

### Test Suite
```yaml
name: suite_name
description: "..."
version: "1.0"

requires:
  builtins: [random, sqrt]  # Required built-ins
  features: [maps, 64bit]   # Required server features

setup:      # Suite-level setup
  permission: wizard
  code: |
    $test_obj = create($nothing);

teardown:   # Suite-level teardown
  permission: wizard
  code: |
    recycle($test_obj);

tests:
  - name: test_name
    ...
```

### Test Case
```yaml
- name: test_name
  description: "..."
  permission: programmer  # or wizard
  skip: false            # or "reason string"
  skip_if: "condition"   # Conditional skip

  # ONE of:
  code: "expression"          # Wrapped in "return <expr>;"
  statement: |                # Multi-line, needs explicit return
    x = 5;
    return x * 2;
  verb: "#0:do_login"         # Verb call

  # Expected result - ONE of:
  expect:
    value: 42                 # Exact match
    error: E_TYPE             # Error code
    type: int                 # Type check
    match: "regex.*"          # Regex match
    contains: "needle"        # Contains check
    range: [1, 100]           # Numeric range
    notifications: ["msg"]    # notify() messages
```

## Key Patterns

### 1. Expression Tests (most common)
```yaml
- name: addition
  code: "1 + 1"
  expect:
    value: 2
```

### 2. Statement Tests
```yaml
- name: loop_sum
  statement: |
    sum = 0;
    for i in [1..10]
      sum = sum + i;
    endfor
    return sum;
  expect:
    value: 55
```

### 3. Error Tests
```yaml
- name: division_by_zero
  code: "1 / 0"
  expect:
    error: E_DIV
```

### 4. Object Relationship Tests
**CRITICAL**: Can't predict object numbers. Test relationships as booleans:
```yaml
- name: parent_relationship
  statement: |
    a = create($nothing);
    b = create(a);
    return parent(b) == a;
  expect:
    value: 1
```

## Go Implementation Considerations

### Test Runner Requirements
1. Parse YAML test files
2. Connect to MOO server or embedded interpreter
3. Execute MOO code
4. Compare results
5. Handle different expectation types

### Possible Go Libraries
- `gopkg.in/yaml.v3` for YAML parsing
- Custom expectation matchers

### Schema as Go Types
```go
type TestSuite struct {
    Name        string       `yaml:"name"`
    Description string       `yaml:"description"`
    Requires    Requirements `yaml:"requires"`
    Setup       *SetupBlock  `yaml:"setup"`
    Teardown    *SetupBlock  `yaml:"teardown"`
    Tests       []TestCase   `yaml:"tests"`
}

type TestCase struct {
    Name       string      `yaml:"name"`
    Code       string      `yaml:"code"`
    Statement  string      `yaml:"statement"`
    Verb       string      `yaml:"verb"`
    Permission string      `yaml:"permission"`
    Skip       interface{} `yaml:"skip"` // bool or string
    Expect     Expectation `yaml:"expect"`
}

type Expectation struct {
    Value    interface{} `yaml:"value"`
    Error    string      `yaml:"error"`
    Type     string      `yaml:"type"`
    Match    string      `yaml:"match"`
    Contains interface{} `yaml:"contains"`
    Range    []float64   `yaml:"range"`
}
```

## Benefits for Go MOO Development

1. **Spec-first**: Tests define expected behavior before implementation
2. **TDD-ready**: Implement features to make tests pass
3. **Compatibility**: Same tests for Python and Go implementations
4. **Documentation**: Tests serve as language specification
5. **Incremental**: Start with basic/, progress to builtins/, etc.

## Next Steps

1. Port YAML test runner to Go
2. Implement minimal interpreter to pass basic/arithmetic.yaml
3. Iterate through test categories
