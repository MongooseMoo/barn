---
title: "Hash-consing Garbage Collection"
authors: ["Andrew W. Appel", "Marcelo J. R. Goncalves"]
year: 1993
source: "https://www.cs.princeton.edu/~appel/papers/hashgc.pdf"
tags: [hash-consing, garbage-collection, interning]
read_date: 2026-11-09
provenance: "paper-reader; all 18 rendered page images read directly"
---

## One-Sentence Summary

Canonicalizing only immutable records that survive collection avoids charging every allocation for hash-consing, but the collector and hash table can cost more than the duplicate storage they remove.

## Problem Addressed

Heap-based functional programs can represent equal values with separate records. Duplication consumes space, makes equality structural rather than a pointer comparison, and can harm locality. Canonicalizing every allocation adds lookup cost even for values that immediately die. The paper implements a survivor-based alternative in SML/NJ rather than claiming canonicalization is always beneficial (pp. 1–4).

## Contributions and Method

- Only non-updatable records enter the canonical table. Mutable cells, arrays and byte arrays retain identity and ordinary copying semantics (pp. 2–4).
- The generational collector canonicalizes records while promoting them into the older generation. Young records are not guaranteed unique. Equality can use pointer identity only after recursion reaches canonical old records; it cannot universally replace structural equality (pp. 3–4).
- Bottom-up traversal ensures children have their canonical addresses before a parent's contents are hashed. The implementation replaces breadth-first copying with depth-first copying, then adds table lookup at the copy step (pp. 4–7, 9–10).
- The old-generation table is rebuilt after major collections because addresses and liveness may change. Resizing follows the measured live-data size (p. 10).

## Runtime Model

SML/NJ uses newer, older and reserve regions. A minor collection copies live young records into reserve space adjacent to the older generation; copied reserve becomes old space, and the remaining heap is split into young and reserve regions. Major collections also collect the older generation. Records carry a tag identifying kind and size, and low-bit tagging distinguishes pointers from immediate values (pp. 3–4).

Forwarding distinguishes pointers outside from-space, pointers to not-yet-copied records, and pointers to already-forwarded records. The new address is left in the old record. The canonicalizing variant looks for an equal old immutable record before copying; on a hit it forwards directly to that record, and on a miss it copies and inserts (pp. 3–4, 10).

## Depth-First Algorithm and Space Bound

Mark an unvisited record, visit its fields, and copy the record only after its children have been forwarded. The non-recursive form tracks the current record and current field and pushes a record/field pair when descending. After copying a child, pop its parent and replace the parent's child pointer with the forwarded address (Figures 1 and 3, pp. 5–7).

The auxiliary stack is placed at the high end of to-space and grows downward; copied records grow upward. Each stack entry needs two words, each record occupies at least two words including its descriptor, and a live record is either on the stack or already copied, never both. Thus space sufficient to copy all live records also suffices for the combined stack and copies. Failure to fit during a major collection means live data already exceeds destination capacity; it is not permission to overwrite either region (pp. 5–6).

## Cycles and Mutation

Minor collections use the compiler-maintained updated-reference list. Mark updated reference cells and stop depth-first descent when one is reached. Copy that cell without forwarding its fields, because mutable cells are not canonicalized. After root processing, forward the fields of moved reference cells; repeat until no newly moved reference cell appears. Repeated passes can be quadratic in the number of distinct reference cells. The update list may itself contain duplicate entries; the implementation removes entries after processing to reduce redundant work (pp. 6, 8–9).

The update list is unavailable for major collections, which do not hash-cons old data. A cycle-closing record can be copied with unresolved from-space pointers, then repaired in a later scan. A list of records needing repair can avoid a full scan, but consumes memory; a partial list plus a scan after its last address combines the approaches. Tags can mark records requiring repair. These strategies remain potentially expensive when cycles are common (p. 9).

## Hash Table

Open addressing uses a second hash to resolve collisions. Hashes derive from the first few record words. Minor collection only inserts, without deletion or movement of old entries. Major collection discards and rebuilds the table by sequentially scanning old immutable data, which permits capacity adjustment. The modified collector is approximately 930 C lines versus 530 for the original; this is a reported implementation size, not an estimate for Barn (p. 10).

## Experimental Parameters

| Parameter | Reported value | Context |
| --- | --- | --- |
| Machine | DECstation 5000/240, 64 MB RAM | Eight functional benchmarks, p. 11 |
| Compiler | SML/NJ 0.75 | Baseline and canonicalizing variants, p. 11 |
| Default heap ratio | 5 | Total heap relative to live data, pp. 11–13 |
| Varied heap ratios | 3, 5, 7, 10, 15, 20, 25, 100 | Lex experiment, Figure 6 |
| Table sizes | heap/16, heap/12, heap/8, heap/4 | Lex experiment, Figure 8 |

The programs are Boyer, Knuth-Bendix, Lex, Life, Mandelbrot, Simple fluid dynamics, VLIW scheduling and Yacc. Timings separate GC time from non-GC time and normalize each program to its non-canonicalizing control. The paper reports no confidence intervals or replication count (pp. 11–15).

## Results

GC cost rises as much as fourfold; for most programs the increase is at most about twofold. Non-GC time often improves slightly, but most gains do not repay collection cost. Yacc is 22% slower overall; Boyer improves by almost 10%. Larger heap ratios can amortize the increased collection cost. The tested table-size range has little effect on Lex runtime (pp. 11–14).

Figure 7 reports cumulative data copied into old generation and duplicate data found, in MB:

| Program | Without | With | Duplicates found |
| --- | ---: | ---: | ---: |
| Boyer | 153.0 | 130.2 | 3.340 |
| Life | 172.1 | 171.8 | 0.066 |
| Knuth-B | 181.6 | 178.5 | 0.354 |
| Simple | 406.3 | 404.1 | 0.452 |
| Mandel | 1482.8 | 1482.7 | 0.000 |
| Lex | 112.4 | 112.9 | 0.007 |
| VLIW | 235.2 | 234.9 | 0.008 |
| Yacc | 52.1 | 51.0 | 0.256 |

These are copying/allocation measurements, not peak RSS or retained heap. Most measured reductions are under one percent. The breadth-first versus depth-first comparison without canonicalization shows no significant difference on this machine; the authors suggest small cache blocks and little paging may limit observable locality differences (pp. 13–16).

## Future Work and Limits

The paper proposes memoizing pure function applications using canonical identities. A global side-effect counter could reject memoization when an evaluation reads or writes mutable state. This is future work, not an implemented general-purpose correctness guarantee (p. 16).

The conclusion reports roughly two- to threefold collector overhead and limited savings in most examples. Its affordability argument assumes GC is a small part of total execution. Results are specific to the old SML runtime, generational collector and machine; they neither establish Go performance nor select a bounded-cache eviction policy (pp. 16–17).

## Implications for Barn

Inference: separate allocation churn, retained graph size, lookup/locking overhead and end-to-end throughput. Target a verified repetitive producer rather than charge all NewStr calls. Cache only byte-immutable representations; keep mutable append backing outside the canonical table. A bounded owning cache is a different design from this collector-integrated algorithm and needs its own tests and paired measurements. Paper results do not justify a GC modification in Barn.

## Testable Properties and Open Questions

- Canonicalization preserves exact content and distinguishes mutable identity.
- Hash collisions never change values; misses may allocate normally.
- A cache that retains an input substring can pin more than its advertised text length; Barn must prove ownership separately.
- Does the actual canonical-name hot working set fit the proposed budget? Measure it and the collision/miss cost.
- Does saved allocation/GC work exceed lookup and synchronization overhead? Test with unchanged fixture, deadlines and command mix.
