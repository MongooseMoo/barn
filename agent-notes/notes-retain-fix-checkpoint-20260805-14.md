# Retain/fix checkpoint 14

- Barn PR #96 is repaired and pushed at `4f6e82878be71b9be289a80c268e98a100ed9285`; build/unit is green and independent exact-head review is running.
- PR #96's legacy conformance failure was inspected from the job log: it is the known old unprofiled workflow plus pre-integration bcrypt failures and the resulting server/probe cascade. It is not being merged alone.
- moo-conformance-tests PR #75 is the clean three-file Anon7 data PR at `c3dbe5b1ee5c8068561f57b598a418305f235c20`, with literal `Closes #71`; protected Toast matrices are running.
- Mixed PR #72 is closed unmerged with an accurate supersession comment.
- Next: finish independent reviews and protected CI, normal-merge #75 with two-parent verification, merge #96 history into Barn integration PR #115, rerun the corrected full default-profile gate, then audit and normal-merge only after green.

