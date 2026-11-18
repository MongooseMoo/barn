# Remove the false "Edit/Write fail" section from CLAUDE.md

## Goal

Delete the "CRITICAL: Subagent File Writing Failures" section of CLAUDE.md.

## State

Section removed on this branch; PR open against master.

## Decisions and why

- The section said Edit/Write "fail" with "file unexpectedly modified" and told
  agents to try four path spellings, then stop. That message means the file
  changed on disk after the agent last read it; reading it again and redoing
  the edit is the whole remedy. Path spelling has nothing to do with it.
- The section also claimed shell writes "DESTROY FILES". Q's actual rule
  (2026-11-17) is narrow: an agent has Read and Write/Edit tools and uses them
  for its own reading and authoring, because those tools track what was read
  and written. Programs run in the shell may write their output anywhere.
- The inflated version of that rule was also in Q's global instructions and in
  a hook (`~/.claude/hooks/no_shell_writes.py`) that blocked every shell
  redirection. Both were corrected the same day, outside this repository.

## Surprises

62 files under `prompts/` carry a shorter "File Modified Error Workaround"
block with the same path-spelling advice. They are old one-task prompts, not
loaded into sessions, and are not changed here.

## Next step

None after merge.
