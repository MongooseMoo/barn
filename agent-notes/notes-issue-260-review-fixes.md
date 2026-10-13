Checkpoint 2026-09-02: Read issue #260 and confirmed its restore-boundary contract includes terminal control flow and stack metadata.
Current state: Branch fix/issue-260-bytecode-verifier contains verifier and restore tests; review reports two panic paths and repeated decoding.
Key finding: Both P1 findings are directly in scope; P2 is also at the issue's deserialization trust boundary.
Blocker: None.
Next action: Add focused failing tests, then implement stack-flow validation, strict fork continuation, and decoded-boundary reuse.
