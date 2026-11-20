WAIF scout checkpoint, 2026-09-21.
Shared WAIF writes currently bypass StoreTxn and are undone by value comparison.
Store commit has decentralized and coarse publication paths; both need WAIF integration.
No implementation changes; scout is inspecting lifecycle, snapshot, and GC boundaries.
Next: finish exact design and return report to parent; database untouched.
