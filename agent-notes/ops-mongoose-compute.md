# Isolated Mongoose bug-discovery compute

## Goal
Provide the approved isolated compute guest on mongoose.world for Barn bug discovery.

## State
Implementation authorized by Q's “Make it so!”. Host survey found Debian 12,
KVM available, 48 logical CPUs, and sufficient free disk.
The separate GitHub runner group accepts only the master fuzz workflow; runtime
policy enforcement remains unverified. QEMU and cloud-image-utils installation
completed. The first apt invocation waited for confirmation; its exact PID was
verified and terminated, then the corrected noninteractive invocation completed.
No production service restart was requested. The guest has booted and cloud-init
completed. Runtime evidence shows 8 CPUs, about 16 GiB RAM, no guest swap,
120 GiB virtual guest disk and a 260 GiB host backing filesystem. Host limits
read back as CPU quota 8 seconds/second, MemoryMax 20 GiB, MemorySwapMax zero.
Guest TCP probes reached api.github.com:443 but timed out for the host SSH alias,
production Barn address, Docker private address, and metadata address. The runner
user has no sudo. File review found no blocking configuration defect. Two listener
services are registered and online as mongoose-barn-fuzz-1 and -2 in the selected
group. Both guest services report active. Runtime GitHub allow/deny assignment
policy acceptance remains unverified; no dedicated fuzz cycle has run yet.
Differential PR 654 and persistence PR 655 are published with required CI running.

## Decisions and why
Use one KVM guest with 8 vCPUs, 16 GiB RAM, a 120 GiB disk, and a dedicated
unprivileged host account. Bound host memory to 20 GiB without swap and CPU to
eight cores. No production mounts, host Docker socket, or broad credentials.
Allow public package and GitHub egress while rejecting host/private destinations.
Keep continuous discovery disabled until isolation and retained-corpus acceptance.

## Surprises
The initial package invocation omitted apt's yes option. The corrected call used
noninteractive installation and needrestart list-only mode; no packages upgraded.
The Debian cloud image checksum was fetched directly from the official host.
Windows CRLF on the token stdin initially prevented runner registration. The
installation script now strips the trailing carriage return, never overwrites
configured listeners, and permits a verified-archive retry of an unconfigured
directory. Registration then succeeded without storing broad credentials.
An automated risk filter rejected the workflow worker twice. This is authorized
bug discovery for Q's own MOO server; the actual commit log includes existing
structured fuzzing and the merged fixes it found. The worker has resumed its
ordinary fuzz workflow integration. No cybersecurity campaign is part of the task.

## Next step
Publish the reviewed provisioning files and README as an independent PR, then
verify actual GitHub assignment restrictions using controlled workflow jobs.
Keep dedicated policy verification and continuous mode disabled until live proof.
Confirm differential/persistence PR checks and reviews separately.
