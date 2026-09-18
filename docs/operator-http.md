# Operator and debug HTTP listeners

Barn exposes two independently configured HTTP listeners with different trust
boundaries:

* `--operator-addr` serves only the cheap, passive `GET`/`HEAD /livez` and
  `GET`/`HEAD /readyz` probes. Liveness says the application loop is running;
  readiness becomes true only after player listeners bind and
  `#0:server_started()` completes, and is withdrawn before shutdown.
* `--debug-addr` exclusively serves `/debug/vars`, `/debug/pprof/*`, and the
  mutating `/debug/loglevel` endpoint. These diagnostics can be expensive or
  sensitive and must not be exposed as a passive monitoring surface.

Both flags default to `127.0.0.1:0`, a loopback address with an ephemeral port.
Barn logs each actual bound address. Set either flag to `off` to disable that
listener. A fixed port or non-loopback bind is an explicit operator choice.

Neither listener provides TLS or authentication. Do not expose either directly
to the Internet. Keep them behind a trusted local or private network boundary,
or use an authenticated TLS reverse proxy. In particular, keep the debug
listener restricted to administrators even when the operator listener is made
available to a scraper or orchestrator.
