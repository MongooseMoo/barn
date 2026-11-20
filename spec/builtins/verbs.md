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
- Looks up the verb on the parent(s) of the object that *defines* the
  running verb (not on the parents of `this`). With several parents, the
  first parent (in order) on which a callable verb is found wins.
- The name looked up is the name the running verb was called by (`verb`),
  matched the same way an ordinary verb call matches: case-insensitively and
  with `*` wildcards in the parent verb's names. A parent verb named
  `gr*eet` or `GREET` answers `pass()` from a verb called as `greet`, and
  `verb` in the parent verb is still `"greet"`.
- `this`, `player` and `caller` semantics are those of a normal call; `this`
  is unchanged.
- The arguments are exactly those given to `pass()`; `pass()` passes an empty
  list. Use `pass(@args)` to forward the caller's arguments.
- Common for inheritance chains

(Toast `execute.cc` `call_verb2()` with `do_pass`, `bf_pass()`; conformance
`builtins/pass_parent_verb_matching.yaml`, `language/pass_no_parent.yaml`.)

**Examples:**
```moo
// In child:describe()
result = pass();              // Call parent:describe()
result = pass("detailed");    // Call with different args
return result + " (extended)";
```

**Errors:**
- E_INVIND: The defining object is invalid, it has no parent (`$nothing`),
  or its single parent is invalid
- E_VERBNF: No parent has a callable verb of that name

---

## 2. Verb Listing

### 2.1 verbs

**Signature:** `verbs(object) → LIST`

**Description:** Returns the names strings of the verbs defined on the object
itself (not inherited), in definition order. Each element is the verb's whole
names string, e.g. `"th*row g*et t*ake"` for a verb with three names.
Anonymous objects are accepted.

**Examples:**
```moo
verbs($thing)   => {"tell", "look", "describe", ...}
```

**Errors:**
- E_TYPE: Argument is not an object
- E_INVARG: Invalid object
- E_PERM: Object not readable

---

### 2.2 Verb Descriptors

The verb builtins other than `verbs()` and `add_verb()` name a verb on an
object with a *descriptor*:

- A STR is a verb name. It is matched against the verbs **defined on the
  object itself**; inherited verbs are never found, so a verb that the
  object only inherits is `E_VERBNF` (including for `set_verb_info`,
  `set_verb_args`, `set_verb_code`, `delete_verb` and `disassemble`, which
  leave the ancestor's verb unchanged).
- Matching is the ordinary verb-name match: the names string is split on
  **spaces only** (a tab is part of a name, so a names string of one tab is
  one name), each name is compared case-insensitively, and `*` in a name
  marks the abbreviation point. A leading `:` is part of the name, so a verb
  named `":greet"` is not found by `"greet"`. The first verb in definition
  order whose names match wins; verbs whose first names are equal (`"foo"`
  and `"foo bar"`) stay separate verbs.
- The names string itself, as `verbs()` or `verb_info()` returns it, also
  finds its own verb when passed back, because Toast compares the descriptor
  to the stored names string by identity before matching. A typed literal
  with the same text is not that string: `verb_info(o, "s es")` is
  `E_VERBNF` unless `"s es"` matches as a name. (Toast `utils.cc`
  `verbcasecmp()`; conformance `builtins/verb_descriptor_full_names.yaml`.)
- An INT is a 1-based index into the object's own verbs in definition order.
  An index of 0 or less raises `E_INVARG`; an index past the last verb
  raises `E_VERBNF`.
- Any other type raises `E_TYPE`.
- When the server option `support_numeric_verbname_strings` is true, a STR
  that is entirely a decimal number is treated as a 0-based index.

(Toast `verbs.cc` `validate_verb_descriptor()`, `find_described_verb()`;
`db_verbs.cc` `db_find_defined_verb()`, `db_find_indexed_verb()`.)

**Object argument:** `verbs`, `verb_info`, `verb_code` and `disassemble`
accept any object, anonymous included. `add_verb`, `delete_verb`,
`set_verb_info`, `set_verb_args`, `set_verb_code` and `verb_args` require a
numbered object: an anonymous object (or a WAIF, or a non-object) is
`E_TYPE`, raised at the same point a non-object would be. An invalid
numbered object is `E_INVARG`, never `E_INVIND`.

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
- Names need not be unique: an object may define several verbs with the same
  or overlapping names, and lookup takes the first in definition order

**Permission characters:**
| Char | Meaning |
|------|---------|
| r | Readable (code visible) |
| w | Writable (modifiable) |
| x | Executable |
| d | Debug info available |

The returned `perms` string is always lowercase in the order `r`, `w`, `x`,
`d`, whatever case or order it was set with.

**Examples:**
```moo
verb_info($thing, "tell")   => {#wizard, "rxd", "tell"}
```

**Errors (in the order checked):**
- E_TYPE: `object` is not an object
- E_TYPE / E_INVARG: Bad descriptor (§2.2)
- E_INVARG: Invalid object
- E_VERBNF: Verb not found
- E_PERM: Verb not readable (caller is not the verb owner, not a wizard, and
  the verb lacks `r`)

---

### 3.2 set_verb_info

**Signature:** `set_verb_info(object, desc, info) → 0`

**Description:** Changes verb metadata. Returns 0.

**Parameters:**
- `desc`: Verb descriptor (§2.2)
- `info`: `{owner, perms, names}`, validated as in §3.3

**Examples:**
```moo
set_verb_info(obj, "secret", {player, "x", "secret"});
```

**Errors (in the order checked):**
- E_TYPE / E_INVARG: Bad descriptor (§2.2)
- E_TYPE: `object` is not a numbered object
- E_INVARG: Invalid object
- E_TYPE / E_INVARG: Bad `info` (§3.3), checked before the verb is looked up
- E_VERBNF: Verb not defined on `object`
- E_PERM: Verb not writable (caller is not the verb owner, not a wizard, and
  the verb lacks `w`), or a non-wizard names an owner different from the
  verb's current owner. The `w` flag lets a non-owner programmer change
  perms and names, but never the owner.

---

### 3.3 Verb Info Validation

`add_verb()` and `set_verb_info()` validate `info` the same way (Toast
`verbs.cc` `validate_verb_info()`):

1. It must be a list of exactly three elements `{OBJ, STR, STR}`; any other
   shape or element type is `E_TYPE`. An anonymous object as owner is
   `E_TYPE`.
2. The owner must be valid, else `E_INVARG`.
3. Each perms character must be one of `r`, `w`, `x`, `d` in either case,
   in any order; any other character is `E_INVARG`.
4. Leading spaces are stripped from the names string; if nothing remains
   (`""` or only spaces) it is `E_INVARG` and nothing changes.

(Conformance `builtins/verb_info_empty_names.yaml`,
`builtins/verb_setter_error_precedence.yaml`,
`builtins/verb_flags_descriptors_write_flag.yaml`.)

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

The preposition field in `verb_args()` returns the full slash-separated spec
(e.g., `"with/using"`). Toast's fifteen prepositions, by number:

| # | Spec returned by `verb_args()` |
|---|-------------------------------|
| 0 | "with/using" |
| 1 | "at/to" |
| 2 | "in front of" |
| 3 | "in/inside/into" |
| 4 | "on top of/on/onto/upon" |
| 5 | "out of/from inside/from" |
| 6 | "over" |
| 7 | "through" |
| 8 | "under/underneath/beneath" |
| 9 | "behind" |
| 10 | "beside" |
| 11 | "for/about" |
| 12 | "is" |
| 13 | "as" |
| 14 | "off/off of" |

Plus `"none"` (no preposition) and `"any"` (any preposition).

**Args validation** (`add_verb()` and `set_verb_args()`, Toast `verbs.cc`
`validate_verb_args()`, `db_verbs.cc` `db_match_prep()`):
- `args` must be a list of exactly three strings, else `E_TYPE` (an object
  or integer element is `E_TYPE`).
- `dobj` and `iobj` must be `"none"`, `"any"` or `"this"`, compared
  case-insensitively, else `E_INVARG`.
- `prep` may be `"none"` or `"any"` (case-insensitive), or a preposition
  given as:
  - any one of its words, e.g. `"with"`, `"using"`, `"on top of"`;
  - a full spec such as `"with/using"` (only the text before the first `/`
    is matched);
  - its number as decimal digits, optionally after `#`: a string whose first
    character (after an optional `#`) is a digit and that is entirely a
    number from 0 to 14, so `"01"` is preposition 1. `"15"`, `"00015"`,
    `"-1"`, `"+1"`, `" 1"`, `"1 "` and `""` are `E_INVARG`.
  Anything else is `E_INVARG`.
- The stored preposition is the number; `verb_args()` always returns the
  full spec from the table above.

(Conformance `builtins/numeric_preposition_range.yaml`,
`builtins/numeric_preposition_lexical.yaml`.)

**Examples:**
```moo
verb_args($thing, "put")   => {"this", "in/inside/into", "any"}
// Matches: "put thing in box"
```

**Errors (in the order checked):**
- E_TYPE: `object` is not a numbered object (checked before the descriptor;
  an anonymous object is `E_TYPE` even with a descriptor of 0)
- E_TYPE / E_INVARG: Bad descriptor (§2.2)
- E_INVARG: Invalid object
- E_VERBNF: Verb not found
- E_PERM: Verb not readable

---

### 4.2 set_verb_args

**Signature:** `set_verb_args(object, desc, args) → 0`

**Description:** Changes verb argument specification. Returns 0.

**Examples:**
```moo
set_verb_args(obj, "put", {"this", "in", "any"});
```

**Errors (in the order checked):**
- E_TYPE / E_INVARG: Bad descriptor (§2.2)
- E_TYPE: `object` is not a numbered object
- E_INVARG: Invalid object
- E_TYPE / E_INVARG: Bad `args` (§4.1), checked before the verb is looked up
- E_VERBNF: Verb not defined on `object`
- E_PERM: Verb not writable (not the verb owner, not a wizard, and no `w`
  flag)

---

## 5. Verb Code

### 5.1 verb_code

**Signature:** `verb_code(object, name_or_index [, fully_paren [, indent]]) → LIST`

**Description:** Returns verb source code as list of lines.

**Parameters:**
- `name_or_index`: Verb descriptor (§2.2)
- `fully_paren`: If true, add parentheses for clarity (default: false)
- `indent`: If true, indent nested blocks (default: **true**; Toast
  `verbs.cc` `bf_verb_code()`: `indent = nargs < 4 || is_true(arg4)`)

Anonymous objects are accepted.

**Float literals:** `verb_code()` decompiles the stored program; it does not
return the source text given to `set_verb_code()`. A float literal is
printed with `%.15g` (15 significant digits), with `.0` appended when the
result has neither `.` nor `e` (Toast `list.cc` `unparse_value()`, called
from `unparse.cc` for `EXPR_VAR`). So after
`set_verb_code(o, "fv", {"return {18.46744073709553614, 1.0000000000000002};"})`,
`verb_code(o, "fv")` is `{"return {18.4674407370955, 1.0};"}` (conformance
`builtins/set_verb_code_float_literal_precision.yaml`). The program itself
keeps the exact doubles (see §5.2).

**Examples:**
```moo
verb_code($thing, "tell")
  => {"if (valid(this))", "  notify(this, @args);", "endif"}
```

**Errors (in the order checked):**
- E_TYPE: `object` is not an object
- E_TYPE / E_INVARG: Bad descriptor (§2.2)
- E_INVARG: Invalid object
- E_VERBNF: Verb not found
- E_PERM: Verb not readable

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

A failed compile leaves the verb's existing program unchanged.

**Float literals:** A float literal is compiled to the full double its
spelling denotes, and the running verb sees that exact value: after the
`set_verb_code()` call in §5.1, `o:fv()` returns
`{18.46744073709553614, 1.0000000000000002}`, which is not equal to
`{18.4674407370955, 1.0}`, even though `verb_code()` shows 15 digits
(conformance `builtins/set_verb_code_float_literal_precision.yaml`). In
Toast this holds only until the next checkpoint: the database stores verb
programs as their decompiled text (`db_io.cc` `dbio_write_program()` calls
`unparse_program()`), so after a restart such a literal is the 15-digit
value. (Float *values* in properties are written with `%.19g`, `db_io.cc`
`dbio_write_float()`.)

**Errors (in the order checked):**
- E_TYPE: Any element of `code` is not a string (checked before anything
  else, including the object and the descriptor)
- E_TYPE: `object` is not a numbered object
- E_TYPE / E_INVARG: Bad descriptor (§2.2)
- E_INVARG: Invalid object
- E_VERBNF: Verb not defined on `object`
- E_PERM: The task's permissions are not a programmer, or the verb is not
  writable (not the verb owner, not a wizard, and no `w` flag). A verb with
  `w` can be reprogrammed by any programmer.

---

## 6. Verb Management

### 6.1 add_verb

**Signature:** `add_verb(object, info, args) → INT`

**Description:** Adds a new verb to the end of the object's verb list and
returns its 1-based index (Toast `verbs.cc` `bf_add_verb()` returns
`db_add_verb()`'s result). Names are not checked for uniqueness: adding
`"foo bar"` to an object that already has `"foo"` succeeds and creates a
second, separate verb.

**Parameters:**
- `object`: Target object (numbered; anonymous is `E_TYPE`)
- `info`: `{owner, perms, names}`, validated as in §3.3
- `args`: `{dobj, prep, iobj}`, validated as in §4.1

**Examples:**
```moo
add_verb(obj, {player, "rxd", "greet"}, {"none", "none", "none"});  => 1
```

**Errors (in the order checked):**
- E_TYPE / E_INVARG: Bad `info` (§3.3)
- E_TYPE / E_INVARG: Bad `args` (§4.1)
- E_TYPE: `object` is not a numbered object
- E_INVARG: Invalid object
- E_PERM: Object not writable by the caller, or a non-wizard names an owner
  other than itself

---

### 6.2 delete_verb

**Signature:** `delete_verb(object, desc) → 0`

**Description:** Removes a verb defined on the object. Returns 0.

**Examples:**
```moo
delete_verb(obj, "obsolete");
```

**Errors (in the order checked):**
- E_TYPE / E_INVARG: Bad descriptor (§2.2)
- E_TYPE: `object` is not a numbered object
- E_INVARG: Invalid object
- E_PERM: Object not writable by the caller. Permission is the object's `w`
  flag (or ownership of the object, or wizard), not the verb's owner or
  flags, and it is checked before the verb is looked up, so a missing verb
  on an unwritable object is `E_PERM`.
- E_VERBNF: Verb not defined on `object`

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

A verb is found by a programmed call (`obj:verb()`, `pass()`) only if it has
the `x` flag; there is no owner or wizard bypass, and a verb without `x` is
skipped as if absent (Toast `db_verbs.cc` `find_verbdef_by_name()` with
`check_x_bit`). Command dispatch ignores `x` (see `spec/objects.md` §5.4).

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

**Format:** Returns Toast-shaped metadata and instruction rows decoded from the
actual compiled bytecode program. Operator mnemonics use Toast's public names;
in particular, semantic index boundaries are `FIRST` and `LAST`, bitwise
complement is `COMPLEMENT`, and shifts are `BITSHL` and `BITSHR`. The builtin
must not reparse source or walk parser syntax or verb IR to synthesize output.

The verb is found as in §2.2 (defined on the object itself; an inherited verb
is `E_VERBNF`). Anonymous objects are accepted. Not wizard-only: errors are,
in order, E_TYPE (not an object), E_TYPE / E_INVARG (bad descriptor),
E_INVARG (invalid object), E_VERBNF, E_PERM (verb not readable). (Toast
`disassemble.cc` `bf_disassemble()`; conformance
`builtins/verb_prop_builtin_precedence.yaml`.)

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

Compiled programs are runtime-derived values owned and cached by `compiler`;
they are not fields of the persistent verb model.

```go
type Verb struct {
    Name       string
    Names      []string      // All names (aliases)
    Owner      int64
    Perms      VerbPerms
    ArgSpec    VerbArgs
    Code       []string      // Original persistent source lines
    HasProgram bool          // Empty code is distinct from never programmed
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
