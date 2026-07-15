# Barn performance campaign setup

- Source branch: `master`
- Source HEAD: `864de996a111674adfe15c330f8e85813f4641f0`
- Source tracked-file state: clean; `git status --short --untracked-files=no` produced no output.
- Campaign branch: `campaign/barn-performance-20260714`
- Campaign worktree path: `C:\Users\Q\code\barn-performance-campaign`
- Pre-commit tree identity: `7985f719e51a66ea0d2aa19c1154a698f47c18da`

## Verification commands and results

Command:

```text
git rev-parse --show-toplevel
```

Result:

```text
C:/Users/Q/code/barn-performance-campaign
```

Command:

```text
git branch --show-current
```

Result:

```text
campaign/barn-performance-20260714
```

Command:

```text
git rev-parse HEAD
```

Result:

```text
864de996a111674adfe15c330f8e85813f4641f0
```

Command:

```text
git status --short --untracked-files=no
```

Result: no output.

Command:

```text
git rev-parse 'HEAD^{tree}'
```

Result:

```text
7985f719e51a66ea0d2aa19c1154a698f47c18da
```
