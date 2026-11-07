---
title: "Dapper, a Large-Scale Distributed Systems Tracing Infrastructure"
authors: "Benjamin H. Sigelman and colleagues"
year: 2010
venue: "Google Technical Report dapper-2010-1"
---

# Dapper

Read from all 14 locally rendered page images on 2026-11-06 using the
research-papers paper-process light flow. Primary PDF is retained locally.

## Problem and contributions

An overall slow request does not identify which component delayed it. Shared
resources also mean that another request can cause the observed delay. The
system follows causal work with an identity propagated through common execution
and communication libraries. Its stated goals are low overhead, transparency,
scalability and timely access to evidence. *(pp.1-2)*

## Data model and protocol

1. Assign a root trace ID. Each span has a name, span ID and parent ID; spans
   contain timestamped events, start/end and optional annotations. Parent links
   express causality; timestamps describe temporal relationships. *(p.3)*
2. Propagate context through threads, queued callbacks and RPC boundaries.
   Asynchronous callbacks retain their creator's context. APIs outside the
   supported control-flow libraries need explicit propagation. *(pp.4,6)*
3. Record client send/receive and server receive/send. Their ordering bounds
   timestamps despite cross-host clock differences. *(pp.3-4)*
4. Write local trace data, collect it using per-host daemons, then assemble
   spans in a repository keyed by trace ID. Collect outside the request path:
   downstream work can outlive its initiating request. *(pp.4-5)*
5. Bound annotation volume; preserve structural records despite annotation
   truncation. Record method identity without RPC payload by default. *(pp.4-5)*

## Parameters and measurements

These are historical measurements of Google's implementation, not Barn targets.

| Name | Units | Default or observed | Range | Page | Context |
| --- | --- | --- | --- | --- | --- |
| Trace/span IDs | bits | 64 | - | 3 | Probabilistic unique identities |
| Desired analysis freshness | minute | 1 | - | 2 | Design goal |
| Median collection delay | seconds | below 15 | - | 5 | Production |
| Fraction of p98 collection under two minutes | percent | about 75 | - | 5 | Remaining tail can take hours |
| C++ core instrumentation | lines | below 1000 | - | 5 | Excludes annotation API |
| Java core instrumentation | lines | about 800 | - | 5 | Excludes annotation API |
| Annotation implementation | lines | about 500 | - | 5 | Additional code |
| Manual propagation C++ applications | applications | 40 | - | 6 | Nonstandard control flow |
| Manual propagation Java applications | applications | 33 | - | 6 | Nonstandard control flow |
| Traces using an annotation | percent | 90 | - | 6 | Observed trace population |
| Spans using an annotation | percent | 70 | - | 6 | Observed span population |
| Applications adding annotations, Java | applications | 41 | - | 6 | Reported adoption |
| Applications adding annotations, C++ | applications | 68 | - | 6 | Reported adoption |
| Root span creation/destruction | ns | 204 | - | 6 | 2.2 GHz x86 |
| Non-root span creation/destruction | ns | 176 | - | 6 | Same machine |
| Unsampled annotation lookup | ns | 9 | - | 6 | Thread-local lookup |
| Sampled literal annotation | ns | 40 | - | 6 | Same machine |
| Daemon CPU at 25 processes and 10K events/s/process | percent core | 0.125 | - | 7 | Synthetic load |
| Daemon CPU at 10 processes and 200K events/s/process | percent core | 0.267 | - | 7 | Synthetic load |
| Daemon CPU at 50 processes and 2K events/s/process | percent core | 0.130 | - | 7 | Synthetic load |
| Average span network size | bytes | 426 | - | 7 | Production |
| Collection fraction of network traffic | percent | below 0.01 | - | 7 | Production |
| Initial uniform sample probability | probability | 1/1024 | - | 7 | High-volume services |
| Repository trace generation | TB/day | above 1 | - | 8 | Sampled traces |
| Desired repository retention | weeks | at least 2 | - | 8 | Additional collection sampling |
| Composite index storage | percent data size | 26 | - | 8 | Service, host, timestamp |
| Persistent DAPI applications | applications | 3 | - | 9 | Reported known users |
| On-demand DAPI tools | tools | 8 | - | 9 | Reported known users |
| One-off DAPI analyses | tools | about 15-20 | - | 9 | Lower bound, untracked use |
| Weekday UI users | users | about 200 | - | 9 | Typical weekday |
| Weekly distinct UI users | users | about 750-1000 | - | 9 | Approximate |

| Sampling probability | Mean latency change (%) | Throughput change (%) | Page |
| --- | --- | --- | --- |
| 1 | 16.3 | -1.48 | 7 |
| 1/2 | 9.40 | -0.73 | 7 |
| 1/4 | 6.38 | -0.30 | 7 |
| 1/8 | 4.12 | -0.23 | 7 |
| 1/16 | 2.12 | -0.08 | 7 |
| 1/1024 | -0.20 | -0.06 | 7 |

Experimental errors are 2.5% for latency and 0.15% for throughput; small changes
are within error. Logging every request materially changes latency in this
workload even though throughput changes much less. *(p.7)*

## Sampling and access algorithms

Adaptive sampling targets a number of traces per time unit so quiet services
remain observable while busy services limit cost. Record the actual probability
with each trace for frequency estimates. A second collection-stage decision
hashes the trace ID to a value z in [0,1] and retains the trace when z is below
the collection fraction, preserving whole traces rather than unrelated spans.
This is the paper's only explicit numeric algorithmic inequality. *(pp.7-8)*

The API supports exact-ID access, bulk analysis over time windows and indexed
access. The UI selects a service/window/metric, groups execution patterns, then
shows histograms and examples with a nested timeline. Fresh traces can be read
from daemons for incident investigation. *(pp.8-9)*

## Experience and figures

Figures 1-3 illustrate request trees, causality and timestamped spans; Figure 4
shows annotation calls; Figure 5 gives the collector pipeline; Figure 6 shows
the histogram-to-example UI workflow. *(pp.2-5,10)*

Ads review used traces during development, testing, correctness analysis and
performance work. The team estimated two orders of magnitude improvement,
without a controlled attribution study. Exceptions carry trace/span identities
to link failures to surrounding execution. *(pp.9-11)*

Figure 7 shows the proportion of requests encountering network degradation
increasing with end-to-end latency, approaching nearly all of the slowest
requests. It is observational evidence, not proof about Barn networking.
Throughput alone can conceal an outlier-latency problem. *(p.11)*

Other uses include service dependency discovery, attributing shared storage and
network cost to callers, and incident triage. Bulk shared-service analyses are
less immediate than viewing fresh individual traces. *(pp.11-12)*

## Limitations and relation to earlier work

Coalesced batches can attribute shared work to only one request. Batch workloads
may need another trace unit. A slow request may have been delayed by requests
ahead of it; tracing its own work alone will not find that cause. Queue-size or
overload annotations and overlapping requests help. Kernel evidence remains
useful but hard to associate with user-space traces. *(pp.12-13)*

Black-box inference needs more data and statistical inference of causality.
Explicit application instrumentation offers portability but needs cooperation;
common library instrumentation trades on deployment homogeneity. Magpie's
event schema avoids global IDs but shifts propagation work to application
schemas. The paper reports production experience rather than a formal proof
or a general guarantee of negligible overhead. *(pp.3,13-14)*

## Barn application and testable properties

- Carry one input identity through queued work and committed output, including
  asynchronous continuations. Timestamp proximity alone is insufficient. *(pp.3-4)*
- Include competing-work context because a request's own trace cannot explain
  all queue delays. Mark snapshots as context, not causal proof. *(p.13)*
- Bound detailed logging and measure its cost; do not infer latency from
  throughput or compare differently instrumented runs. *(pp.6-7,11)*
- Preserve missing-output and incomplete traces explicitly; work can outlive
  the initiating callback. *(p.5)*

## Collection Cross-References

No cited papers or reverse citations found in the current collection's Markdown.
Topic searches did not find existing notes about request tracing or attribution.
The collection's concurrency papers address different mechanisms; no strong
or moderate connection established from their available notes.

### New Leads (Not Yet in Collection)

- Fonseca et al. (2007), X-Trace: A Pervasive Network Tracing Framework.
- Barham et al. (2004), Using Magpie for Request Extraction and Workload Modeling.
- Aguilera et al. (2003), Performance Debugging for Distributed Systems of Black Boxes.
- Reynolds et al. (2006), Detecting the Unexpected in Distributed Systems.

## Open questions

How much overhead is acceptable for Barn's quieter interactive workload, and
which concurrent-work snapshots explain its real tail? This paper supplies no
Barn-specific threshold or scheduling policy.
