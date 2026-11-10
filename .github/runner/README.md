# Mongoose Barn compute guest

This guest runs Barn's public fuzz workloads. It does not run the production
world. Its organization runner group is `barn-bug-discovery`, selected only for
`MongooseMoo/barn/.github/workflows/fuzz.yml@refs/heads/master` and
`MongooseMoo/barn/.github/workflows/fuzz-controller.yml@refs/heads/master`.
Routing, workers, acceptance and replenishment all use this group. Do not add its
listeners to the ordinary `barn` runner pool or the privileged repair workflow.

The guest has 8 virtual CPUs, 16 GiB RAM and a 120 GiB virtual disk. Its host
service caps memory at 20 GiB without swap and CPU at eight cores, with low CPU
and IO weight. A separate 260 GiB loop filesystem bounds guest/base/rollback
images together. The console journal has its own namespace capped at 256 MiB.
The host firewall table selects only the `barncompute` account. Public DNS and
HTTP(S) are allowed; host, private, metadata and IPv6 destinations are rejected.
Management SSH forwards only from host loopback port 22822. No host filesystem
share, production database, Docker socket or private SSH key enters the guest.

## Host installation

Use root SSH to the intended host. Check the live address, free disk/memory,
existing ports/accounts/paths and `/dev/kvm` before these commands. These paths
must be absent on first installation. Do not run an operating-system upgrade or
restart production services. Copy the reviewed files in this directory into
`/etc/barn-compute/` on the host, along with a deployment `user-data` edited from
`user-data.example` containing only the operator's **public** SSH key.

```sh
set -euo pipefail
DEBIAN_FRONTEND=noninteractive NEEDRESTART_MODE=l apt-get install -y --no-install-recommends qemu-system-x86 qemu-utils cloud-image-utils
useradd --system --user-group --home-dir /var/lib/barncompute --no-create-home --shell /usr/sbin/nologin --groups kvm barncompute
install -d -m 0755 /etc/barn-compute /var/lib/barncompute
install -m 0644 /etc/barn-compute/barn-compute.service /etc/barn-compute/barn-compute-network.service /etc/barn-compute/var-lib-barncompute.mount /etc/systemd/system/
install -m 0644 /etc/barn-compute/journald-barn-compute.conf /etc/systemd/journald@barn-compute.conf
nft --check --file /etc/barn-compute/egress.nft
systemd-analyze verify /etc/systemd/system/barn-compute.service /etc/systemd/system/barn-compute-network.service /etc/systemd/system/var-lib-barncompute.mount
test ! -e /var/lib/barncompute.img
truncate --size=260G /var/lib/barncompute.img
chmod 0600 /var/lib/barncompute.img
mkfs.ext4 -q -m 0 /var/lib/barncompute.img
systemctl daemon-reload
systemctl enable --now var-lib-barncompute.mount
chown barncompute:barncompute /var/lib/barncompute
```

Fetch the official Debian 12 genericcloud amd64 image. The deployment verified
the following SHA-512 against Debian's HTTPS `SHA512SUMS` and the downloaded file:

```text
0464b5d4ae3aa4f658468d88da83af7857b71548b30d46dcb6282770955a747fb95a983476db90d25afd0653af607224c7ebd05122ab71faa106fd712aaad652
```

The `latest` URL changes. Keep the digest check: a newer image requires an
explicitly reviewed digest update, rather than accepting a mismatch.

```sh
set -euo pipefail
curl -fL --output /var/lib/barncompute/base.qcow2 https://cloud.debian.org/images/cloud/bookworm/latest/debian-12-genericcloud-amd64.qcow2
test "$(sha512sum /var/lib/barncompute/base.qcow2 | cut -d' ' -f1)" = 0464b5d4ae3aa4f658468d88da83af7857b71548b30d46dcb6282770955a747fb95a983476db90d25afd0653af607224c7ebd05122ab71faa106fd712aaad652
chmod 0444 /var/lib/barncompute/base.qcow2
test ! -e /var/lib/barncompute/guest.qcow2
qemu-img create -f qcow2 -F qcow2 -b /var/lib/barncompute/base.qcow2 /var/lib/barncompute/guest.qcow2 120G
cloud-localds --network-config=/etc/barn-compute/network-config /var/lib/barncompute/seed.iso /etc/barn-compute/user-data /etc/barn-compute/meta-data
chown barncompute:barncompute /var/lib/barncompute/guest.qcow2 /var/lib/barncompute/seed.iso
systemctl enable --now barn-compute-network.service
systemctl start barn-compute.service
```

Wait for `cloud-init status` to report `done` inside the guest. Connect through
the operator's SSH agent/key on their own computer, using host SSH as a jump;
do not copy that private key to either machine. Check the guest host key through
the trusted management path and keep a separate known-hosts file.

## Listener installation and acceptance

Inside the guest, root runs `bash install-listener.sh SLOT` with a short-lived
organization registration token on stdin, with `listener.conf` and
`barn-fuzz.slice` in the same directory as the script. The script installs
the slice and the drop-in (`Slice=barn-fuzz.slice`, `Restart=always`,
`OOMPolicy=continue`), so a listener whose lane is OOM-killed, or which exits
for any reason, comes back on its own. All listeners share one memory bound,
`MemoryMax=14G` on the slice: a per-listener 3500M bound killed
FuzzReadDatabase, whose processes normally use about 3.4 GB. To apply both
to listeners installed before them, copy the two files to the guest as root
and run:

```sh
install -m 0644 barn-fuzz.slice /etc/systemd/system/barn-fuzz.slice
for s in 1 2 3 4; do
  d=/etc/systemd/system/actions.runner.MongooseMoo.mongoose-barn-fuzz-$s.service.d
  install -d -m 0755 "$d" && install -m 0644 listener.conf "$d/barn.conf"
done
systemctl daemon-reload
```

`Restart=`, `OOMPolicy=` and `Slice=` apply from each listener's next start;
restart a listener only while its runner is idle, or its running job is
cancelled. `python scripts/fuzz_ops.py status
--host` shows each listener's state and restart count, and
`python scripts/fuzz_ops.py runner restart N` starts a stopped one.

The script verifies the official
runner archive digest, registers the listener in the selected group, and runs
the installed service as `runner`, which has no sudo. It accepts slots 1–4;
start with two. Do not store a PAT, OAuth secret or organization management token
in the guest. A Windows CRLF token line is normalized before registration. An
already configured listener cannot be overwritten by the installation script.

The runner user cannot install packages. `user-data.example` installs the
ToastDifferential job's Toast build dependencies through cloud-init. The first
guest was provisioned before nettle-dev, libargon2-dev, libaspell-dev and
libjemalloc-dev were in that list, so root installed them afterwards with:

```sh
DEBIAN_FRONTEND=noninteractive NEEDRESTART_MODE=l apt-get install -y --no-install-recommends nettle-dev libargon2-dev libaspell-dev libjemalloc-dev
```

Read the group policy back from GitHub before assigning work. Prove both actual
master-workflow assignment and non-assignment for a disallowed ref/workflow while
the listeners are online; an accepted group setting alone is not that proof.
Keep `FUZZ_RUNNER_POLICY_VERIFIED` false/unset until this acceptance completes.
Never compensate for a failed dedicated assignment with workstation fallback.

Verify host cgroup limits, filesystem capacity, guest CPU/RAM/disk/no-swap,
absence of host shares and Docker socket, runner sudo denial, public GitHub
connectivity, and rejection of host/private/metadata connections. Check console
storage with `journalctl --namespace=barn-compute --disk-usage`. Guest runner
diagnostics and corpus storage consume the bounded guest disk. Preserve findings
before pruning anything; storage exhaustion is an infrastructure failure.

Complete one real fuzz cycle, restart the guest after jobs finish, replay retained
corpus, and finish a second cycle consuming it before enabling continuous mode.
Distinguish executed inputs and confirmed root causes from saved-input counts.

## Stop and recovery

Disable continuous replenishment first. Wait for or cancel this campaign's jobs,
stop the guest runner services, then stop `barn-compute.service`. Restarting this
guest is not a fix for a Barn defect. Preserve the guest disk and evidence during
investigation. Remove listeners using short-lived removal tokens only when
decommissioning them. Leave production services and ordinary CI runners alone.

After the guest stops, disabling its service and mount removes this compute
capacity. Its firewall can be removed with `nft delete table inet barn_compute`
only after no compute process remains; never flush the host ruleset. Do not
delete the storage image until findings and useful corpus have another verified
copy. Re-enable the mount and network units before restarting the guest.

Sources: [Debian cloud images](https://cloud.debian.org/images/cloud/bookworm/),
[QEMU invocation](https://www.qemu.org/docs/master/system/invocation.html),
[systemd execution controls](https://www.freedesktop.org/software/systemd/man/252/systemd.exec.html),
[GitHub runner groups](https://docs.github.com/en/actions/how-tos/manage-runners/self-hosted-runners/manage-access).
