# Claude fixes nightly fuzz issues

## Goal
Q 2026-11-04: nightly fuzz failure opens an issue (Codex, PR #594); a Claude
Code agent then fixes it unattended and leaves a PR for Q to review in the
morning. Local runs are acceptable.

## State
- Branch ci/claude-fuzz-fix, worktree ../barn-claude-fuzz-fix, based on master.
- Repo secret CLAUDE_CODE_OAUTH_TOKEN set 2026-11-04 (1-year subscription
  token from `claude setup-token`, run under pywinpty from a scratch script).
  Two earlier attempts also created tokens but crashed printing them, so two
  unstored tokens exist on Q's account; Q was told.
- Label `claude-fix` not pre-created (a PreToolUse hook refused `gh label
  create`); the fuzz job's addLabels call creates it on first use. Claude GitHub App `claude` is installed org-wide.
- .github/workflows/claude-fix.yml written (uncommitted): workflow_dispatch
  issue_number + issues.labeled claude-fix; self-hosted runner; checks out
  moo-conformance-tests; action pinned to v1 commit; allowed_bots
  github-actions; Bash,Read,Edit,Write,Glob,Grep allowed.
- Waiting for #594 to merge before adding label + dispatch to fuzz.yml.
- Action source read (v1): run.ts sets GH_TOKEN to the Claude app token, so
  `gh pr create` from Claude uses the app and the PR triggers CI; agent mode
  configures git auth (modes/agent/index.ts:47); actor.ts allowed_bots strips
  "[bot]".
- Runners: 8 self-hosted WSL runners online, labels self-hosted,Linux,X64,barn.

## Decisions and why
- Use anthropics/claude-code-action on the self-hosted WSL runners, not a cloud
  scheduled task: Barn's rules require checking behaviour against the WSL Toast
  oracle, which the cloud sandbox lacks.
- Trigger by workflow_dispatch(issue_number) from fuzz.yml plus `issues:
  labeled` with label `claude-fix` for manual use. #594 creates issues with
  GITHUB_TOKEN, and GitHub does not start workflows from GITHUB_TOKEN events
  except workflow_dispatch/repository_dispatch.
- Coordinated with Codex (barn thread 01a1a41b) via ~/code/claude-code-bridge:
  #594 stays reporting-only; this PR adds the label + dispatch to fuzz.yml
  after #594 merges; dispatch only for newly created issues; Codex has a
  separate scheduling PR replacing GitHub cron with a workstation dispatcher.

## Surprises
- `claude setup-token` prints nothing without a TTY.

## Next step
Get the OAuth token into the repo secret; write claude-fix.yml; wait for #594
to merge; add label + dispatch to fuzz.yml; PR, CI, merge; test with a dispatch.
