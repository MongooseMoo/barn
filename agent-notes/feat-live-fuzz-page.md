# Live fuzz machine page

## Goal

Q, 2026-11-11: a live view of the fuzz machine ("what happened to the actual
machinery to see this live?"); answered "Make it so" to: one static page on
GitHub Pages reading GitHub's public API in the browser, Pages turned on for
barn, and the unused OAuth App and claude.ai custom connector deleted.

## State

- `docs/live/index.html`: plain-text page (headings, paragraphs, lists; one
  `role=status` summary line). Every 6 minutes it reads fuzz.yml runs, jobs of
  the current and previous dedicated cycle, the latest controller run, issues
  updated since the earlier of 24 hours ago and the previous cycle's start
  (100 at most), and the 100 most recently updated PRs. No token.
- Cleanup (Q, 2026-11-11: "put the good info up front first, clean it up,
  consider what a real 'failure' is"; "also timezones!"): the summary is two
  sentences, machine state with lane progress, then new bugs and merged fixes
  in the last 24 hours. Then current cycle, previous cycle, bugs found and
  fixes merged in 24 hours, earlier cycles, backup controller, a glossary.
  Every time is the viewer's local time with its zone name, relative first.
- `docs/live/fuzz_live.js` holds the logic with no DOM (classification,
  progress, health, last-24-hours, summary, time text);
  `scripts/test_live_fuzz_page.py` runs it in Node (CI's unittest discovery).
  The Node lookup is `node_binary()` in `scripts/test_fuzz_compute.py`.
- Checked 2026-11-11 in headless Chrome on a private desktop from a local
  server against live GitHub data; no UTC or ISO strings rendered.
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
- Failure kinds per job (fuzz_live.js `classifyJob`), from what the code
  that decides them leaves visible in the public API:
  - bug found: only the campaign step failed and an issue titled
    "Nightly fuzz failure: <job>" names this run in its body
    (fuzz_report.py publish() opens it for a new signature or regression).
  - repeat: same title, a different run, and the issue's last update falls
    inside the job (publish() comments on an open issue, or on one closed
    after the fuzzed commit). An update equal to `closed_at` is the closing
    itself (a repair PR merging), not a repeat.
  - machine problem: an "infrastructure failure" issue matched the same
    way (the in-step `cycle=infrastructure` path: no saved input, signal or
    setup failure); or any failed step other than the campaign step; or a
    failure with no failed step (runner lost); or cancelled/timed out.
  - red, no report found: only the campaign step failed and no issue
    matched. Shown as such rather than guessed.
  - healthy: the Accept job's conclusion (accept() passes red lanes with
    findings). The run conclusion is not shown: any finding makes it
    "failure".
- Why issues and not step outputs: the jobs API gives step names and
  conclusions only; `cycle=` outputs and logs need a token. A finding and
  an in-step infrastructure failure both leave only `Fuzz <target>` failed.
- Limits accepted to stay at 6 requests: a repeat is matched by the
  issue's last update, so once a later cycle reports to the same issue an
  older lane's repeat no longer matches (exact matching needs one comments
  request per red lane). Older cycles' health needs one jobs request each,
  so "Earlier cycles" shows times only.
- Progress is jobs done, running and waiting, never minutes: lanes are
  about to get different durations (a `minutes` field in the matrix).
- The `role=status` line is written only when its text changes and holds
  no relative time, so NVDA speaks it only when the machine's state or the
  24-hour counts change. Other sections are replaced only when their HTML
  changed.
- Refresh every 6 minutes (Q, 2026-11-11, chose this over 3 minutes with 3
  requests): at most 6 requests per refresh, 10 refreshes an hour = the 60
  unauthenticated limit. The first version refreshed every 2 minutes (180 an
  hour) and would have run out after about 20 minutes.

## Surprises

- Unauthenticated limit is 60 requests an hour per address; one refresh makes
  about 6. GitHub's responses carry `max-age=60` and an ETag.

## Next step

Pages is on (master, `/docs`); https://mongoosemoo.github.io/barn/live/
loaded live data on 2026-11-11. After the cleanup branch
(`docs/live-fuzz-page-cleanup`) merges, load the published page and confirm
`fuzz_live.js` is served beside `index.html`.
