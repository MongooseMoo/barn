# network-connection-options

## Goal

Make `listen` and `open_network_connection` accept and honour the options the
Mongoose ToastStunt fork (`mongoosemoo/mongoose`, 0ff6fd7) accepts: `listen`
`tls`/`certificate`/`key`; `open_network_connection` `tls`, `tls_verify`,
`ipv6`, `listener`. Options that need a design decision are reported, not built.

## State

Done and committed locally (not pushed):
- Test `tests/mongoose-conformance/builtins/network_connection_options.yaml`
  (6 cases, loopback only): passes on the fork Toast 0ff6fd7, fails 5 of 6 on
  Barn before, passes 6 of 6 after.
- `listen`: option keys matched in any case; `tls` true makes a TCP listener
  TLS; `certificate`/`key` without `tls` are ignored; a non-string path raises
  `(E_INVARG, "Certificate path should be a string" | "Private key path should
  be a string", 0)`, key message last; the key pair is read at each handshake
  (startup listeners still refuse an unloadable pair at bind).
- `open_network_connection`: `ipv6` (any case) chooses the address family;
  default IPv4 only. A host with no address in that family raises
  `(E_INVARG, "getaddrinfo error", detail)` via `builtins.HostLookupError`.
- `tls_verify`: the fork ignores it; Barn ignores it. Covered by a test.

## Decisions and why

- `listen` `tls` without both paths: not done. The fork falls back to a compiled
  default (`/etc/letsencrypt/live/...`) or `-r`/`-k`; Barn has no default
  certificate setting. Where Barn should find one is a decision for Q.
- `open_network_connection` `tls` and `listener`: not done. Barn has no outbound
  connection object: `ConnectionManager.OpenNetworkConnection` waits for its own
  listener to accept the dialled socket and returns that accepted end; any other
  host fails after 2 s. Both options act on the outbound connection itself, so
  honouring them first needs a real outbound connection over the dialled socket.
  That rewrite changes the builtin for every caller and is a decision for Q.
- Error order: Barn still checks wizard before options; the fork parses options
  first (a non-wizard with a bad path gets E_INVARG there, E_PERM here). Not
  changed.

## Surprises

- `net.Resolver.LookupIP` reports a literal of the wrong family as
  `*net.AddrError`, not `*net.DNSError`; both become `HostLookupError`.
- `git` in WSL cannot read this worktree (its `.git` file holds a Windows path).

## Next step

Q decides the two open items above. Report:
`C:\Users\Q\projects\mongoose\reports\barn-switch-work-09-barn-network-options-2026-11-13.md`.
