# Consulting Claude Fable

Run the external reviewer from this checkout with a bounded prompt file:

```powershell
./scripts/consult-fable.ps1 -PromptPath prompts/fable-scheduler-gate-20260918.md
```

The script selects `--model fable`, creates a session UUID, prints it, and saves
the CLI's JSON response and stderr under `.tmp/fable-consultations`. It waits for
the review to finish and reports a nonzero exit or an error result. The prompt
specifies which report the reviewer may write; inspect that report afterward.

Challenge the result in the same conversation with another prompt file:

```powershell
./scripts/consult-fable.ps1 -PromptPath prompts/<followup>.md -Resume -SessionId <printed-uuid>
```

The CLI runs noninteractively with permission checks bypassed, following the
external-agents protocol. Keep the prompt's authorized scope explicit: named
input sources, owned report path, read-only runtime/source access, and whether
any implementation is requested. Do not put credentials in review prompts.
The script itself does not stage or commit the report.

Use file-backed prompts for reproducibility, record the source revision, and
distinguish source-proven behavior from proposed policies and measured results.
For scheduling reviews, ask for timelines that include execution slots, commit
gate ownership, shared committers, lifecycle barriers, checkpoints, and
cancellation. A favorable ready-queue ordering alone is not a latency guarantee.
