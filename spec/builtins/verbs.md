# MOO Verb Built-ins

## Overview

Functions for managing object verbs (methods/code).

---

## 1. Verb Calling

### 1.1 Direct Call

```moo
obj:verb_name(args...)       // Call verb
obj:(expr)(args...)          // Dynamic name call
```

**Examples:**
```moo
player:tell("Hello");
verb = "tell";
player:(verb)("Hello");
```

---

### 1.2 pass

**Signature:** `pass(args...) → VALUE`

**Description:** Calls same verb on parent object.

**Semantics:**
- Looks up verb on parent of current `this`
- Passes through all arguments if none specified
- Common for inheritance chains

**Examples:**
```moo
// In child:describe()
result = pass();              // Call parent:describe()
result = pass("detailed");    // Call with different args
return result + " (extended)";
```

**Errors:**
- E_VERBNF: Verb not found on parent

---

## 2. Verb Listing

### 2.1 verbs

**Signature:** `verbs(object) → LIST`

**Description:** Returns list of verb names defined on object.

**Examples:**
```moo
verbs($thing)   => {"tell", "look", "describe", ...}
```

**Errors:**
- E_INVIND: Invalid object
- E_PERM: Object not readable

---

## 3. Verb Information

### 3.1 verb_info

**Signature:** `verb_info(object, name_or_index) → LIST`

**Description:** Returns verb metadata.

**Parameters:**
- `name_or_index`: Either a verb name (STR) or 1-based integer index (INT)

**Returns:** `{owner, perms, names}`
- `owner`: Object that owns the verb
- `perms`: Permission string
- `names`: Verb name(s) as space-separated string

**Verb aliases:**
- The `names` field contains all verb aliases separated by spaces
- Example: `"get take grab"` defines three aliases for the same verb
- The first name is the primary name
- All aliases invoke the same verb code
- When adding verbs, all names must be unique (no conflicts with existing verb names or aliases)

**Permission characters:**
| Char | Meaning |
|------|---------|
| r | Readable (code visible) |
| w | Writable (modifiable) |
| x | Executable |
| d | Debug info available |

**Examples:**
```moo
verb_info($thing, "tell")   => {#wizard, "rxd", "tell"}
```

**Errors:**
- E_INVIND: Invalid object
- E_VERBNF: Verb not found

---

### 3.2 set_verb_info

**Signature:** `set_verb_info(object, name, info) → none`

**Description:** Changes verb metadata.

**Parameters:**
- `info`: `{owner, perms, names}`

**Examples:**
```moo
set_verb_info(obj, "secret", {player, "x", "secret"});
```

**Errors:**
- E_PERM: Not owner/wizard
- E_VERBNF: Verb not found

---

## 4. Verb Arguments

### 4.1 verb_args

**Signature:** `verb_args(object, name_or_index) → LIST`

**Description:** Returns verb argument specification.

**Parameters:**
- `name_or_index`: Either a verb name (STR) or 1-based integer index (INT)

**Returns:** `{dobj, prep, iobj}`
- `dobj`: Direct object spec
- `prep`: Preposition spec
- `iobj`: Indirect object spec

**Object specs:**
| Value | Meaning |
|-------|---------|
| "this" | This object |
| "none" | No object |
| "any" | Any object |

**Preposition specs:**

The preposition field in `verb_args()` returns the full slash-separated spec (e.g., `"with/using"`), but when setting verb arguments via `add_verb()` or `set_verb_args()`, only **individual preposition names** are valid (e.g., `"with"` or `"using"`), not the full slash-separated string.

| Value | Matches |
|-------|---------|
| "none" | No preposition |
| "any" | Any preposition |
| "with/using" | "with", "using" |
| "at/to" | "at", "to" |
| "in/inside/into" | "in", "inside", "into" |
| "on/onto/upon" | "on", "onto", "upon" |
| "from/out of" | "from", "out of" |
| ... | (many more) |

**Prep spec validation:**
- When calling `add_verb()` or `set_verb_args()`, the prep field must be either `"none"`, `"any"`, or a single preposition name like `"with"`, `"on"`, `"in"`, etc.
- Full slash-separated strings like `"with/using"` will return E_INVARG
- The server expands the single prep to its full form when storing (e.g., `"with"` becomes `"with/using"`)
- `verb_args()` always returns the expanded form

**Examples:**
```moo
verb_args($thing, "put")   => {"this", "in/inside/into", "any"}
// Matches: "put thing in box"
```

**Errors:**
- E_VERBNF: Verb not found

---

### 4.2 set_verb_args

**Signature:** `set_verb_args(object, name, args) → none`

**Description:** Changes verb argument specification.

**Examples:**
```moo
set_verb_args(obj, "put", {"this", "in", "any"});
```

---

## 5. Verb Code

### 5.1 verb_code

**Signature:** `verb_code(object, name_or_index [, fully_paren [, indent]]) → LIST`

**Description:** Returns verb source code as list of lines.

**Parameters:**
- `name_or_index`: Either a verb name (STR) or 1-based integer index (INT)

**Parameters:**
- `fully_paren`: Add parentheses for clarity (default: false)
- `indent`: Indent level (default: 0)

**Examples:**
```moo
verb_code($thing, "tell")
  => {"if (valid(this))", "  notify(this, @args);", "endif"}
```

**Errors:**
- E_PERM: Verb not readable
- E_VERBNF: Verb not found

---

### 5.2 set_verb_code

**Signature:** `set_verb_code(object, name, code) → LIST`

**Description:** Sets verb source code.

**Parameters:**
- `code`: List of source lines

**Returns:** Empty list `{}` on success, or list of error strings on compile failure.

**Error format:** Returns a list of strings describing parse/compile errors, not error objects. Each string contains a human-readable error message.

**Examples:**
```moo
errors = set_verb_code(obj, "greet", {"notify(player, \"Hello!\");"});
if (errors)
    notify(player, "Compile errors: " + tostr(errors));
endif

// Example error return:
// {"parse error: expected ';' after expression statement"}
```

**Errors:**
- E_PERM: Verb not writable
- E_VERBNF: Verb not found

---

## 6. Verb Management

### 6.1 add_verb

**Signature:** `add_verb(object, info, args) → none`

**Description:** Adds a new verb to object.

**Parameters:**
- `object`: Target object
- `info`: `{owner, perms, names}`
- `args`: `{dobj, prep, iobj}`

**Examples:**
```moo
add_verb(obj, {player, "rxd", "greet"}, {"none", "none", "none"});
```

**Errors:**
- E_PERM: Not owner/wizard
- E_INVARG: Verb already exists

---

### 6.2 delete_verb

**Signature:** `delete_verb(object, name) → none`

**Description:** Removes verb from object.

**Examples:**
```moo
delete_verb(obj, "obsolete");
```

**Errors:**
- E_PERM: Not owner/wizard
- E_VERBNF: Verb not found

---

## 7. Verb Dispatch

### 7.1 call_function

**Signature:** `call_function(name, args...) → VALUE`

**Description:** Calls built-in function by name.

**Examples:**
```moo
call_function("length", "hello")   => 5
call_function("max", 1, 2, 3)      => 3
```

**Errors:**
- E_INVARG: Unknown function

---

## 8. Verb Context

Inside a verb, these variables are set automatically:

| Variable | Type | Description |
|----------|------|-------------|
| `this` | OBJ | Object verb is on |
| `player` | OBJ | Initiating player |
| `caller` | OBJ | Calling object |
| `verb` | STR | Verb name as called |
| `args` | LIST | Arguments passed |
| `argstr` | STR | Original arg string |
| `dobj` | OBJ | Direct object |
| `dobjstr` | STR | Direct object text |
| `iobj` | OBJ | Indirect object |
| `iobjstr` | STR | Indirect object text |
| `prepstr` | STR | Preposition text |

---

## 9. Verb Permissions

### 9.1 Execution

Verb callable if:
- Caller owns the object
- Caller is wizard
- Verb has 'x' permission

### 9.2 Read

Verb code readable if:
- Caller owns the verb
- Caller is wizard
- Verb has 'r' permission

### 9.3 Write

Verb modifiable if:
- Caller owns the verb
- Caller is wizard
- Verb has 'w' permission

---

## 10. Eval and Dynamic Execution

### 10.1 eval

**Signature:** `eval(code) → LIST`

**Description:** Compiles and executes code string.

**Returns:** `{success, result_or_error}`
- `{1, value}` on success
- `{0, error}` on failure

**Examples:**
```moo
eval("return 2 + 2;")        => {1, 4}
eval("return undefined;")    => {0, E_VARNF}
eval("syntax error")         => {0, "compile error..."}
```

**Errors:**
- E_PERM: Not a programmer

---

## 11. Disassembly (ToastStunt)

### 11.1 disassemble

**Signature:** `disassemble(object, name_or_index) → LIST`

**Description:** Returns bytecode disassembly.

**Parameters:**
- `name_or_index`: Either a verb name (STR) or 1-based integer index (INT)

**Format:** Returns simplified pseudo-opcodes like "PUSH", "ADD", "RETURN" generated from an AST walk, not actual VM bytecode.

**Wizard only.**

---

## 12. Verb Cache (ToastStunt)

### 12.1 verb_cache_stats

**Signature:** `verb_cache_stats() → LIST`

**Description:** Returns statistics for the verb cache.

**Returns:** `{hits, negative_hits, misses, generation, histogram}`
- `hits`: Cache hits
- `negative_hits`: Negative cache hits
- `misses`: Cache misses
- `generation`: Current verb generation counter
- `histogram`: List of counts by bucket depth (length 17; index 1 is depth 0, index 17 is depth >= 16)

**Wizard only.**

---

### 12.2 log_cache_stats

**Signature:** `log_cache_stats() → INT`

**Description:** Logs verb cache statistics to the server log and returns 0.

**Wizard only.**

---

## Go Implementation Notes

```go
type Verb struct {
    Name     string
    Names    []string        // All names (aliases)
    Owner    int64
    Perms    VerbPerms
    ArgSpec  VerbArgs
    Code     []string        // Source lines
    Program  *Program        // Compiled bytecode (cached)
}

type VerbPerms uint8

const (
    VERB_READ  VerbPerms = 1 << 0
    VERB_WRITE VerbPerms = 1 << 1
    VERB_EXEC  VerbPerms = 1 << 2
    VERB_DEBUG VerbPerms = 1 << 3
)

type VerbArgs struct {
    Dobj string  // "this", "none", "any"
    Prep string  // preposition spec
    Iobj string  // "this", "none", "any"
}

func builtinVerbs(args []Value) (Value, error) {
    objID := int64(args[0].(ObjValue))
    obj := db.GetObject(objID)
    if obj == nil {
        return nil, E_INVIND
    }

    names := make([]Value, 0, len(obj.Verbs))
    for name := range obj.Verbs {
        names = append(names, StringValue(name))
    }
    return &MOOList{data: names}, nil
}

func builtinVerbCode(args []Value) (Value, error) {
    objID := int64(args[0].(ObjValue))
    name := string(args[1].(StringValue))

    verb, err := db.FindVerb(objID, name)
    if err != nil {
        return nil, E_VERBNF
    }

    if verb.Perms&VERB_READ == 0 && !callerIsWizard() {
        return nil, E_PERM
    }

    lines := make([]Value, len(verb.Code))
    for i, line := range verb.Code {
        lines[i] = StringValue(line)
    }
    return &MOOList{data: lines}, nil
}

func builtinSetVerbCode(args []Value) (Value, error) {
    objID := int64(args[0].(ObjValue))
    name := string(args[1].(StringValue))
    codeList := args[2].(*MOOList)

    verb, err := db.FindVerb(objID, name)
    if err != nil {
        return nil, E_VERBNF
    }

    if verb.Perms&VERB_WRITE == 0 && !callerIsWizard() {
        return nil, E_PERM
    }

    // Convert list to lines
    lines := make([]string, len(codeList.data))
    for i, v := range codeList.data {
        lines[i] = string(v.(StringValue))
    }

    // Compile
    program, errors := compiler.Compile(lines)
    if len(errors) > 0 {
        errList := make([]Value, len(errors))
        for i, e := range errors {
            errList[i] = StringValue(e.Error())
        }
        return &MOOList{data: errList}, nil
    }

    verb.Code = lines
    verb.Program = program
    return &MOOList{data: nil}, nil  // Empty = success
}
```
