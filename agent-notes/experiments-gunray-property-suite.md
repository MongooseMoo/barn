# Gunray property suite

Goal: preserve Barn's MOO correctness experiments with runnable models and demonstrate defeasible handler revision.

State: the experiment directory contains the model generators, managed observer and runner, short historical replays and scalar observation tables. The README gives the standalone model command and managed engine commands. The handler experiment tests both adding a rule against fixed facts and adding one fact against fixed rules.

Decisions: use the existing two-error handler as the default so both clause orders form legal MOO. A no-preference theory gives Gunray a control that has no corresponding MOO source order. Preserve its undecided result.

Evidence: both VMs choose the first matching handler for the original clause-order tests. The full semantic hunt and reduced replay are from the versions identified in the experiment README. The suite preserves their disagreements as regression evidence.
