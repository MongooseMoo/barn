#!/bin/bash
# Run as root inside the compute guest; registration token arrives on stdin.
set -euo pipefail
test "$(id -u)" = 0
test "$(hostname)" = barn-compute
test "$(systemd-detect-virt)" = kvm
slot=${1:?listener slot required}
case "$slot" in 1|2|3|4) ;; *) exit 2 ;; esac
read -r registration_token
registration_token=${registration_token%$'\r'}
test -n "$registration_token"
version=2.338.2
digest=baebf204307bac6193480852732f0aba2ae97b6c5597c244f4d5558390147c40
archive=/opt/actions-runner-linux-x64-${version}.tar.gz
directory=/opt/actions-runner-${slot}
# A failed registration may leave an unpacked directory; never overwrite a
# configured listener. Re-extracting the verified archive repairs that case.
test ! -e "$directory/.runner"
if [ ! -f "$archive" ]; then
    curl --fail --location --output "$archive" "https://github.com/actions/runner/releases/download/v${version}/actions-runner-linux-x64-${version}.tar.gz"
fi
printf '%s  %s\n' "$digest" "$archive" | sha256sum --check
install -d -o runner -g runner "$directory"
tar -xzf "$archive" -C "$directory"
chown -R runner:runner "$directory"
cd "$directory"
# The short-lived token is the only GitHub registration credential in the guest.
runuser -u runner -- ./config.sh --unattended --url https://github.com/MongooseMoo \
    --token "$registration_token" --name "mongoose-barn-fuzz-${slot}" \
    --runnergroup barn-bug-discovery --labels barn-fuzz --work _work
unset registration_token
./svc.sh install runner
./svc.sh start
