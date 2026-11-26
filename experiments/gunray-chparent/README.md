# Test MOO properties with Gunray

From a Barn checkout, run one authored model with Python 3.11+ and uv. The model follows the active verb's ancestors and predicts the inherited implementation named `alpha`.

```bash
cd experiments/gunray-chparent
gunray=git+https://github.com/ctoth/gunray.git@ccd6a06906c032fb85be12df9d26c2df0cbfc5c0
uv run --no-project --with "$gunray" gunray run inherited-pass.gnr --query winner
# {"winner":[["alpha"]]}
uv run --no-project --with "$gunray" python handler_revision.py
```

### What changes when a handler is added?

The group rule accepts `E_TYPE` and `E_RANGE`. The narrower typed rule accepts `E_TYPE`. Their claims conflict when both apply. The first five scenarios change available rules or precedence with identical facts.

| Rules and preference | Error | Warranted | Undecided |
|---|---|---|---|
| Group alone | E_TYPE | group | — |
| Add typed, prefer typed | E_TYPE | typed | group |
| Add typed, prefer typed | E_RANGE | group | — |
| Prefer group over typed | E_TYPE | group | typed |
| Both, no preference | E_TYPE | — | typed and group |

The first two rows demonstrate loss of a previously warranted conclusion. Two more scenarios keep the same two rules, conflict and typed-over-group priority. Both rules require a matching error and a fact stating the handler is present. Initially only `present("group")` holds and group is warranted. Adding exactly `present("typed")` makes typed warranted and group undecided. The script checks that the fact set grows by one and the rest of the theory stays identical. Six scenarios generate MOO programs. Both VMs choose the first matching `except` clause. The unranked control exercises Gunray alone because MOO gives every clause a source position.

### Compare the predictions with both VMs

Use Bash on Linux or WSL with uv, a Barn build and the stock Toast executable for the repository's oracle profile. Set `TOAST_BINARY` to that executable's absolute path. Starting in the experiment directory above:

```bash
run=$(mktemp -d)
git clone -q https://github.com/MongooseMoo/moo-conformance-tests.git "$run/suite"
git -C "$run/suite" checkout -q abaa76a05ce798d57aa65dcac11bf72d64463de3
mkdir -p "$run/candidate/cases" "$run/evidence"
cp -R "$run/suite/src/moo_conformance/_db" "$run/candidate/db"
# Set BARN_BINARY to an existing build, or build this checkout.
(cd ../.. && go build -o "$run/barn-current" ./cmd/barn)
uv run --no-project --with "$gunray" python handler_revision.py \
  --suite "$run/candidate/cases/gunray_handler_revision.yaml"
bash run-managed.sh "$run" model toast gunray_handler_revision
bash run-managed.sh "$run" model current gunray_handler_revision
```

The runner executes capability admission in the same session. Each engine writes the command, admission JSON, raw calls and returned values, JUnit results and log under `$run/evidence/model-{toast,current}`. Give each new run a fresh phase or directory to preserve its evidence.

### Run the semantic hunt

```bash
uv run --no-project --with "$gunray" python ten_hunt.py generate "$run" gunray_ten_full
bash run-managed.sh "$run" hunt toast gunray_ten_full
bash run-managed.sh "$run" hunt current gunray_ten_full
# A failing MOO assertion gives the runner exit status 1. Compare both completed sessions.
uv run --no-project --with "$gunray" python ten_hunt.py compare "$run" gunray_ten_full hunt
```

Gunray evaluates the source-selected hypotheses into expected values. The two engines provide independent observations. The full CSV records 60 cases and 37 Barn mismatches at Barn `926c6b4`. The reduced CSV records 12 repeated discrepancies and ten passing controls. Eleven discrepancies were new to this investigation. Barn PR #123 had already documented suffix-relative reverse matching. The stock oracle binary had SHA-256 `61dd1799d82f75e33cb9c42fe48bd1966199ad83fda2f6298e3a7f872cc613dd`. This hunt used the bounded semantic laws in `ten-laws.gnr`; the defeasible handler experiment tests a separate question.

### Earlier property families

Use a separate prepared run directory per row to retain its predictions. Run `uv run --no-project --with "$gunray" python GENERATOR.py "$run"`, then use the suite name as the final argument to `run-managed.sh`. The two short `*_replay.yaml` files can also be copied directly to `$run/candidate/cases/` and selected by their stem.

| Generator | Managed suite | What it varies |
|---|---|---|
| experiment | gunray_499 | Inherited property values after reparenting |
| paired | gunray_pairs | Pairs of reparent, write and clear histories |
| roundtrip | gunray_roundtrip | Partitions of a property history into tasks |
| vm_cases | gunray_vm | Dispatch, containment and finally transfers |
| parent_order | gunray_order | Ordered parents and inherited verb dispatch |
| command_finally | gunray_cf | Parsed command selection and finally transfers |
| inherited_pass | gunray_inherited_pass | Changes of parent during inherited calls |
| assignment_order | gunray_assignment_order | Evaluation, faults and suspension in assignment |
| range_handlers | gunray_range_handlers | Range writes, ordered handlers and rethrows |
