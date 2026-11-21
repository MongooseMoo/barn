#!/usr/bin/env bash
# The outer process retains the GitHub publisher's identity. Codex runs as
# barn-codex with its own ChatGPT sign-in and no publisher environment.
set -euo pipefail
root=$(cd -- "$(dirname -- "$0")/.." && pwd -P)
cd /
exec flock -n /run/lock/barn-repair-worker.lock unshare --mount --propagation private bash -c '
  set -euo pipefail
  root=$1
  shift
  install -d -m 0755 /usr/local/libexec/barn-codex
  touch /usr/local/libexec/barn-codex/{moo,uv}
  mount --bind /root/src/toaststunt/build-release/moo /usr/local/libexec/barn-codex/moo
  mount --bind /root/.local/bin/uv /usr/local/libexec/barn-codex/uv
  if mountpoint -q /mnt/c; then umount -l /mnt/c; fi
  if mountpoint -q /mnt/wsl; then umount -l /mnt/wsl; fi
  exec python3 "$root/scripts/repair_worker.py" "$@"
' barn-repair-worker "$root" "$@"
