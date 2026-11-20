# Install Claude Code per repair run

## Goal
Q 2026-11-10 ("Make it so!"): stop concurrent claude-fix runs from losing their
repair at install time.

## State
claude-fix.yml: both jobs run step `Install Claude Code for this run`
(anchor `install-claude`), which runs the official installer with
`HOME=$RUNNER_TEMP/claude-home`, and pass
`path_to_claude_code_executable: $RUNNER_TEMP/claude-home/.local/bin/claude`
to claude-code-action, which then skips its own install.

## Decisions and why
- Cause, from the logs of the repair runs for issues #694 and #695 (2026-11-10
  04:37): `/home/github-runner/.claude/downloads/claude-2.1.322-linux-x64: Text
  file busy`, then `Failed to install Claude Code after 3 attempts`. The
  repairs for #692/#693 started 45 s earlier and were running that file.
- The installer always writes under `$HOME` (`DOWNLOAD_DIR="$HOME/.claude/downloads"`
  in https://claude.ai/install.sh), and the action's installer is that same
  script (`buildInstallCommand`, src/entrypoints/run.ts at the pinned action
  commit). So the only lever is HOME for the install. HOME is overridden for
  the install only, not the whole action step, so Go/uv caches, git and the
  Toast oracle paths Claude uses are unchanged.
- No version argument: the action pins 2.1.318 internally; copying that number
  here would drift when the action pin moves. The installer's default channel
  is used instead.

## Surprises
The action's bootstrap download is always the latest release (2.1.322) even
though it then installs 2.1.318.

## Next step
After merge: the next dispatched repair run shows "Using custom Claude Code
executable" in its log. Re-running repairs for #694/#695 is held for session
barn-a8's OOM diagnosis.
