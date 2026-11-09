# test/structured-differential

## Goal

Make the nightly ToastDifferential job (`scripts/fuzz_differential.py`, run from
`.github/workflows/fuzz.yml`) accept structured fuzz corpus records (Go
`[]byte(...)` choice encodings), render them through the single Go owner
(`internal/fuzzseed` via `cmd/moo-fuzz-source`), keep choice bytes + rendered
source in each result row, keep per-lane provenance and completed counts, and
add the retained structured lanes to the workflow invocation.

## State

Implemented and committed locally on test/structured-differential; not pushed,
no PR (PR body drafted at `.tmp/pr-body.md`, untracked).

- `cmd/moo-fuzz-source -corpus`: stdin lists corpus file paths; one JSON line
  per file with `path`, `choices` (hex), `source`, or `error`.
- `scripts/fuzz_differential.py`: `[]byte(` records go through that tool in one
  `go run` call; rows carry `lane`, `origin`, `choices` (structured) and
  `source`; `summary.lanes[<dir>]` has records, duplicates, excluded, eligible,
  completed and the undecodable records with their error. Undecodable records
  (bad literal, several values, neither string nor []byte, no MOO spelling) are
  excluded as `undecodable corpus record` and never reach the engines.
- `.github/workflows/fuzz.yml`: the differential also reads the Go fuzz cache of
  vm FuzzStructuredSourceExecution, compiler FuzzStructuredCompileVerifiable and
  parser FuzzStructuredParseFormatRoundTrip.
- Fixtures: two real cache records, byte-identical (`cmp`) to the local GOCACHE
  copies, under `scripts/fixtures/fuzz/FuzzStructured*/`.
- Focused test: `cd scripts && py -3 -m unittest test_fuzz_differential` ran 21
  tests, OK. `gofmt -l` and `go vet ./cmd/moo-fuzz-source` clean.

Unverified: no real nightly run with structured lanes; CI has not run.

## Decisions and why

- Go owns decoding of the `[]byte(...)` line too, not only rendering. Real
  structured records (local GOCACHE, e.g.
  `vm/FuzzStructuredSourceExecution/e58adbf989baa13f`) are written by Go's
  `internal/fuzz/encoding.go` `fmt.Fprintf(b, "[]byte(%q)\n", t)`, and `%q`
  leaves printable UTF-8 runs as raw characters (`ޚ`, `ɇ`) mixed with `\xNN`
  escapes. Go's reader is `strconv.Unquote`; Python has no exact inverse, so
  `cmd/moo-fuzz-source -corpus` decodes with `strconv.Unquote` and renders with
  `fuzzseed.Source`, one JSON line per file. The Python side only classifies the
  record by its `[]byte(` prefix and sends paths on stdin (one Go process).
- All three structured targets (vm, compiler, parser) use `fuzzseed.AddChoices`
  and `fuzzseed.Program`/`Source`, so all three lanes are rendered the same way.
- PR #657 (retention layout `.fuzz/retained/<pkg>/<target>/inputs`) is OPEN,
  not merged (checked 2026-11-07). The workflow edit is limited to what master
  has: the Go fuzz-cache dirs of the structured targets, next to the existing
  string-lane cache dirs.

- Each corpus directory given on the command line is one lane; a `[]byte(`
  record anywhere is treated as structured choices, so only structured-target
  directories should be passed alongside the string lanes.

## Surprises

- The real vm structured record renders a program containing `fork`, which the
  existing exclusion list drops. Structured programs also use `suspend` from
  `fuzzseed.builtinNames`. Expect a sizeable share of the vm structured lane to
  be excluded; the per-lane `excluded` count shows how many.

## Next step

- When PR #657 merges, its differential job reads
  `.fuzz/retained/<pkg>/<target>/inputs`; add
  `.fuzz/retained/vm/FuzzStructuredSourceExecution/inputs`,
  `.fuzz/retained/compiler/FuzzStructuredCompileVerifiable/inputs` and
  `.fuzz/retained/parser/FuzzStructuredParseFormatRoundTrip/inputs` there (merge
  origin/master, no rebase). The fuzz.yml hunk in this branch will conflict with
  #657's rewrite of that step.
- Out of scope: reduction/minimisation of divergences against the same
  differential predicate from fresh initial state (plan section 3, second
  paragraph).
