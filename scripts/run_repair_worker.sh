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
  resolver=$(readlink -f /etc/resolv.conf)
  if [[ "$resolver" == /mnt/wsl/* ]]; then
    resolver_contents=$(<"$resolver")
  fi
  if mountpoint -q /mnt/c; then umount -l /mnt/c; fi
  if mountpoint -q /mnt/wsl; then umount -l /mnt/wsl; fi
  # WSL can place the resolver behind a symlink into the detached mount.
  if [[ "$resolver" == /mnt/wsl/* ]]; then
    mount -t tmpfs -o mode=0755,nosuid,nodev,noexec tmpfs /mnt/wsl
    mkdir -p -- "$(dirname -- "$resolver")"
    printf "%s\n" "$resolver_contents" > "$resolver"
    chmod 0644 "$resolver"
  fi
  exec python3 "$root/scripts/repair_worker.py" "$@"
' barn-repair-worker "$root" "$@"
