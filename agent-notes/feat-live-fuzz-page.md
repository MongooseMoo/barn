# Live fuzz machine page

## Goal

Q, 2026-11-11: a live view of the fuzz machine ("what happened to the actual
machinery to see this live?"); answered "Make it so" to: one static page on
GitHub Pages reading GitHub's public API in the browser, Pages turned on for
barn, and the unused OAuth App and claude.ai custom connector deleted.

## State

- `docs/live/index.html`: plain-text page (headings, paragraphs, lists; one
  `role=status` summary line). Every 2 minutes it reads fuzz.yml runs, jobs of
  the current and previous dedicated cycle, controller runs, newest 40 issues
  (titles starting "Nightly fuzz") and newest 40 PRs (head `claude/issue-*`).
  No token. Checked in Chrome from a local server on 2026-11-11 against live
  GitHub data.
- `docs/.nojekyll`: Pages serves `docs/` as files, without Jekyll rendering
  the Markdown there.
- Deleted 2026-11-11: GitHub OAuth App "Claude.ai GitHub connector" (ctoth,
  never authorized, 0 users) and the claude.ai custom connector "GitHub".

## Decisions and why

- Public API from the browser: barn is public; the claude.ai dashboard type can
  be live only through a connector, and GitHub has none in the directory
  (the earlier attempt needed an OAuth App with full `repo` scope).
- Run state comes from jobs: GitHub reported run 42955669432 as `queued`
  while four lanes were in progress.
- Lane set mirrors `fuzz_compute.cjs` NOT_LANES.

## Surprises

- Unauthenticated limit is 60 requests an hour per address; one refresh makes
  about 6. GitHub's responses carry `max-age=60` and an ETag.

## Next step

After merge: turn on Pages (master, `/docs`) and open
https://mongoosemoo.github.io/barn/live/ to confirm it loads.
