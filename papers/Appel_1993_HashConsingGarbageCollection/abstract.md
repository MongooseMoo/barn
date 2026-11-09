# Abstract

## Original Text (Short Excerpt)

"The cost of a hash table lookup for each record allocated"

The full abstract is available on the canonical PDF's first page. This file
uses a short quotation and a paraphrase rather than reproducing it in full.

## Our Interpretation

The report evaluates canonicalizing immutable records in SML/NJ. Its collector
does that work for surviving records, avoiding lookups for short-lived garbage.
Potential benefits include reduced duplicate storage, cheaper equality and
locality, but lookup and collector costs must be measured against those gains.
