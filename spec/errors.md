# MOO Error Codes Specification

## Overview

MOO has 18 standard error codes. Each error has a numeric value, a symbolic name, and specific conditions that trigger it.

---

## 1. Error Code Table

| Code | Name | Value | Category |
|------|------|-------|----------|
| E_NONE | No error | 0 | Success |
| E_TYPE | Type mismatch | 1 | Type |
| E_DIV | Division by zero | 2 | Arithmetic |
| E_PERM | Permission denied | 3 | Security |
| E_PROPNF | Property not found | 4 | Object |
| E_VERBNF | Verb not found | 5 | Object |
| E_VARNF | Variable not found | 6 | Runtime |
| E_INVIND | Invalid index | 7 | Object |
| E_RECMOVE | Recursive move | 8 | Object |
| E_MAXREC | Max recursion | 9 | Runtime |
| E_RANGE | Range error | 10 | Collection |
| E_ARGS | Wrong argument count | 11 | Call |
| E_NACC | Not accessible | 12 | Security |
| E_INVARG | Invalid argument | 13 | Call |
| E_QUOTA | Quota exceeded | 14 | Resource |
| E_FLOAT | Float error | 15 | Arithmetic |
| E_FILE | File error | 16 | I/O |
| E_EXEC | Exec error | 17 | I/O |

---

## 2. Detailed Error Specifications

### 2.1 E_NONE (0) - No Error

**Description:** Success indicator, not typically raised.

**When returned:**
- Default return when no error occurs
- Explicit return from error-checking functions

---

### 2.2 E_TYPE (1) - Type Mismatch

**Description:** Operation received wrong type of value.

**Trigger conditions:**

| Context | Example | Trigger |
|---------|---------|---------|
| Arithmetic | `"a" + 1` | String + Int |
| Arithmetic | `{} - 1` | List - Int |
| Bitwise | `1.5 \|. 2` | Float in bitwise |
| Comparison | `"a" < 1` | String < Int |
| Index | `list["key"]` | Non-int list index |
| Property | `123.name` | Property on non-object |
| Verb call | `"str":verb()` | Verb on non-object |
| Builtin | `length(42)` | Wrong arg type |

**Test cases needed:**
```yaml
- name: type_error_arithmetic
  code: '"hello" + 42'
  expect:
    error: E_TYPE

- name: type_error_bitwise
  code: '1.5 |. 2'
  expect:
    error: E_TYPE

- name: type_error_comparison
  code: '"a" < 1'
  expect:
    error: E_TYPE
```

---

### 2.3 E_DIV (2) - Division by Zero

**Description:** Division or modulo by zero.

**Trigger conditions:**

| Operation | Example |
|-----------|---------|
| Integer division | `5 / 0` |
| Float division | `5.0 / 0.0` |
| Integer modulo | `5 % 0` |

**Note:** Float division by zero may return Infinity instead of raising E_DIV in some implementations.

**Test cases needed:**
```yaml
- name: div_by_zero_int
  code: '5 / 0'
  expect:
    error: E_DIV

- name: div_by_zero_float
  code: '5.0 / 0.0'
  expect:
    error: E_DIV

- name: mod_by_zero
  code: '5 % 0'
  expect:
    error: E_DIV
```

---

### 2.4 E_PERM (3) - Permission Denied

**Description:** Caller lacks required permissions.

**Trigger conditions:**

| Context | Condition |
|---------|-----------|
| Property read | Property not readable by caller |
| Property write | Property not writable by caller |
| Verb call | Verb not executable by caller |
| Object modification | Caller not owner or wizard |
| Create | Caller can't create children |
| Recycle | Caller not owner or wizard |

**Permission model:**
- Owner: Full access to owned objects
- Wizard: Full access to everything
- Programmer: Can run code
- User: Limited to granted permissions

**Test cases needed:**
```yaml
- name: perm_read_property
  permission: player
  code: 'wizard_obj.secret_prop'
  expect:
    error: E_PERM

- name: perm_write_property
  permission: player
  statement: |
    wizard_obj.read_only = 1;
  expect:
    error: E_PERM
```

---

### 2.5 E_PROPNF (4) - Property Not Found

**Description:** Requested property doesn't exist on object.

**Trigger conditions:**

| Context | Example |
|---------|---------|
| Property read | `obj.nonexistent` |
| Property write | `obj.nonexistent = 1` (if not adding) |
| property_info() | `property_info(obj, "missing")` |

**Note:** Property lookup searches the inheritance chain.

**Test cases needed:**
```yaml
- name: propnf_access
  code: '#0.definitely_not_a_property'
  expect:
    error: E_PROPNF

- name: propnf_info
  code: 'property_info(#0, "missing")'
  expect:
    error: E_PROPNF
```

---

### 2.6 E_VERBNF (5) - Verb Not Found

**Description:** Requested verb doesn't exist on object.

**Trigger conditions:**

| Context | Example |
|---------|---------|
| Verb call | `obj:nonexistent()` |
| verb_info() | `verb_info(obj, "missing")` |
| verb_code() | `verb_code(obj, "missing")` |

**Note:** Verb lookup searches the inheritance chain.

**Test cases needed:**
```yaml
- name: verbnf_call
  code: '#0:definitely_not_a_verb()'
  expect:
    error: E_VERBNF

- name: verbnf_info
  code: 'verb_info(#0, "missing")'
  expect:
    error: E_VERBNF
```

---

### 2.7 E_VARNF (6) - Variable Not Found

**Description:** Referenced variable is not defined.

**Trigger conditions:**

| Context | Example |
|---------|---------|
| Variable read | Using undefined variable |
| eval() | Evaluating code with undefined var |

**Note:** This is typically a compile-time error in verb definitions but can occur with eval().

**Test cases needed:**
```yaml
- name: varnf_eval
  code: 'eval("return undefined_var;")'
  expect:
    value: {0, E_VARNF}
```

---

### 2.8 E_INVIND (7) - Invalid Index

**Description:** Object reference is invalid (recycled or never existed).

**Trigger conditions:**

| Context | Example |
|---------|---------|
| Property access | `#99999.name` (if #99999 doesn't exist) |
| Verb call | `#99999:verb()` |
| valid() check | After recycling |
| Builtin | `parent(#99999)` |

**Test cases needed:**
```yaml
- name: invind_property
  code: '#99999.name'
  expect:
    error: E_INVIND

- name: invind_verb
  code: '#99999:test()'
  expect:
    error: E_INVIND
```

---

### 2.9 E_RECMOVE (8) - Recursive Move

**Description:** Attempted to move object into itself or descendant.

**Trigger conditions:**

| Context | Example |
|---------|---------|
| move() | `move(obj, obj)` |
| move() | `move(obj, child_of_obj)` |

**Test cases needed:**
```yaml
- name: recmove_self
  permission: wizard
  statement: |
    obj = create($nothing);
    move(obj, obj);
  expect:
    error: E_RECMOVE
```

---

### 2.10 E_MAXREC (9) - Maximum Recursion

**Description:** Call stack depth exceeded limit.

**Trigger conditions:**
- Verb calling itself too deeply
- Mutual recursion exceeding limit
- Stack overflow

**Default limit:** Typically 50-100 levels (server-configurable)

**Test cases needed:**
```yaml
- name: maxrec_direct
  statement: |
    // Requires setup of recursive verb
    recursive_verb();
  expect:
    error: E_MAXREC
```

---

### 2.11 E_RANGE (10) - Range Error

**Description:** Index or value out of valid range.

**Trigger conditions:**

| Context | Example |
|---------|---------|
| List index | `list[0]` (1-based) |
| List index | `list[length+1]` |
| String index | `str[0]` |
| Sublist range | `list[5..3]` (end < start) |
| Builtin range | `random(0)` |

**Test cases needed:**
```yaml
- name: range_list_zero
  code: '{1,2,3}[0]'
  expect:
    error: E_RANGE

- name: range_list_overflow
  code: '{1,2,3}[4]'
  expect:
    error: E_RANGE

- name: range_string_zero
  code: '"abc"[0]'
  expect:
    error: E_RANGE

- name: range_negative
  code: 'random(-1)'
  expect:
    error: E_RANGE
```

---

### 2.12 E_ARGS (11) - Wrong Argument Count

**Description:** Function/verb called with wrong number of arguments.

**Trigger conditions:**

| Context | Example |
|---------|---------|
| Too few args | `length()` |
| Too many args | `length(a, b)` |
| Verb mismatch | Verb expects different arity |

**Test cases needed:**
```yaml
- name: args_too_few
  code: 'length()'
  expect:
    error: E_ARGS

- name: args_too_many
  code: 'length("a", "b")'
  expect:
    error: E_ARGS
```

---

### 2.13 E_NACC (12) - Not Accessible

**Description:** Value cannot be accessed in current context.

**Trigger conditions:**

| Context | Example |
|---------|---------|
| Clear property | Reading cleared property |
| Protected value | Accessing protected data |

**Less common than E_PERM; specifically for value accessibility.**

---

### 2.14 E_INVARG (13) - Invalid Argument

**Description:** Argument value is invalid for the operation.

**Trigger conditions:**

| Context | Example |
|---------|---------|
| Invalid parent | `create(recycled_obj)` |
| Invalid mode | `file_open(path, "invalid")` |
| Invalid format | `parse_json("not json")` |
| Out of domain | `sqrt(-1)` (returns NaN or error) |

**Test cases needed:**
```yaml
- name: invarg_create
  permission: wizard
  code: 'create(#-1)'
  expect:
    error: E_INVARG

- name: invarg_json
  code: 'parse_json("not valid json")'
  expect:
    error: E_INVARG
```

---

### 2.15 E_QUOTA (14) - Quota Exceeded

**Description:** Resource quota limit reached.

**Trigger conditions:**

| Context | Example |
|---------|---------|
| Object creation | Too many objects owned |
| Property creation | Too many properties |
| Memory | Allocation limit exceeded |

**Test cases needed:**
```yaml
- name: quota_objects
  permission: player
  statement: |
    // Create objects until quota hit
    while (1)
      create($nothing);
    endwhile
  expect:
    error: E_QUOTA
```

---

### 2.16 E_FLOAT (15) - Float Error

**Description:** Invalid floating-point operation.

**Trigger conditions:**

| Context | Example |
|---------|---------|
| NaN result | `0.0 / 0.0` |
| Infinity overflow | `1e308 * 10` |
| Domain error | `asin(2.0)` |

**Note:** Some operations return NaN/Infinity instead of raising E_FLOAT.

**Test cases needed:**
```yaml
- name: float_nan
  code: '0.0 / 0.0'
  expect:
    error: E_FLOAT  # or check for NaN

- name: float_domain
  code: 'asin(2.0)'
  expect:
    error: E_FLOAT
```

---

### 2.17 E_FILE (16) - File Error

**Description:** File I/O operation failed.

**Trigger conditions:**

| Context | Example |
|---------|---------|
| File not found | `file_open("missing.txt", "r")` |
| Permission denied | Opening protected file |
| Read error | Corrupted file |
| Write error | Disk full |

**Test cases needed:**
```yaml
- name: file_not_found
  code: 'file_open("/nonexistent/path", "r")'
  expect:
    error: E_FILE
```

---

### 2.18 E_EXEC (17) - Exec Error

**Description:** External process execution failed.

**Trigger conditions:**

| Context | Example |
|---------|---------|
| Command not found | `exec({"nonexistent_cmd"})` |
| Execution failed | Process couldn't start |
| Timeout | Process exceeded time limit |

**Test cases needed:**
```yaml
- name: exec_not_found
  code: 'exec({"definitely_not_a_command"})'
  expect:
    error: E_EXEC
```

---

## 3. Error Handling

### 3.1 Try/Except

```moo
try
  risky_operation();
except e (E_TYPE, E_RANGE)
  // Handle specific errors
except (ANY)
  // Handle all other errors
endtry
```

### 3.2 Catch Expression

```moo
result = `risky() ! E_TYPE => default_value`;
```

### 3.3 Error Propagation

Unhandled errors propagate up the call stack until:
1. Caught by try/except or catch expression
2. Reach top level (task aborts)

---

## 4. Error Testing Matrix

For comprehensive coverage, each error code should have tests for:

| Category | Test Type |
|----------|-----------|
| Direct trigger | Operation that raises error |
| Catch handling | Error caught and handled |
| Propagation | Error bubbles up correctly |
| Edge cases | Boundary conditions |

**Target: 5+ tests per error code = 90+ error tests**

---

## 5. Go Implementation

```go
type ErrorCode int

const (
    E_NONE   ErrorCode = 0
    E_TYPE   ErrorCode = 1
    E_DIV    ErrorCode = 2
    E_PERM   ErrorCode = 3
    E_PROPNF ErrorCode = 4
    E_VERBNF ErrorCode = 5
    E_VARNF  ErrorCode = 6
    E_INVIND ErrorCode = 7
    E_RECMOVE ErrorCode = 8
    E_MAXREC ErrorCode = 9
    E_RANGE  ErrorCode = 10
    E_ARGS   ErrorCode = 11
    E_NACC   ErrorCode = 12
    E_INVARG ErrorCode = 13
    E_QUOTA  ErrorCode = 14
    E_FLOAT  ErrorCode = 15
    E_FILE   ErrorCode = 16
    E_EXEC   ErrorCode = 17
)

var errorNames = map[ErrorCode]string{
    E_NONE:   "E_NONE",
    E_TYPE:   "E_TYPE",
    // ... etc
}

func (e ErrorCode) String() string {
    return errorNames[e]
}
```

---

## 6. Error Message Guidelines

When raising errors, include context:

```go
// Good
return fmt.Errorf("%w: cannot add STR and INT", E_TYPE)

// Better
return fmt.Errorf("%w: operator '+' requires matching types, got STR and INT", E_TYPE)
```
