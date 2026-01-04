# MOO Cryptography Built-ins

## Overview

Functions for hashing, password hashing, secure random generation, and base64 encoding.

---

## 1. Hashing

### 1.1 string_hash

**Signature:** `string_hash(string [, algorithm]) → STR`

**Description:** Computes cryptographic hash of string.

**Algorithms:**
| Algorithm | Output Size |
|-----------|-------------|
| "MD5" | 32 hex chars |
| "SHA1" | 40 hex chars |
| "SHA256" | 64 hex chars |
| "SHA512" | 128 hex chars |

**Default:** SHA256

**Examples:**
```moo
string_hash("hello")
// => "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824" (SHA256)

string_hash("hello", "MD5")
// => "5d41402abc4b2a76b9719d911017c592"

string_hash("password", "SHA512")
// => "b109f3bbbc244eb82441917ed06d618b..."
```

**Errors:**
- E_TYPE: Non-string input
- E_INVARG: Unknown algorithm

---

### 1.2 binary_hash

**Signature:** `binary_hash(string [, algorithm]) → STR`

**Description:** Computes cryptographic hash, identical to string_hash().

**Note:** Despite the name, returns hex-encoded string (not raw binary).

**Algorithms:** Same as string_hash (MD5, SHA1, SHA256, SHA512).

**Default:** SHA256

---

### 1.3 value_hash

**Signature:** `value_hash(value) → STR`

**Description:** Returns SHA256 hash of any MOO value as hex string.

**Examples:**
```moo
value_hash(123)        => "a665a45920422f9d417e4867efdc4fb8a04a1f3fff1fa07e998e86f7f7a27ae3"
value_hash("hello")    => "5aa762ae383fbb727af3c7a36d4940a5b8c40a989452d2304fc958ff3f354e7a"
value_hash({1, 2, 3})  => "8298d0492354e620262b63e1d84fb85e1b3d9df71672a4894441a5ee30b08c0c"
```

**Note:** Not suitable for hash tables (use map keys instead).

---

## 2. Password Hashing

### 2.1 crypt

**Signature:** `crypt(plaintext, salt) → STR`

**Description:** One-way password hash using crypt().

**Parameters:**
- `plaintext`: Password to hash
- `salt`: Salt string (bcrypt format required)

**Platform Support:**
- **Windows:** Only bcrypt salts ($2a$, $2b$) supported
- **Unix/Linux:** Full support (MD5, SHA-256, SHA-512, DES, bcrypt)

**Salt formats (Unix/Linux only):**
| Prefix | Algorithm |
|--------|-----------|
| `$2a$`, `$2b$` | bcrypt (Windows + Unix) |
| `$1$` | MD5 (Unix only) |
| `$5$` | SHA-256 (Unix only) |
| `$6$` | SHA-512 (Unix only) |
| (other) | DES (Unix only, legacy) |

**Examples:**
```moo
// On Windows, use bcrypt salt
hash = crypt("password", "$2b$10$saltsaltsaltsaltsaltse");

// Verify password
if (crypt("password", stored_hash) == stored_hash)
    notify(player, "Correct!");
endif
```

**Errors:**
- E_INVARG: Unsupported salt format (e.g., non-bcrypt on Windows)

---

### 2.2 argon2 (ToastStunt)

**Signature:** `argon2(plaintext, salt [, options]) → STR`

**Description:** Argon2id password hash (Argon2 version 19).

**Parameters:**
- `plaintext`: Password to hash
- `salt`: Salt string
- `options`: Optional map for memory cost, time cost, parallelism

**Default Parameters:**
- Memory cost (m): 4096 KB
- Time cost (t): 3 iterations
- Parallelism (p): 1 thread
- Algorithm: Argon2id (hybrid)

**Examples:**
```moo
hash = argon2("password", "randomsalt");
// => "$argon2id$v=19$m=4096,t=3,p=1$cmFuZG9tc2FsdA$..."
```

**Note:** Argon2id won the Password Hashing Competition. Recommended for new applications.

---

### 2.3 argon2_verify (ToastStunt)

**Signature:** `argon2_verify(plaintext, hash) → INT`

**Description:** Verifies password against argon2 hash.

**Returns:**
- 1: Password matches hash
- 0: Password does not match

**Examples:**
```moo
if (argon2_verify("password", stored_hash))
    notify(player, "Login successful!");
endif
```

---

## 3. Random Generation

### 3.1 random_bytes (ToastStunt)

**Signature:** `random_bytes(count) → STR`

**Description:** Generates cryptographically secure random bytes.

**Examples:**
```moo
key = random_bytes(32);   // 256-bit key
iv = random_bytes(16);    // 128-bit IV
```

---

### 3.2 random

See [math.md](math.md) - not cryptographically secure.

---

## 4. Encoding

### 4.1 encode_base64

**Signature:** `encode_base64(data) → STR`

**Description:** Encodes string as base64.

**Examples:**
```moo
encode_base64("hello")   => "aGVsbG8="
encode_base64("\x00\x01") => "AAE="
```

---

### 4.2 decode_base64

**Signature:** `decode_base64(string) → STR`

**Description:** Decodes base64 string.

**Errors:**
- E_INVARG: Invalid base64

---

## 5. Error Handling

| Error | Condition |
|-------|-----------|
| E_TYPE | Non-string input |
| E_INVARG | Invalid algorithm/key/data |
| E_ARGS | Wrong argument count |

---

## 6. Security Considerations

1. **Use argon2 for passwords** - Not MD5, SHA, or plain crypt()
2. **Use random_bytes for salts/keys** - Not random()
3. **Platform compatibility** - Test crypt() on target platform
4. **Constant-time comparison** - For security-critical comparisons

---

## 7. Go Implementation Notes

```go
import (
    "crypto/md5"
    "crypto/rand"
    "crypto/sha1"
    "crypto/sha256"
    "crypto/sha512"
    "encoding/base64"
    "encoding/hex"
    "strings"

    "github.com/GehirnInc/crypt"
    _ "github.com/GehirnInc/crypt/sha256_crypt"
    _ "github.com/GehirnInc/crypt/sha512_crypt"

    "github.com/matthewhartstonge/argon2"
)

func builtinStringHash(args []Value) (Value, error) {
    data := []byte(string(args[0].(StringValue)))

    algorithm := "SHA256"
    if len(args) > 1 {
        algorithm = string(args[1].(StringValue))
    }

    var hash []byte
    switch strings.ToUpper(algorithm) {
    case "MD5":
        h := md5.Sum(data)
        hash = h[:]
    case "SHA1":
        h := sha1.Sum(data)
        hash = h[:]
    case "SHA256":
        h := sha256.Sum256(data)
        hash = h[:]
    case "SHA512":
        h := sha512.Sum512(data)
        hash = h[:]
    default:
        return nil, E_INVARG
    }

    return StringValue(strings.ToUpper(hex.EncodeToString(hash))), nil
}

func builtinBinaryHash(args []Value) (Value, error) {
    // Identical to string_hash despite the name
    return builtinStringHash(args)
}

func builtinValueHash(args []Value) (Value, error) {
    // Serialize value to canonical form, then SHA256
    serialized := serializeValue(args[0])
    h := sha256.Sum256([]byte(serialized))
    return StringValue(strings.ToLower(hex.EncodeToString(h[:]))), nil
}

func builtinCrypt(args []Value) (Value, error) {
    plaintext := string(args[0].(StringValue))
    salt := string(args[1].(StringValue))

    // On Windows, only bcrypt is supported
    // On Unix, full crypt() support
    crypter := crypt.NewFromHash(salt)
    if crypter == nil {
        return nil, E_INVARG
    }

    hash, err := crypter.Generate([]byte(plaintext), []byte(salt))
    if err != nil {
        return nil, E_INVARG
    }

    return StringValue(hash), nil
}

func builtinArgon2(args []Value) (Value, error) {
    plaintext := []byte(string(args[0].(StringValue)))
    salt := []byte(string(args[1].(StringValue)))

    // Default config: m=4096, t=3, p=1
    config := argon2.DefaultConfig()
    if len(args) > 2 {
        // Parse options map if provided
        // opts := args[2].(MapValue)
        // Update config based on opts
    }

    hash, err := config.Hash(plaintext, salt)
    if err != nil {
        return nil, E_INVARG
    }

    return StringValue(string(hash.Encode())), nil
}

func builtinArgon2Verify(args []Value) (Value, error) {
    plaintext := []byte(string(args[0].(StringValue)))
    encoded := string(args[1].(StringValue))

    ok, err := argon2.VerifyEncoded(plaintext, []byte(encoded))
    if err != nil {
        return IntValue(0), nil
    }

    if ok {
        return IntValue(1), nil
    }
    return IntValue(0), nil
}

func builtinRandomBytes(args []Value) (Value, error) {
    count := int(args[0].(IntValue))
    if count <= 0 || count > 1024*1024 {
        return nil, E_INVARG
    }

    bytes := make([]byte, count)
    _, err := rand.Read(bytes)
    if err != nil {
        return nil, E_FILE
    }

    return StringValue(string(bytes)), nil
}

func builtinEncodeBase64(args []Value) (Value, error) {
    data := []byte(string(args[0].(StringValue)))
    encoded := base64.StdEncoding.EncodeToString(data)
    return StringValue(encoded), nil
}

func builtinDecodeBase64(args []Value) (Value, error) {
    encoded := string(args[0].(StringValue))
    decoded, err := base64.StdEncoding.DecodeString(encoded)
    if err != nil {
        return nil, E_INVARG
    }
    return StringValue(string(decoded)), nil
}
```
