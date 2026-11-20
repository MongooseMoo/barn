# Deferred differential comparisons

## Goal

Temporarily defer two exact corpus sources from differential comparison.

## State

The existing exclusion path recognizes source fingerprints and reports only
`deferred comparison`. Excluded inputs remain counted in coverage totals.

## Decisions and why

Use exact source fingerprints so the exclusion applies across corpus paths
without excluding a broader syntax category. Preserve native corpus coverage
and the existing comparison acceptance policy.

## Surprises

None.

## Next step

Run PR checks and merge the bounded exclusion. Regression coverage checks that
a deferred source is counted and an unrelated source remains eligible.
