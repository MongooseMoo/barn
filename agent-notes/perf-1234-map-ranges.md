# Map marker and range traversal

Goal: close #1234 by avoiding full key and pair slices for map markers and ranges.

Decisions: keep VM validation and range construction order. Use existing ordered lookup tree paths for boundary markers and cache subtree sizes on that tree for rank lookup and bounded interval traversal. Preserve the prior `Value.Equal` lookup for escaped control characters, floats, and error keys, whose comparisons can differ from equality. Assignment appends the prefix, replacement, and suffix in one result buffer, including overlapping pieces for reversed bounds.

State: the focused types and VM tests passed. The warm 2,000-entry boundary benchmark measured 43.06 ns/op for both endpoints and zero allocations. The six-pair rank lookup and traversal measured 874.4 ns/op and one allocation. Managed and differential verification are recorded separately in the dispatch report.

Next step: publish for CI.
