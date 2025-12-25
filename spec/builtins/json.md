# MOO JSON Built-ins

## Overview

Functions for JSON encoding and decoding.

---

## 1. Encoding

### 1.1 generate_json

**Signature:** `generate_json(value [, options]) → STR`

**Description:** Converts MOO value to JSON string.

**Type mapping:**

| MOO Type | JSON Type |
|----------|-----------|
| INT | number |
| FLOAT | number |
| STR | string |
| LIST | array |
| MAP | object |
| BOOL | boolean |
| OBJ | string ("#N") |
| ERR | string ("E_XXX") |

**Options (ToastStunt):**
| Option | Effect |
|--------|--------|
| "pretty" | Indented output |
| "ascii" | Escape non-ASCII |

**Examples:**
```moo
generate_json(42)                => "42"
generate_json("hello")           => "\"hello\""
generate_json({1, 2, 3})         => "[1,2,3]"
generate_json(["a" -> 1])        => "{\"a\":1}"
generate_json(true)              => "true"
generate_json(#0)                => "\"#0\""

// Nested
generate_json(["name" -> "Alice", "age" -> 30])
  => "{\"name\":\"Alice\",\"age\":30}"

// Pretty
generate_json(["a" -> 1], "pretty")
  => "{\n  \"a\": 1\n}"
```

**Errors:**
- E_TYPE: Unsupported type (WAIF)
- E_INVARG: Invalid options

---

### 1.2 JSON Object Keys

MOO map keys are converted to strings:

```moo
generate_json([1 -> "one"])      => "{\"1\":\"one\"}"
generate_json([#0 -> "sys"])     => "{\"#0\":\"sys\"}"
```

---

## 2. Decoding

### 2.1 parse_json

**Signature:** `parse_json(string) → VALUE`

**Description:** Parses JSON string to MOO value.

**Type mapping:**

| JSON Type | MOO Type |
|-----------|----------|
| number (int) | INT |
| number (float) | FLOAT |
| string | STR |
| array | LIST |
| object | MAP |
| true/false | BOOL |
| null | INT (0) |

**Examples:**
```moo
parse_json("42")                 => 42
parse_json("3.14")               => 3.14
parse_json("\"hello\"")          => "hello"
parse_json("[1,2,3]")            => {1, 2, 3}
parse_json("{\"a\":1}")          => ["a" -> 1]
parse_json("true")               => true
parse_json("null")               => 0
```

**Errors:**
- E_INVARG: Invalid JSON syntax

---

## 3. Special Values

### 3.1 Numbers

JSON numbers map to INT or FLOAT based on format:

```moo
parse_json("42")        => 42 (INT)
parse_json("42.0")      => 42.0 (FLOAT)
parse_json("1e10")      => 10000000000.0 (FLOAT)
```

Large integers that overflow INT are converted to FLOAT.

### 3.2 Null

JSON `null` becomes MOO integer 0:

```moo
parse_json("null")               => 0
parse_json("[1,null,3]")         => {1, 0, 3}
parse_json("{\"x\":null}")       => ["x" -> 0]
```

### 3.3 Unicode

Unicode escapes are decoded:

```moo
parse_json("\"\\u0041\"")        => "A"
parse_json("\"\\u00e9\"")        => "é"
```

---

## 4. Round-Trip Considerations

Not all MOO values survive round-trip:

| MOO Value | JSON | Round-Trip |
|-----------|------|------------|
| 42 | "42" | 42 ✓ |
| "hello" | "\"hello\"" | "hello" ✓ |
| {1, 2} | "[1,2]" | {1, 2} ✓ |
| ["a"->1] | "{\"a\":1}" | ["a"->1] ✓ |
| #0 | "\"#0\"" | "#0" (string!) |
| E_TYPE | "\"E_TYPE\"" | "E_TYPE" (string!) |

**Object IDs and Errors become strings and don't round-trip.**

---

## 5. Error Handling

| Error | Condition |
|-------|-----------|
| E_INVARG | Invalid JSON syntax |
| E_TYPE | Unsupported value type |
| E_ARGS | Wrong argument count |

Common parse errors:
- Trailing comma
- Single quotes (must use double)
- Unquoted keys
- Comments (not allowed in JSON)

---

## 6. Go Implementation Notes

```go
import "encoding/json"

func builtinGenerateJson(args []Value) (Value, error) {
    value := args[0]

    pretty := false
    if len(args) > 1 {
        opts := string(args[1].(StringValue))
        pretty = strings.Contains(opts, "pretty")
    }

    jsonValue := mooToJson(value)

    var data []byte
    var err error
    if pretty {
        data, err = json.MarshalIndent(jsonValue, "", "  ")
    } else {
        data, err = json.Marshal(jsonValue)
    }

    if err != nil {
        return nil, E_TYPE
    }
    return StringValue(string(data)), nil
}

func mooToJson(v Value) any {
    switch val := v.(type) {
    case IntValue:
        return int64(val)
    case FloatValue:
        return float64(val)
    case StringValue:
        return string(val)
    case BoolValue:
        return bool(val)
    case ObjValue:
        return fmt.Sprintf("#%d", int64(val))
    case ErrValue:
        return val.String()
    case *MOOList:
        arr := make([]any, len(val.data))
        for i, item := range val.data {
            arr[i] = mooToJson(item)
        }
        return arr
    case *MOOMap:
        obj := make(map[string]any)
        for k, v := range val.entries {
            obj[k.String()] = mooToJson(v)
        }
        return obj
    default:
        return nil
    }
}

func builtinParseJson(args []Value) (Value, error) {
    jsonStr := string(args[0].(StringValue))

    var data any
    if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
        return nil, E_INVARG
    }

    return jsonToMoo(data), nil
}

func jsonToMoo(v any) Value {
    switch val := v.(type) {
    case nil:
        return IntValue(0)
    case bool:
        return BoolValue(val)
    case float64:
        // Check if it's really an integer
        if val == float64(int64(val)) && val <= float64(math.MaxInt64) {
            return IntValue(int64(val))
        }
        return FloatValue(val)
    case string:
        return StringValue(val)
    case []any:
        list := make([]Value, len(val))
        for i, item := range val {
            list[i] = jsonToMoo(item)
        }
        return &MOOList{data: list}
    case map[string]any:
        m := NewMOOMap()
        for k, v := range val {
            m.Set(StringValue(k), jsonToMoo(v))
        }
        return m
    default:
        return IntValue(0)
    }
}
```
