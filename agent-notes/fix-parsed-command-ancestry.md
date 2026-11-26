# Parsed command ancestry order

**Goal** Use left-to-right depth-first precedence when searching inherited verbs for parsed player commands.

**State** The focused Go regression reproduced selection of definer #4 instead of #2 with the same receiver on the baseline. All three cases passed after the change. WSL Debian managed conformance reported `2 passed` on both stock Toast and fixed Barn, including capability admission and the unchanged command replay.

**Decisions and why** Keep the command target order, command argument checks, per-object definition order, and visit deduplication. The command name-existence check produces a boolean and uses its existing traversal.

**Surprises** The callable verb fix did not reach `FindVerbInAncestry`, the command-specific filtered selector.

**Next step** Publish the branch for review. The full pinned Elle driver is collecting evidence separately.
