# Saved nightly fuzz failures

## Goal
Fix the two saved snapshot failures tracked by issue #593 and retain their exact inputs as regressions.

## State
Inputs recovered from the original GitHub artifacts and retained byte-for-byte in the corpus. Both saved-input replays pass after the repairs, as do focused tests for restored transaction initialization, allocation quotas, exclusive pending limits, and integer endpoints. Source review found no concrete defect. PR CI pending.

## Decisions and why
The November 4 input calls a primitive verb after restore. Resume does not initialize context dependencies, unlike Run and ExecuteLoop, so the store transaction remains nil and prototype lookup dereferences it. The smallest repair calls the existing initialization helper at Resume entry (one line), and retains the input.
The November 3 input shifts 35 left by 35, then constructs a descending integer list range ending at 35. executeListRange allocates the entire length before consulting any list quota, and its signed length arithmetic/endpoint loops can also overflow. The smallest repair checks the existing task list-byte quota before allocating, computes range distance without signed overflow, and fills by bounded count. It preserves existing range direction/type behavior and uses the existing value-size accounting. About 30 production lines plus focused regression cases.
The earlier compiler failure is already repaired in merged PR #576; do not reopen it.

## Surprises
The November 3 log reports only unexpected worker termination, not an OOM diagnosis. The code attempts a list with more than one trillion elements for that exact input; do not describe the historical operating-system cause as proved.

## Next step
Publish an independent PR closing #593 and use PR CI. Reporting PR #594 and the scheduling repair merge first.
