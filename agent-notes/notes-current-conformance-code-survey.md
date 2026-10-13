# Current conformance code survey

- Scope: read-only survey of current executable code, configuration, and tests in `C:/Users/Q/code/moo-conformance-tests`; historical Markdown is excluded.
- State: current branch is `main` at `aa312b9`; the checkout has extensive pre-existing untracked artifacts, which are being left untouched.
- Findings so far: the package exposes a pytest plugin and CLI; the managed server copies a selected database into a temporary directory and supports placeholder substitution for port, database, manifest, and server directory; exact YAML selectors and strict runtime-skip rejection have unit coverage.
- Observation: the repository contains many generated and handwritten YAML families, so the next pass will quantify categories, schema constructs, gating, fixtures, and managed lifecycle contracts from source only.
- Blocker: none.
- Next action: inspect the core source in bounded chunks and collect line-numbered evidence without running Barn or Toast.
