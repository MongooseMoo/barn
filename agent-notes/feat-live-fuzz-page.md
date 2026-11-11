# Live fuzz machine page

## Goal

Q, 2026-11-11: a live view of the fuzz machine ("what happened to the actual
machinery to see this live?"); answered "Make it so" to: one static page on
GitHub Pages reading GitHub's public API in the browser, Pages turned on for
barn, and the unused OAuth App and claude.ai custom connector deleted.

## State

- `docs/live/index.html`: plain-text page (headings, paragraphs, lists; one
  `role=status` summary line). Every 6 minutes it reads fuzz.yml runs, jobs of
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
- Refresh every 6 minutes (Q, 2026-11-11, chose this over 3 minutes with 3
  requests): at most 6 requests per refresh, 10 refreshes an hour = the 60
  unauthenticated limit. The first version refreshed every 2 minutes (180 an
  hour) and would have run out after about 20 minutes.

## Surprises

- Unauthenticated limit is 60 requests an hour per address; one refresh makes
  about 6. GitHub's responses carry `max-age=60` and an ETag.

## Next step

Pages is on (master, `/docs`); https://mongoosemoo.github.io/barn/live/
loaded live data on 2026-11-11. After the refresh change merges, confirm the
page says "every 6 minutes".
