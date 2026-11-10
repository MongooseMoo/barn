# Tests: per-entry conflicts on a player-keyed map read through a catch

## Goal
Q, 2026-11-09: "Make it so!" for a test with two players' tasks on a
player-keyed map. Pin that the MOO shape deployed in Mongoose's `$away:stored`
is tracked per map entry, so two players' tasks do not conflict:

    s = `this.state[who] ! E_RANGE => 0';
    ...
    this.state[who] = s;

## State
Done. Tests added to `vm/map_key_test.go`; no production code changed.
- `TestCatchReadOfObjectKeyedEntriesForDifferentPlayersBothCommit` (entries
  present; entries absent and inserted): both commit.
- `TestCatchReadOfTheSameObjectKeyedEntryConflicts` (present; absent): the
  second commit loses validation.
- `TestWholeMapReadOfObjectKeyedStateConflictsAcrossPlayers`: the verb's
  previous shape (`states = this.state; ... states[who]`) conflicts across
  players.
Run: `go test ./vm -run 'TestKeyed|TestCatchRead|TestWholeMapRead' -count=1`.

## Decisions and why
- Tests run real MOO source through the file's existing compile-and-run
  helper, as the neighbouring keyed-map tests do.
- The helper gained `runOnThis`, which binds `this` in the frame the way
  `RunWithVerbContext` does, so the source can say `this.state` unchanged.

## Surprises
- `PrepareVerbFrame` does not set the `this` local; without the explicit
  bind the source fails with E_VARNF.
- Before these, the keyed-map tests covered string keys only and no catch
  expression.

## Next step
None for this branch. The wider work is in `agent-notes/mongoose-goodput.md`.
