---
title: "The Slab Allocator: An Object-Caching Kernel Memory Allocator"
authors: "Jeff Bonwick"
year: 1994
venue: "USENIX Summer 1994 Technical Conference"
doi_url: null
---

# The Slab Allocator: An Object-Caching Kernel Memory Allocator

## Reading provenance

Read every image, `pngs/page-000.png` through `page-011.png`, on 2026-11-07,
using the research-papers paper-process notes-only workflow and paper-reader.
Page citations below use PDF pages 1-12; the copy has no printed folios.
Source PDF: https://www.cs.dartmouth.edu/~sergey/cs258/2010/bonwick-slab-allocator-usenix.pdf
SHA256: `4d638486fd869a41ee6c8b16515dae8ecf51269e8109a0321601d843bb6c2825`.

## One-Sentence Summary

Retaining reusable constructed state can remove object initialization and
destruction costs beyond the savings available from faster memory allocation. *(p.1)*

## Problem and contributions

Kernel objects are allocated and released frequently, and construction can cost
more than allocating their memory. The allocator combines typed object caches,
slab allocation, pressure-driven reclamation, cache coloring and debugging.
Constructed state includes embedded locks and invariant empty-container state.
Clients must return objects in their constructed state; live references are not
reusable cached state. *(pp.1-3)*

## Algorithms and interface

1. On allocation, take an already constructed object if present; otherwise
   allocate memory and run the constructor. On free, return the object without
   destruction. On reclamation, destroy cached objects and release memory.
   Constructors and destructors populate and drain caches rather than running
   on every client allocation. *(pp.2-4)*
2. Clients describe name, size, alignment, constructor and destructor through
   `kmem_cache_create`. Allocation flags determine whether waiting is allowed;
   `kmem_cache_free` returns constructed objects. Cache destruction requires all
   allocated objects to have been returned. Size arguments support related
   object families. Allocation policy belongs centrally; clients own object
   descriptions. Private caches obscure pressure, accounting and diagnostics and
   duplicate maintenance. *(pp.2-3)*
3. A cache front end allocates/frees objects; a back end grows with VM pages and
   reaps under memory pressure. Each cache has its own lock and statistics.
   Generic allocation dispatches to caches sized from 8 bytes to 9 KiB in
   approximately 10-20% increments; larger requests use the page supplier.
   *(p.4)*
4. A slab consists of one or more virtually contiguous pages partitioned into
   equal objects. Slab metadata contains cache links, a reference count and a
   buffer free list. Buffer control records hold free links, addresses and slab
   pointers. Constant-time allocate/free updates a free list and reference
   count; zero-count whole slabs can be reclaimed. Typed lifetime grouping can
   reduce pages retained by isolated long-lived objects. *(p.5)*
5. Small-object slabs put slab metadata at the page end; each unused object holds
   its own free-list link at the end, with an extra word protecting constructed
   state. Large objects use separately allocated slab/control structures and a
   per-cache self-scaling hash for address-to-control lookup. *(p.6)*
6. A circular slab list groups fully allocated slabs, partial slabs, and wholly
   free slabs. The first-not-empty pointer locates allocatable slabs. Reclaiming
   a full free slab removes its whole free list without per-object unlinking.
   Free slabs go to the tail; a 15-second working set limits reclamation
   thrashing. The paper reports low sensitivity to that retention interval.
   *(pp.6-7)*
7. Cache coloring shifts each new slab's first object by an alignment increment
   through available slack. This distributes frequently used leading fields
   across cache lines and memory buses without enlarging slabs. *(p.7)*

## Equations and parameters

$$
\text{internal fragmentation per buffer} \leq 1/n
$$

Here n is the number of objects per slab; larger slabs reduce internal waste
but can make full-slab reclamation harder. *(p.5)*

| Name | Symbol | Units | Default | Range | Page | Notes |
| --- | --- | --- | --- | --- | --- | --- |
| Small-object cutoff | - | page fraction | 1/8 | - | 6 | Embedded slab layout below threshold |
| Internal waste limit | - | percent | 12.5 | - | 5 | Empirical internal/external tradeoff |
| Free working set | - | seconds | 15 | - | 6 | Retains recent free slabs |
| Example page | - | bytes | 4096 | - | 5,7 | Worked examples |
| Example buffer | - | bytes | 400 | - | 5 | Ten buffers leave 96 bytes |
| Per-buffer example waste | - | percent | 2.4 | - | 5 | 9.6 bytes per buffer |
| Coloring example buffer | - | bytes | 200 | - | 7 | Twenty buffers per page |
| Coloring alignment | - | bytes | 8 | - | 7 | Offsets increment by eight |
| Coloring metadata | - | bytes | 32 | - | 7 | Leaves 64 bytes for coloring |
| Color maximum | - | bytes | 64 | 0-64 | 7 | Offset sequence wraps |
| Synthetic workload processes | - | processes | 256 | - | 9 | Eight forks, socket, 60-second sleep |
| Synthetic comparison memory | - | MiB | 16 | - | 9 | SPARCstation-1 |
| Desktop comparison memory | - | MiB | 32 | - | 9 | One week, uncontrolled |

## Experimental design and results

The compared allocators were ported into the same SunOS 5.4 base. This controls
the OS implementation while changing allocation. Microbenchmarks used
SPARCstation-2; workload memory comparisons used a SPARCstation-1. The one-week
desktop comparison was explicitly uncontrolled. No confidence intervals or
significance tests are supplied. These are historical measurements. *(pp.8-9)*

| Allocator/interface | Allocate + free (microseconds) | Page |
| --- | ---: | --- |
| Slab/cache alloc | 3.8 | 8 |
| 4.4BSD/general alloc | 4.1 | 8 |
| Slab/general alloc | 4.7 | 8 |
| SVr4/general alloc | 9.4 | 8 |
| SunOS 4.1.3/general alloc | 25.0 | 8 |

Lock enter/exit cost 1.0 microsecond, suggesting a 2.0-microsecond lower bound
for a locked allocate/free pair. The 4.4BSD fastest-path comparison does not
include reclamation. Generic interfaces also pay size-class selection. *(p.8)*

| Cached operation | Without/with caching (microseconds) | Reported factor | Page |
| --- | --- | --- | --- |
| allocb | 8.3 / 6.0 | 1.4 | 8 |
| dupb | 13.4 / 8.7 | 1.5 | 8 |
| shalloc | 29.3 / 5.7 | 5.1 | 8 |
| allocq | 40.0 / 10.9 | 3.7 | 8 |
| anonmap_alloc | 16.3 / 10.1 | 1.6 | 8 |
| makepipe | 126.0 / 98.0 | 1.3 | 8 |

For stream heads, construction/destruction fell from 23.6 to 0 microseconds,
allocation from 9.4 to 3.8, while other initialization remained 1.9. Thus most
of the improvement in that example comes from avoiding construction. *(p.1)*

| Allocator | Waste: boot / spike / find / kenbus (%) | Kenbus scripts/min | Page |
| --- | --- | ---: | --- |
| Slab | 11 / 13 / 14 / 14 | 233 | 9 |
| SunOS 4.1.3 | 7 / 12 / 19 / 27 | 210 | 9 |
| 4.4BSD | 20 / 43 / 43 / 45 | 205 | 9 |
| SVr4 | 23 / 45 / 45 / 46 | 199 | 9 |

In the one-week desktop comparison, slab used 6.0 MiB at 9% fragmentation,
SunOS 4.1.3 6.7 MiB at 17%, SVr4 8.5 MiB at 35%, and 4.4BSD 9.0 MiB at 38%.
Workload-level gains reported: DeskBench 12%, kenbus 17%, TPC-B 4%, LADDIS 3%,
parallel make 5%, terminal server 5%. Paging mattered to the first two; the
TPC-B gain was attributed to cache behavior rather than allocator speed. Parallel
make reduced primary misses by 5% and secondary misses by 4%. Terminal-server
kernel time fell from 25% to 20%, yielding its smaller overall gain. *(pp.9-10)*

The SPARCcenter bus-coloring experiment reduced observed bus imbalance from 43%
to 17% and primary-cache miss rate by 13%. A 300-byte inode aligned into 512-byte
blocks can concentrate its first 48 bytes in only about 9% of the available cache
positions, motivating alignment-aware placement. *(pp.6-7)*

## Debugging and future work

Audit mode records thread, timestamp and stack in per-buffer circular logs.
Hash validation detects invalid frees; setting the large-object threshold to
zero makes this coverage universal. Freed-memory checking uses `0xdeadbeef`,
uninitialized memory `0xbaddcafe`; guards detect writes past buffer ends.
Synchronous unmapping reclaims free slabs immediately; page-per-buffer mode
unmaps every released object at substantial memory cost. Audit timestamps help
identify suspected leaks. The worked inode example includes modification offset,
old/new word values and prior transaction stacks. *(pp.10-11)*

The page supplier can be replaced to manage other memory types. Per-processor
front-end lists are future work, not a measured result of this paper. User-level
back ends could use mmap/sbrk. *(p.11)*

## Figures and design rationale

The front/back-end diagram separates client operations from pressure-driven
growth/reaping. The slab-control diagram shows slab links and buffer records;
the small-slab diagram places metadata at the page end. Segregated free lists
have small instruction footprints compared with sequential-fit searches and
buddy coalescing. Whole-slab reclaim trades fragmentation against reclaimability;
coloring spends existing slack to improve locality. *(pp.4-7)*

## Limitations and testable properties

- A returned cache object must satisfy its constructed-state invariant. Reusing
  an object with live users breaks the interface contract. *(pp.2-3)*
- Reclaim only wholly free slabs; a remaining live object pins its slab. *(pp.5-6)*
- Typed caches need coordinated reclamation and accounting, not unbounded private
  lists. *(pp.2,4)*
- The generic size caches do not receive all typed lifetime-segregation benefits.
  Larger slabs can worsen external fragmentation. *(p.5)*
- Isolated allocator improvements do not establish application throughput gains;
  the paper measures application workloads separately. *(pp.8-10)*
- Debug modes add cost; the desktop experiment is uncontrolled and all machines
  and kernel implementations are historically specific. *(pp.9-10)*

## Relevance to Barn

Medium. Measure inherited-slot construction, transient copying and cleanup
separately from allocating Go objects. Reuse Barn's existing immutable shared
property bases where evidence justifies it. This is an application hypothesis,
not the paper claiming anything about Barn. Stable object identities, MVCC
history and anonymous roots make whole-object pooling a separate correctness
problem; this paper does not justify introducing such a pool.

## Open questions

- Which construction and destruction costs dominate each Barn object type?
- Does avoiding transient property construction improve complete lifecycle work,
  including retained heap and anonymous collection?

## Collection Cross-References

No reference-list author/title is present in the current collection index, whose
only preexisting entry is Dapper. Reverse author/title and allocator-topic
searches found no connection. No superseding or substantive conceptual link was
established. New leads: Barrett and Zorn (1993), lifetime prediction; Grunwald,
Zorn and Henderson (1993), allocator locality; McKenney and Slingwine (1993),
multiprocessor allocation. These leads have not been read.
