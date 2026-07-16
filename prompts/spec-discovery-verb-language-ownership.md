**You are a WORKER agent launched via the Task tool. Execute this task directly. Do NOT read foreman.md. Do NOT coordinate — DO the work yourself.**

# Task: Discover Specifications for Verb Language Ownership

## Objective

Find every repository specification or package charter affected by Phase 1 of
`plans/multilingual-verb-language-cleanup-plan.md`, and write a source-cited
discovery report. Do not edit specifications or production code.

## Files to Read

- `plans/multilingual-verb-language-cleanup-plan.md`
- `spec/README.md`
- `spec/vm.md`
- `spec/statements.md`
- `spec/grammar.md`
- `spec/database.md`
- `parser/AGENTS.md`
- Any directly cross-referenced spec needed to verify a claim

## Required Report

Write `reports/spec-discovery-verb-language-ownership.md` containing:

- The exact spec sections that must change.
- Cross-references that must remain consistent.
- Current statements contradicted by the target architecture.
- Terms that require definitions.
- Open questions, or `None` if repository evidence resolves them.
- A source file and line number for every factual claim.

## Verification Constraint

Do not speculate. Every claim in your report must be verified by reading source
code or observing test output. If you cannot verify something, say "I did not
verify this". Do not use words like "may", "possibly", "might", "could be", or
"likely". If you do not know, say you do not know. Trace the actual source.

## Parallel Work Safety

Other agents share this worktree. Do not run `git stash`, `git restore`,
`git checkout`, `git reset`, or `git clean`. Do not modify any file except the
required report.

## No Oneliners

Never write `python -c "..."` or `uv run python -c "..."`. Not even for quick
checks. If substantive scripting is required, write a reusable script file and
run it. This task should not require scripting.
