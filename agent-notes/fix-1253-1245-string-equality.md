# String equality and list membership

Goal: Make unique() agree with Value.Equal on binary control escapes (#1253) and reduce equality and membership work (#1245).

State: The types package owns the equality key for unique() and its binary control normalization. Value.Equal compares identical bytes and simple ASCII folds directly, then uses the existing normalizer and Unicode comparison when the first differing bytes could match after normalization. List membership visits the backing slice directly and checks numeric coercion only for operands that can use it.

Decisions: Preserve the full input for the Unicode fallback because a single invalid byte anywhere in the string disables Unicode folding. Keep the case-sensitive is_member() default and its explicit case-insensitive flag. Keep the equality check in the unique() bucket and retain the first representative.

Verification: Focused tests cover a comparison matrix against the previous algorithm, unique() on the reported escape pairs, and first-match membership under both numeric modes. The Windows Go benchmark dropped from 37–67 ns/op to 8–17 ns/op at zero allocations. The documented WSL differential benchmark matched return values for all three issue workloads. The initial optimized run measured 2.01x Toast for string in, 0.66x for default is_member(), and 0.32x for integer in. The 10 ns/op and 1.3x targets still have misses. Repository checks run on the PR.
