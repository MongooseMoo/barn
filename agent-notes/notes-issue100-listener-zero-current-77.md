# Current issue 100 listener-zero review state

- PR #101 exact pushed head: `80ea9b2645aa165cb812e914f2c601d4b07b417f`, based on Barn master `977f8cb04c1bfd522a6f553fcf71ef9b4012662a` through a normal two-parent branch merge.
- Local focused/full Go tests, build, formatting, and diff checks pass.
- Independent review found a real blocker: TCP/TLS and WebSocket accept paths call `NewConnectionFromTransport`, which publishes the connection in manager maps, before calling `SetListener`. Concurrent lookup can observe listener metadata as unset and use the startup-port fallback.
- The current builtin stub regression proves explicit zero versus unset semantics but does not prove accepted connections are published with descriptor metadata already initialized.
- Required repair: centralize accepted-connection construction so listener metadata is set before insertion into `connections` and `playerConns`; route both TCP/TLS and WebSocket through it; add a real accepted-connection regression that observes descriptor zero through the manager-visible connection.
- Managed paired run 31059517935 is still in progress. It has passed trusted-controller quality and is running all three Toast profiles before the Barn pair.
- Barn legacy CI passes build/unit and reaches the packaged suite after admission, then fails on independent pre-existing packaged failures and a later crash cascade; those fixes must not be imported into PR #101.
- Blocker: producer publication ordering and missing producer-path regression.
- Next action: implement the atomic publication helper and real acceptance test, rerun focused/race/full/build gates, push a new exact head, supersede the paired run, and request re-review.
