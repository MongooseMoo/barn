# Preserve failed goodput samples privately

## Goal

Keep the evidence for a failed workstation goodput sample after temporary files are removed. Keep the harness failure and the successful public measurement path.

## State

Runs 46126044879 and 46099117988 reported failed harness samples, successful cleanup, and zero artifacts. The workflow now saves the failed sample in `~/.local/state/barn/goodput-failures/run-<run-id>-<attempt>.tar.gz` for the workstation runner account. The public error names the runner and file. The safe public point artifact still requires all samples to complete.

## Decisions and why

- Keep the newest ten compressed archives. Each can contain the last 16 MiB of raw harness output, the last 16 MiB of JSON diagnostics if those were enabled, and the run, fixture, sample, and exit metadata. The directory and archives use modes `0700` and `0600`.
- Preserve the existing log-dir opt-in without changing the standard measurement's instrumentation. When enabled, the collector writes to a separate temporary directory per sample, which the existing cleanup removes after archiving the failed sample.
- Allow only the measurement's numeric fields and fixed test verdict tokens through the public log filter. The archive contains the original diagnostics and excludes the copied database and executable.

## Surprises

The previous public filter allowed arbitrary suffix text after the goodput prefix. The synthetic case puts a private sentinel after that prefix and verifies it is absent from the public output and present in the retained evidence.

## Next step

Publish through the owner and watch workflow CI. Local evidence: `actionlint` exit `0`; the extracted production step with a synthetic harness returned `exit=7`, `raw_and_json_retained=true`, `public_private_text=false` and `survives_cleanup=true`. A successful sample published `complete=true` and created no archive. A blocked archive path kept `exit=7` and reported the retention failure.
