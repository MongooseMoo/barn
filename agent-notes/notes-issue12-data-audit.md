# Issue 12 data-only audit checkpoint

- Scope: read-only audit of the three YAML cases from moo-conformance-tests PR 22 against current main e9b6532c5dd9f46104739f4671137e38bc95363e.
- Restrictions observed: no Markdown, notes, plans, or reports read; no servers launched; no source or YAML edited.
- Repository identity: MongooseMoo/moo-conformance-tests, not the current Barn checkout origin.
- Current main: e9b6532c5dd9f46104739f4671137e38bc95363e is the two-parent merge of PR 62, which promoted the controller-only issue 12 implementation.
- Old PR 22 YAML file: src/moo_conformance/_tests/features/steps_basic.yaml.
- Intended additions: top_level_satisfies_binds_result_value, step_satisfies_binds_result_value, step_notifications_are_enforced.
- Defect in old diff: 22 additions and 5 deletions; five existing CRLF lines were rewritten with LF, including the multi_permission_steps name, its cleanup lines and following comment.
- Required minimal delta: add only the three new YAML mappings between the existing multi_permission_steps cleanup and skip_if_builtin_missing_but_exists comment, preserving every existing byte and CRLF line ending.
- Collection ID rule on current main: relative YAML path plus double colon plus test name.
- Expected classifier: mode data, one data path, zero controller paths, zero neutral paths.
- Current workflow quality gates include trusted harness pytest, Ruff, mypy, candidate inventory, collect-only, duplicate lint, startup-fixture validation, and managed Toast profiles.
- Blocker: none.
- Next action: verify raw-byte EOL counts, exact collected IDs and focused selector spelling, then deliver final audit to parent.
