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

### 2.2 all_verbs (ToastStunt)

**Signature:** `all_verbs(object) → LIST`

**Description:** Returns all verbs including inherited.

---

## 3. Verb Information

### 3.1 verb_info

**Signature:** `verb_info(object, name) → LIST`

**Description:** Returns verb metadata.

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

**Signature:** `verb_args(object, name) → LIST`

**Description:** Returns verb argument specification.

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

**Signature:** `verb_code(object, name [, fully_paren [, indent]]) → LIST`

**Description:** Returns verb source code as list of lines.

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

**Returns:** Empty list on success, or list of compile errors.

**Examples:**
```moo
errors = set_verb_code(obj, "greet", {"notify(player, \"Hello!\");"});
if (errors)
    notify(player, "Compile errors: " + tostr(errors));
endif
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

### 7.1 verb_location (ToastStunt)

**Signature:** `verb_location(object, name) → OBJ`

**Description:** Returns object where verb is defined.

**Examples:**
```moo
// If obj inherits "tell" from $thing
verb_location(obj, "tell")   => $thing
```

---

### 7.2 responds_to (ToastStunt)

**Signature:** `responds_to(object, name) → BOOL`

**Description:** Tests if object has verb (directly or inherited).

---

### 7.3 call_function

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

**Signature:** `disassemble(object, name) → LIST`

**Description:** Returns bytecode disassembly.

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
