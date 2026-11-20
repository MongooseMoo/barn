# Current GitHub inventory checkpoint

- Repository: `MongooseMoo/barn`, default branch `master`.
- Live open state: 62 issues and one PR as of 2026-08-04.
- No open issue has labels, assignees, or milestones.
- The sole open PR is #81, `Fix reseed_random shared generator`, closing #4. It has one commit (`e2fc365`), four changed files, no reviews, and is mergeable but `UNSTABLE`: unit/build succeeds and conformance fails one current test (`index_call_shapes::index_third_arg_str`, expected int but Barn returns an E_INVARG traceback plus `{1, 2}`).
- Verified overlap clusters so far: coarse transaction atomicity/retry (#29, #45, #77); task dispatch and locking (#30, #59, #63, #64, #76); connection/input/transport (#32, #57, #58, #60, #61, #62); SQLite suspension/blocking (#65, #66); bytecode 16-bit bounds (#69, #70); byte-string semantics (#67, #68); configured value limits (#49, #71, #73); crypto behavior (#4, #44, #72, #73, #74); parser denial of service (#25, #50, #51); checkpoint/load integrity (#5, #28, #42, #43); random family (#4, #15); exec three-argument handling (#47, #53); map performance/equality (#40, #56).
- Highest-risk current issues include privilege escalation #13; remote/process crashes #14 and #25; checkpoint data loss #28; coarse-store corruption #29/#77; task double-dispatch #30; and cross-player connection confusion #32.
- Current work is read-only live GitHub inspection. No Git/GitHub changes were made.
- Next action: finish reading the remaining live issue bodies and comments, verify all link/dependency relationships, then deliver a complete clustered inventory to the parent agent.
