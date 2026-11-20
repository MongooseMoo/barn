# Hash-consing Garbage Collection

Appel and Goncalves integrate immutable-record canonicalization into SML/NJ's
generational collector, paying lookup costs only for survivors. The measured
collector overhead frequently exceeds duplication savings. Relevant to Barn's
runtime string work as evidence for measuring lookup cost, lifetime and net
application behavior separately; it supplies no Go cache capacity or speedup.
