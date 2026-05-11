# Barn/ToastStunt/Mongoose Convergence Workstream

## Fixed Point

Make Barn and ToastStunt converge on the real Mongoose workload by expanding
`../moo-conformance-tests` until the suite describes the ToastStunt behavior
Mongoose needs, then making Barn pass that expanded suite at matching profiles.

The stable loop is:

1. identify the profile and connection contract;
2. get MOO-visible control of Barn and ToastStunt through that contract;
3. use Mongoose to find an uncovered behavior;
4. reduce the behavior into `../moo-conformance-tests`;
5. verify ToastStunt passes that test at the named profile;
6. run Barn against the unchanged test at the matching profile;
7. keep either a coverage test, an unsupported-profile record, or the smallest
   Barn fix required by the Barn failure.

If an action does not move a behavior through that loop, it is not convergence
work.

## Repos And Ownership

- `C:/Users/Q/code/barn` owns Barn implementation, Barn runtime config, Barn
  profile metadata, and Barn-side MOO command tooling.
- `C:/Users/Q/code/moo-conformance-tests` owns behavioral truth.
- `C:/Users/Q/code/mongoose` supplies the real workload and its existing bridge
  is reference material for login/control patterns.

The existing Barn command surface for interactive MOO control is
`cmd/moo_client`. The workstream should extend that surface or a close sibling
only after reading it and the listener code it must drive. `../mongoose/bridge`
is an example of how to talk to a MOO, not a component to patch.

## Current Code Facts To Preserve

These facts come from the current Barn code and must shape the work:

- `cmd/barn` starts listeners from `-port` or repeatable `-listen` specs.
- Startup listener protocols are `tcp`, `tls`, `ws`, and `wss`.
- TCP input strips Telnet IAC sequences and terminates on CR or LF.
- Output is line-oriented on TCP/TLS and message-oriented on WebSocket.
- Login dispatch happens through the listener object, not only through `#0`.
- "The haproxy thing" means ToastStunt trusted-proxy login behavior around the
  HAProxy PROXY protocol. It is not a WebSocket issue.
- If the remote IP is listed in the database's
  `server_options.trusted_proxies`, ToastStunt suppresses ordinary blank-login
  behavior: blank initial input calls the listener's `do_blank_command`, and
  `do_login_command("")` runs only if that hook returns true.
- A pre-login line beginning with `PROXY ` is special only from a trusted remote
  IP. ToastStunt parses it, rewrites connection metadata, clears the login
  input, and then calls `do_login_command` with empty args and argstr.
- Barn currently has trusted-proxy blank-line handling and clears trusted
  `PROXY ...` input into empty login input; full Toast-style connection metadata
  rewrite is a known behavior surface to verify through conformance.
- `listeners()` exposes listener protocol, port, path, interface, TLS state,
  object, and print-message metadata from inside the MOO.

If later code inspection contradicts any of these facts, update this workstream
first and commit the correction before proceeding.

## Profile

A profile is the comparable runtime identity for one target. It includes:

- implementation: Barn or ToastStunt;
- implementation ref and build identity;
- database fixture identity and checksum;
- runtime platform;
- command used to start the server;
- connection contract;
- config/options/features;
- support status.

The connection contract includes:

- listener protocol and endpoint, normally TCP for ToastStunt comparison;
- whether the client IP is trusted by `server_options.trusted_proxies`;
- whether blank initial input should be consumed or passed through by
  `do_blank_command`;
- whether a `PROXY ...` prelude is expected, forbidden, or irrelevant;
- login script and account profile;
- expected first observable MOO output;
- how to verify the listener from inside the MOO, usually `listeners()`;
- whether the profile is `supported`, `unsupported`, or `diagnostic`.

No Barn/ToastStunt comparison is valid without a named profile on both sides and
a matching connection contract for the behavior being compared.

## Behavior Row

Every unit of convergence work is one row:

`behavior | profile | connection contract | Toast result | conformance test | Barn result | disposition`

Legal state transitions:

- `mongoose-observation -> toast-profiled-behavior`
- `toast-profiled-behavior -> conformance-test`
- `conformance-test -> barn-result`
- `barn-result -> barn-fix`
- `barn-result -> coverage-only`
- `barn-result -> unsupported-profile`
- `barn-result -> invalid-comparison`
- any final disposition -> `closed`

The next action is always the missing field for the active row. If there is no
active row, the next action is to create one from a concrete Mongoose behavior
or a known profile/config gap.

## Halt Conditions

Stop and report the exact unfinished row when any of these is true:

- The active profile cannot name its listener protocol, endpoint, and login
  account.
- A localhost or other trusted-IP connection produces no banner/login output and
  the next step is anything other than checking `trusted_proxies`,
  `do_blank_command`, and PROXY-prelude handling.
- A `PROXY ...` line is being used without first verifying that the database
  profile trusts the client's remote IP.
- The profile expects Toast-style PROXY metadata rewrite and the implementation
  under test only clears the login command without proving connection metadata.
- Barn cannot be started by the same lifecycle surface intended for repeated
  work.
- ToastStunt cannot be started by the same lifecycle surface intended for
  repeated work.
- Barn starts, but the selected client cannot obtain MOO-visible output through
  the profile's actual connection contract.
- ToastStunt starts, but the selected client cannot obtain MOO-visible output
  through the profile's actual connection contract.
- The client can connect at the socket level but cannot distinguish startup
  banner, login prompt, login failure, command output, timeout, and disconnect.
- The target database copy cannot be identified.
- Barn and ToastStunt are not using equivalent fixture inputs for a comparison.
- A Mongoose observation cannot be assigned to server behavior, database
  content, profile mismatch, client/transport behavior, or unsupported profile.
- The conformance harness cannot express the behavior being promoted.
- ToastStunt does not pass the candidate conformance test at the named profile.
- Barn does not fail the Toast-passing test and the planned next step is a Barn
  source change.
- The current slice would create a second truth surface outside
  `../moo-conformance-tests`.

These are not style rules. They are points where continuing would manufacture
unreliable work.

## Workstream

### 1. Recover Profile And Option Support

Recover the useful work from `exp/barn-options-profile-workstream` without
recovering the rejected bridge/artifact path.

Useful commits to inspect:

- `8327538`: runtime options parser;
- `ab0ebd4`: options wired through Barn execution;
- `9ff5357`: profile manifest emission;
- `fbf1155`: profile registry and Barn profile configs.

The first recovered option is `OUTBOUND_NETWORK`, because it is a real
Toast-sensitive runtime behavior and the prior branch already showed the shape.
The output of this slice is Barn config/profile support that changes runtime
behavior and is covered by Barn tests.

### 2. Make Profiles Visible To Conformance

Teach `../moo-conformance-tests` only the profile machinery required by actual
tests: feature requirements, profile manifests, matrix expectations, or invalid
comparison handling. Unknown feature metadata fails closed.

This slice is complete when a test can state the profile facts it depends on and
the managed harness refuses to compare mismatched profile facts.

### 3. Make MOO Control Match The Profile Contract

Extend `cmd/moo_client` or a close Barn-side sibling so an agent can drive the
selected profile contract:

- TCP line transport for ToastStunt-comparable MOO control;
- optional `PROXY ...` prelude before login when the profile requires it;
- explicit handling for trusted-proxy blank initial input: the tool must let the
  operator distinguish normal no-output from Toast's trusted-proxy login gate;
- scripted login/account selection from uncommitted local config;
- Barn-only, Toast-only, and send-both command execution;
- tagged output that lets the operator see which target produced each line.

The command tool proves control only when it can start from the named profile,
observe MOO-visible output, log in, send `look`, and show the response for both
Barn and ToastStunt through their declared connection contracts.

### 4. Discover With Mongoose

Use equivalent disposable Mongoose fixture inputs and the command tool to run
the same MOO-level actions on Barn and ToastStunt. Begin with behavior families
that Mongoose actually exercises:

- login and account selection;
- `look`, movement, contents, exits, and room rendering;
- parser shortcuts and `huh` behavior;
- `@who`, `@display`, `@props`, `@verbs`, and object inspection;
- connection metadata, listener metadata, trusted-proxy blank-line behavior,
  PROXY-prelude metadata rewrite, and reconnect/disconnect hooks;
- task scheduling, suspended reads, queued tasks, and persistence-visible
  workflows.

Discovery output creates behavior rows. It is not the authority for a Barn fix.

### 5. Promote To Conformance

For each real row, add the smallest useful test to `../moo-conformance-tests`.
Prefer `Test.db` or a reduced fixture. Use Mongoose as the fixture only when the
behavior cannot be reduced without losing the behavior.

The test must pass on ToastStunt first at the named profile. Then run Barn
against the unchanged test at the matching profile. If Barn passes, keep the
test as coverage. If Barn fails, the row enters the Barn fix slice.

### 6. Fix Barn From The Test

Read the failing conformance test, the profile facts, and the Barn failure.
Change the smallest Barn production surface needed for that behavior. Run the
focused test, then the relevant managed conformance suite. Commit the Barn fix
atomically.

If the profile is unsupported, do not edit Barn for that row. Record unsupported
profile debt instead.

### 7. Repeat By Behavior Family

Stay on one behavior family until rows in that family are closed, unsupported,
or invalid comparisons. Do not switch families because diagnostics are
interesting. The expected output over time is a larger conformance suite and a
Barn that passes it, not a pile of run artifacts.

## Completion Criteria

- Barn has committed config/profile support for the Toast-sensitive options
  required by the expanded suite.
- `../moo-conformance-tests` can represent the profile requirements used by the
  added tests.
- Barn-side MOO command tooling can drive Barn and ToastStunt through the
  selected TCP profile connection contracts, including trusted-proxy blank-line
  and PROXY-prelude cases when those profiles require them.
- Every accepted Mongoose-discovered behavior is either represented in
  `../moo-conformance-tests` or closed as database content, unsupported profile,
  diagnostic-only behavior, client/transport behavior, or invalid comparison.
- Barn passes the expanded managed conformance suite for every supported
  matching profile.
