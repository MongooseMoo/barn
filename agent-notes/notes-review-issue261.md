2026-09-03 review checkpoint
- Scope: compare HEAD with merge base 32664a21a20b8fe61eb1f4fc5579d624dad76b79.
- Current state: diff and VM root/finalization call paths inspected; `go test ./vm` passed.
- Key candidate: fork payload `ForkInfo.Body` retains a program whose constants are not visited.
- Next: confirm whether omitted body constants can carry finalizable values, then finish findings.
