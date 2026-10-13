# Bytecode bounds cluster audit checkpoint

- Current audit head: `aa82abf05a6a23783ba69105d0c393f926304a13` on `master`.
- Used only live GitHub issue bodies for #39, #69, #70 and current `bytecode`/`vm` source. Historical project Markdown was not read.
- #39 remains current: `addConstant` rejects only `idx > 255`, so constant index 255 can still be emitted as operand `0xFF`, which is the dynamic-name sentinel for property/verb opcodes.
- #69 remains current: catch-expression, try/except handler, and try/finally absolute IP patches still write two bytes directly without range checks.
- #70 remains current: `compileFork` still writes `bodyLen` directly into two bytes without checking `bodyLen > 0xFFFF`.
- #69 and #70 share the 16-bit bytecode representability boundary. #39 is adjacent bytecode operand validation but has a distinct one-byte reserved-sentinel boundary.
- Existing `bytecode`, `compiler`, and `vm` suites are green but have no constant-pool, handler-IP, fork-length, or >64KB regression coverage.
- Fixed-width census found adjacent unchecked backward-loop encodings at compiler lines 1863, 2045, 2160, and 2204 (`emitShort(uint16(offset))`). A repair that only changes the three direct handler/fork patches would leave the same truncation class active.
- Recommended implementation boundary: one checked unsigned-16 encoder/patch helper used by `patchJump`, absolute catch/try/finally targets, fork body lengths, and compiler-computed backward-loop offsets; keep #39 as a separate checked constant-index boundary (`idx >= 0xFF`).
- TDD should include exact helper boundaries (65535 accepted, 65536 rejected), source/IR integration for catch expression, try/except, try/finally, oversized fork, and one oversized backward loop, plus 255 usable constant entries versus a rejected 256th entry for static get/set/call operands.
- Managed WSL Toast evidence is required before choosing user-visible rejection semantics: probe `set_verb_code` with 255/256 distinct constants and >64KB try/finally/except/catch/fork verbs, recording success or exact returned diagnostics. Current conformance search found no existing coverage for these limits.
- One PR can close #39/#69/#70 if it explicitly scopes itself as bytecode operand representability and covers both the one-byte sentinel boundary and all current 16-bit compiler encoders. A global 64KB program cap is simpler but may reject representable large programs and must not be selected before Toast evidence.
