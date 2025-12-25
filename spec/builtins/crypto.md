# MOO Cryptography Built-ins

## Overview

Functions for hashing, encryption, and secure random generation.

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

**Default:** MD5 (for compatibility)

**Examples:**
```moo
string_hash("hello")
// => "5d41402abc4b2a76b9719d911017c592" (MD5)

string_hash("hello", "SHA256")
// => "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"

string_hash("password", "SHA512")
// => "b109f3bbbc244eb82441917ed06d618b..."
```

**Errors:**
- E_TYPE: Non-string input
- E_INVARG: Unknown algorithm

---

### 1.2 binary_hash

**Signature:** `binary_hash(string [, algorithm]) → STR`

**Description:** Returns raw binary hash (not hex-encoded).

**Use case:** When binary output needed for further processing.

---

### 1.3 value_hash

**Signature:** `value_hash(value) → INT`

**Description:** Returns integer hash of any MOO value.

**Note:** Not cryptographic; for hash tables.

---

## 2. Password Hashing

### 2.1 crypt

**Signature:** `crypt(plaintext [, salt]) → STR`

**Description:** One-way password hash using Unix crypt().

**Parameters:**
- `plaintext`: Password to hash
- `salt`: Salt string (auto-generated if omitted)

**Salt formats:**
| Prefix | Algorithm |
|--------|-----------|
| `$1$` | MD5 |
| `$5$` | SHA-256 |
| `$6$` | SHA-512 |
| (other) | DES (legacy) |

**Examples:**
```moo
hash = crypt("password");
// => "$6$rounds=5000$salt$hash..."

// Verify password
if (crypt("password", stored_hash) == stored_hash)
    notify(player, "Correct!");
endif
```

---

### 2.2 bcrypt (ToastStunt)

**Signature:** `bcrypt(plaintext [, cost]) → STR`

**Description:** Bcrypt password hash.

**Parameters:**
- `cost`: Work factor (default: 10)

**Examples:**
```moo
hash = bcrypt("password");
// => "$2b$10$..."

hash = bcrypt("password", 12);  // Higher cost
```

---

### 2.3 bcrypt_verify (ToastStunt)

**Signature:** `bcrypt_verify(plaintext, hash) → BOOL`

**Description:** Verifies password against bcrypt hash.

**Examples:**
```moo
if (bcrypt_verify("password", stored_hash))
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

## 4. HMAC (ToastStunt)

### 4.1 hmac

**Signature:** `hmac(key, data [, algorithm]) → STR`

**Description:** Computes HMAC (keyed hash).

**Algorithms:** Same as string_hash.

**Examples:**
```moo
signature = hmac(secret_key, message, "SHA256");
```

---

## 5. Encryption (ToastStunt)

### 5.1 encrypt

**Signature:** `encrypt(plaintext, key [, algorithm]) → STR`

**Description:** Encrypts data.

**Algorithms:**
| Algorithm | Key Size |
|-----------|----------|
| "AES-128" | 16 bytes |
| "AES-256" | 32 bytes |

**Returns:** Base64-encoded ciphertext with IV prepended.

**Examples:**
```moo
key = random_bytes(32);
encrypted = encrypt("secret message", key, "AES-256");
```

---

### 5.2 decrypt

**Signature:** `decrypt(ciphertext, key [, algorithm]) → STR`

**Description:** Decrypts data.

**Examples:**
```moo
decrypted = decrypt(encrypted, key, "AES-256");
// => "secret message"
```

**Errors:**
- E_INVARG: Decryption failed (wrong key, corrupted data)

---

## 6. Encoding

### 6.1 encode_base64

**Signature:** `encode_base64(data) → STR`

**Description:** Encodes string as base64.

**Examples:**
```moo
encode_base64("hello")   => "aGVsbG8="
encode_base64("\x00\x01") => "AAE="
```

---

### 6.2 decode_base64

**Signature:** `decode_base64(string) → STR`

**Description:** Decodes base64 string.

**Errors:**
- E_INVARG: Invalid base64

---

### 6.3 encode_hex (ToastStunt)

**Signature:** `encode_hex(data) → STR`

**Description:** Encodes as hexadecimal.

**Examples:**
```moo
encode_hex("ABC")   => "414243"
```

---

### 6.4 decode_hex (ToastStunt)

**Signature:** `decode_hex(string) → STR`

**Description:** Decodes hexadecimal.

---

## 7. UUID (ToastStunt)

### 7.1 uuid

**Signature:** `uuid() → STR`

**Description:** Generates random UUID (v4).

**Examples:**
```moo
uuid()   => "550e8400-e29b-41d4-a716-446655440000"
```

---

## 8. Error Handling

| Error | Condition |
|-------|-----------|
| E_TYPE | Non-string input |
| E_INVARG | Invalid algorithm/key/data |
| E_ARGS | Wrong argument count |

---

## 9. Security Considerations

1. **Use bcrypt for passwords** - Not MD5 or SHA
2. **Use random_bytes for keys** - Not random()
3. **Key management** - Store keys securely
4. **Constant-time comparison** - For security-critical comparisons

---

## 10. Go Implementation Notes

```go
import (
    "crypto/aes"
    "crypto/cipher"
    "crypto/hmac"
    "crypto/md5"
    "crypto/rand"
    "crypto/sha1"
    "crypto/sha256"
    "crypto/sha512"
    "encoding/base64"
    "encoding/hex"

    "golang.org/x/crypto/bcrypt"
)

func builtinStringHash(args []Value) (Value, error) {
    data := []byte(string(args[0].(StringValue)))

    algorithm := "MD5"
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

    return StringValue(hex.EncodeToString(hash)), nil
}

func builtinBcrypt(args []Value) (Value, error) {
    password := []byte(string(args[0].(StringValue)))

    cost := bcrypt.DefaultCost
    if len(args) > 1 {
        cost = int(args[1].(IntValue))
    }

    hash, err := bcrypt.GenerateFromPassword(password, cost)
    if err != nil {
        return nil, E_INVARG
    }

    return StringValue(string(hash)), nil
}

func builtinBcryptVerify(args []Value) (Value, error) {
    password := []byte(string(args[0].(StringValue)))
    hash := []byte(string(args[1].(StringValue)))

    err := bcrypt.CompareHashAndPassword(hash, password)
    return BoolValue(err == nil), nil
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

func builtinEncrypt(args []Value) (Value, error) {
    plaintext := []byte(string(args[0].(StringValue)))
    key := []byte(string(args[1].(StringValue)))

    block, err := aes.NewCipher(key)
    if err != nil {
        return nil, E_INVARG
    }

    // Generate IV
    iv := make([]byte, aes.BlockSize)
    rand.Read(iv)

    // Pad plaintext
    plaintext = pkcs7Pad(plaintext, aes.BlockSize)

    // Encrypt
    mode := cipher.NewCBCEncrypter(block, iv)
    ciphertext := make([]byte, len(plaintext))
    mode.CryptBlocks(ciphertext, plaintext)

    // Prepend IV and base64 encode
    result := append(iv, ciphertext...)
    return StringValue(base64.StdEncoding.EncodeToString(result)), nil
}
```
