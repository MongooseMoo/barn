PR #304 explicit managed restart coverage completed against WSL Toast and Barn.
Head 866d0fc2 integrates current master; auto-merge is enabled.
Latest live CI output: build/unit tests passed, full conformance pending; queue entry null.
User additionally requested removal of all Claudio hooks from Codex; global hooks.json contains Claudio entries.
Next: run the Codex-scoped Claudio uninstaller, verify remaining hooks, then check PR CI and queue.
