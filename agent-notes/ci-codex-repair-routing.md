# Codex repair routing

## Goal

Run Barn repair, conformance and alarm work using Claude or Q's Codex subscription. Show open work on the existing live page.

## State

- `.github/workflows/claude-fix.yml` and `alarm-investigate.yml` use `repair-route` for provider selection. `stage=conformance` accepts an existing Barn PR number. Known Claude provider failures can hand off to Codex. Generic action failures remain unknown.
- `scripts/run_repair_worker.sh` runs the publisher and starts Codex under the separate WSL `barn-codex` account. The worker requests ChatGPT authentication and sends no API key to Codex. It checks the recorded PR head, scope and required checks before requesting the merge queue.
- Open work is visible in `docs/live/`, including Codex repair, conformance and alarm queue labels. The live Actions link opens the provider and stage dispatch controls.
- The WSL Codex account completed a subscription-backed smoke turn. Q confirmed the private App installation on the two repositories. Its token returned exactly those repositories, and worker label initialization succeeded.
- The live dispatch added `codex-repair` to #987 and the worker took it. Codex recorded `cyber_policy` and produced no repair. #987 is held with `repair-needs-attention`; the queue label was removed. Policy and unclassified outcomes now stop the attempt without provider fallback.

## Decisions and why

- Enable routing only after the WSL worker has ChatGPT sign-in and a GitHub App installation token. The public Actions runner never receives subscription credentials. The worker's GitHub App token stays in the publishing process.
- The App needs access to `MongooseMoo/barn` and `MongooseMoo/moo-conformance-tests`, with Actions, Contents, Issues and Pull requests write plus Checks and Administration read. Store `client_id`, `installation_id`, and `key_file` in root-only `/etc/barn-repair/github-app.json`. Store the app key outside the checkout with mode 0600.
- Run the launcher from a trusted Linux checkout with the Debian `bubblewrap` package and the native Codex binary at `/usr/local/libexec/barn-codex/codex`. The launcher exposes the existing WSL Toast build to the worker and hides Windows mounts from the private worker namespace. The Codex command profile allows the managed harness's loopback and dependency registries. Initialize labels with `scripts/run_repair_worker.sh --initialize-labels` before setting `BARN_CODEX_WORKER_ENABLED=true`. Run one subscription worker with `scripts/run_repair_worker.sh --interval 60`.

## Surprises

- Claude repair run 44882831976 reported `is_error:true`, `modelUsage:{}`, and no downloadable artifact. That output does not establish a usage limit.
- GitHub's self-repository action syntax uses the workflow revision, including when the PR's head is checked out. The repository's existing actionlint release predates the syntax. The affected action and workflow audit uses zizmor.
- This WSL installation places the target of `/etc/resolv.conf` under `/mnt/wsl`. The launcher snapshots that resolver into its private namespace so detaching `/mnt/wsl` no longer breaks App authentication. The Claude actions explicitly allow the installed App bot to dispatch fallback work.
- The scheduled launcher uses `umask 077`. The private input and schema files are created exclusively with mode 0600 and assigned to the model account so the same job can start from the scheduled launcher.

## Next step

Verify the conformance stage on the ordinary compiler repair in merged PR #919. Its already-merged companion is MongooseMoo/moo-conformance-tests #231. Verify an eligible live repair before enabling default-branch routing and finishing the integration PR.
