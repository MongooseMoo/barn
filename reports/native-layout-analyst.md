# Native layout analysis

## Scope

Read-only analysis of the completed native layout pass and its oracle fixtures.
The in-progress continuation converter and Barn symbolic lowering are excluded.
No implementation changes or publication.

## State

Inspecting exact native layout, symbolic state identity, failure handling and
boundary cases against the actual native producer source. Findings will be
recorded with concrete evidence.

## Findings

### P2: Named while binding is a conversion prerequisite still missing in Barn

This is an integration prerequisite identified while reviewing the named-loop
layout, not a finding against the unfinished converter. Native execute.cc
EOP_WHILE_ID at2616-2622 assigns TOP_RT_VALUE to the named variable before the
ordinary truth test, including the final false condition. Barn compileWhile in
compiler/lower_control.go uses the label for loop control and tick metadata but
never stores the condition to that variable. Resuming native code into that
compiled loop will diverge when the condition is tested again.

Sent root this exact witness for managed native verification before repair:

```moo
x=99; seen={};
while x (length(seen)<2 ? length(seen)+1 | 0)
  seen={@seen,x};
endwhile
return {seen,x};
```

Expected from primary native execution source: `{{1,2},0}`. This analyst has not
run the native witness and does not present that expected result as an observed
oracle result.

### P2: Original-checkpoint layout census permits zero coverage

compiler/native_layout_test.go:179 initializes frames/sites to0, skips every
non-native frame and ends at221 with only a Logf. There is no nonzero coverage
assertion. A valid but wrong BARN_MONGOOSE_DB can therefore give a green run
without checking any continuation. Concrete existing input `.tmp/conformance/db/Test.db`
is a valid version4 fixture with zero suspended/interrupted records:

```text
baseline version: 4 saved suspended/interrupted: 0
```

The test itself was not rerun because this review is read-only and the project
permits only the explicit test file being written to run locally. The zero-path
acceptance follows directly from the test's complete control flow. Require
nonzero native frames/sites when the environment opts into this census; retain
the logged counts. The reported actual70-frame run is not invalidated.

## Limits

The15-case raw corpus meaningfully checks widths, maximum stack and post-call
PCs; it does not independently check all semantic origin/control descriptions.
No additional concrete native-layout offset discrepancy was established in this
review. Existing source inspection confirmed that native num_stacks remains0,
so omitting stack-ref expansion from label values matches that historical native
behavior rather than proving an error here.

Two exploratory read failures made no changes: an unverified/nonexistent path
search failed; an initial v17-only reader call on the version4 baseline was then
replaced with the reader's version-dispatching load API. The advertised role
protocol path is absent.
