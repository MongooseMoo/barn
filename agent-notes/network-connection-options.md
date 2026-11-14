# network-connection-options

## Goal

Make `listen` and `open_network_connection` accept and honour the options the
Mongoose ToastStunt fork (`mongoosemoo/mongoose`, 0ff6fd7) accepts: `listen`
`tls`/`certificate`/`key`; `open_network_connection` `tls`, `tls_verify`,
`ipv6`, `listener`. Part 2 (team lead's go-ahead): replace the self-connection
lookup with a real outbound connection so `tls` and `listener` have something
to act on.

## State

Done and committed locally (not pushed):
- Tests `tests/mongoose-conformance/builtins/network_connection_options.yaml`
  and `open_network_connection_outbound.yaml`, loopback only, both passing on
  the fork Toast 0ff6fd7 before any Barn change, failing on Barn before, passing
  after.
- `listen`: keys in any case; `tls` makes a TCP listener TLS; certificate/key
  without `tls` ignored; non-string paths raise the fork's E_INVARG messages;
  key pair read at each handshake (startup listeners still checked at bind).
- `open_network_connection`: a real outbound `Connection` over the dialled
  socket (any host, not only Barn's own listeners); `ipv6` picks the family
  (IPv4 by default); `tls` makes it a TLS client; `listener` sets its listener
  object and gives it an initial empty login line; without a listener, no
  initial line and listener #0; outbound connections never get the connect
  timeout; `connection_info` source fields "" / 0. The old matching loop,
  `outboundClients` and `detachOutboundClient` are deleted.
- `tls_verify`: unused, as in the fork.

## Decisions and why

- Team lead, 2026-11-13: `listen` `tls` without both paths stays refused
  (E_INVARG); no default-certificate config.
- Team lead, 2026-11-13: outbound TLS verifies chain AND host name (Go
  default). Listed difference: the fork verifies the chain only
  (options.h:212 `VERIFY_TLS_PEERS`, network.cc:1278-1283, SNI only at
  network.cc:1142). Reverse with InsecureSkipVerify plus a chain-only
  VerifyPeerCertificate in `server/connection_manager.go` `dialOutbound`.
- `outboundTLSRoots` on ConnectionManager (nil = system roots) exists so Go
  tests can trust a generated certificate; production never sets it.
- No loopback TLS conformance test: on the fork a TLS connection to its own
  listener cannot complete (blocking SSL_connect inside the builtin while the
  single-threaded loop cannot accept). TLS is covered by Go tests.

## Surprises

- The self-connection model came from the 2026-04-02 commit "Implement real
  listener and outbound connection admin behavior", to pass server_admin tests
  that dial the server's own port; reports/review-server.md already flagged it
  (SUSPECTED-1, ARCH-1).
- `net.Resolver.LookupIP` reports a wrong-family literal as `*net.AddrError`.
- A Go `tls.NewListener` conn handshakes only on first I/O; a test peer must
  call `Handshake()` or the client waits until its deadline.
- `git` in WSL cannot read this worktree (its `.git` file holds a Windows path).
- Remaining differences: own-listener numbering order can differ (Barn accepts
  concurrently); with several listeners on the `listener` object Barn takes any
  one's print-messages; `connection_info` has no `TLS` key.

## Next step

None in this assignment. Report:
`C:\Users\Q\projects\mongoose\reports\barn-switch-work-09-barn-network-options-2026-11-13.md`.
