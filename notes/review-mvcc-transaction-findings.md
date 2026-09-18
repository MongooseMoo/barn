# MVCC Transaction Review Checklist

Each finding is resolved in order with a failing regression test, the narrow implementation fix, targeted verification, and its own commit.

- [x] 1. Deep-copy collections before publishing COW images (`db/store/store_cow.go`)
- [x] 2. Hold read-set slots through validation and publish (`db/store/store_cow.go`)
- [x] 3. Do not erase conflicts after a live mutation (`db/store/store_txn.go`)
- [x] 4. Prevent retries after irreversible side effects (`scheduler/task_runtime.go`)
- [x] 5. Route ticker input through the connection lane (`server/input_processor.go`)
- [x] 6. Support delete-then-redefine in one transaction (`db/store/store_txn.go`)
- [x] 7. Preserve call order when flushing side effects (`scheduler/task_runtime.go`)
- [x] 8. Keep property definitions in insertion order (`db/store/store_cow.go`)
- [x] 9. Release terminal task transactions (`scheduler/task_runtime.go`)
- [x] 10. Do not turn flush failures into uncatchable task errors (`scheduler/task_runtime.go`)

Final gate:

- [x] Touched Go packages pass (`go test ./... -count=1`)
- [x] Full managed conformance suite passes (run `20260721_194411`: 11,412 passed, 146 skipped)
- [x] `git diff --check` passes
