# Automatic fixes: label, then a conformance test PR on merge

## Goal

Q, 2026-11-10: "Can we label the automatic prs something, and get it so that
when they are merged, moo conformance tests are automatically made in moo
conformance tests and pr'd?" Q chose a fine-grained PAT and said "Make it so!".

## State

- Label `automatic-fix` created in barn. The repair prompt in
  `.github/workflows/claude-fix.yml` now opens PRs with `--label automatic-fix`.
- New job `conformance` in the same workflow, on `pull_request: closed` when
  merged, labelled `automatic-fix`, and from this repository. It checks out the
  merge commit and moo-conformance-tests main, and Claude writes a test, runs it on
  the WSL Toast oracle and on the merged Barn, and commits it to branch
  `barn-fix/pr-<N>` locally, with the PR title and body in `conformance-pr/`.
  Otherwise it writes `conformance-pr/no-test.md` and comments it once on the
  merged Barn PR.
- Step "Open conformance pull request" pushes that branch and opens the PR in
  moo-conformance-tests with secret `CONFORMANCE_PR_TOKEN` (Q created it; read
  back in `gh secret list`). It fails visibly when Claude left neither a branch
  nor `no-test.md`, an empty branch, or missing title/body.

## Decisions and why

- A separate credential is required: claude-code-action docs (faq.md, security.md
  at the pinned version) say its app token is limited to the current repository.
- The token is given only to the push/PR shell step, never to the agent.
- Shared steps (conformance checkout, setup-go, setup-uv, claude_args) are YAML
  anchors in the one workflow, not copies.
- `allowed_bots: github-merge-queue` because merges go through the merge queue.
  Issue events for #690 show the merge attributed to the queuing user; which actor
  the `closed` event carries was not verified.
- Toast truth: moo-conformance-tests PR CI runs the full Toast suite on every PR.

## Surprises

None yet.

## Next step

PR CI (actionlint, zizmor), merge queue. Then the first merged `automatic-fix` PR
proves the job end to end; PRs merged before this change carry no label.
