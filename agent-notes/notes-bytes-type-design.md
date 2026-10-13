# Notes: MOO binary strings → a real bytes type

## Established facts (read from source, not guessed)

Toast source (WSL Debian, `/root/src/toaststunt/src/`):

- `utils.cc:672 stream_add_raw_bytes_to_binary` — encoder. `if (c != '~' && (isgraph(c) || c == ' ')) emit c; else stream_printf("~%02X", c)`.
- `utils.cc:687 raw_bytes_to_binary` — wraps it, returns a **shared `static Stream *`** (non-reentrant; pointer invalidated by the next call).
- `utils.cc:700 binary_to_raw_bytes` — decoder. Any non-`~` byte passes through **raw**; `~` requires exactly 2 hex digits (case-insensitive) else `nullptr` → `E_INVARG "Invalid binary string"`. Loop is `while (*ptr)` — stops at NUL, which cannot occur in a C string anyway.
- `list.cc:1472 bf_decode_binary` — `fully` arg gives list of ints; default splits into runs of `isgraph(c) || c==' ' || c=='\t'` (note: **`\t` included here, escaped by the encoder**) vs individual ints.
- `list.cc:1544 encode_binary(s, v, min, max)` — recursive over INT/STR/LIST; ints outside [min,max] → 0 → `E_INVARG`.
- `list.cc:1570 bf_encode_binary` — min 0 max 255.
- `list.cc:1595 bf_chr` — same encoder, min/max **32/254 for non-wizards, 0/255 for wizards**, and does **NOT** ~XX-encode the result. So `chr(200)` puts a raw 0xC8 into a TYPE_STR.
- Consumers: `fileio.cc:181-182` (`file_type_binary` in/out filters), `network.cc:547,1720` + `server.cc:966,2939` (connection `binary` option), `exec.cc:185,196,426` (stdout/stderr out, stdin in), `base64.cc:66` (`encode_base64` decodes its arg from binary-string first).
- Type codes, `include/structures.h:100-119`: 0 INT, 1 OBJ, 2 STR, 3 ERR, 4 LIST, 5 CLEAR, 6 NONE, 7 CATCH, 8 FINALLY, 9 FLOAT, 10 MAP, 11 ITER, 12 ANON, 13 WAIF, 14 BOOL. **15 is the next free slot.**

Barn side:

- `types/typecode.go` mirrors those codes exactly; no bytes type.
- `types/str.go:151-173` `literal()` — comment already pins the correct rule: toliteral escapes only `"` and `\`, `~XX` belongs to I/O only. So Barn TYPE_STR already carries the *encoded* text, matching Toast.
- `builtins/fileio.go:171 encodeBinaryBytes` / `:184 EncodeRawToBinary`; `builtins/crypto.go:279 decodeBinaryString`. Codec is duplicated/scattered across packages.
- `types/str.go:47 compareFoldedASCII` already branches on `utf8.ValidString` — evidence that TYPE_STR is doing double duty as text and as bytes.

## Key analytical conclusions

1. The only byte a Toast MOO string genuinely cannot hold is **0x00** (C NUL termination). `chr()` proves raw high bytes are legal in strings. So the `~XX` alphabet escapes ~60% of the byte space to solve a problem with exactly 1 byte.
2. Binary strings are **not a type** — no type-system distinction, so `length`/index/slice/`==`/`toliteral` all operate on the *encoding*.
3. The encoding is **not canonical** — `"A"` and `"~41"` decode identically, so equality and map-key identity are broken w.r.t. the represented value.
4. Encoder/decoder are **asymmetric** (strict out, lenient in) and disagree on `\t`.
5. **Therefore Barn must NOT "just store raw bytes in TYPE_STR"** — `length()` would diverge from Toast. The escaped form must stay in TYPE_STR; the raw payload needs its own type.

## Deliverable

`docs/design/bytes-type.md` — TYPE_BYTES (15) proposal with a 3-mode I/O compatibility hinge.

## State

Design doc written. No code changes made. Nothing blocked.
