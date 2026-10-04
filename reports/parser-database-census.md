# Database parser acceptance census

Issue #292 replaces the test that stopped after 50 successfully parsed nonempty
verbs. Run the complete tracked-fixture census with:

```sh
go test ./parser -run '^TestDatabaseProgramCensus$' -count=1 -v
```

The test emits `CENSUS_REPORT` followed by the complete JSON report and compares
it with [the checked inventory](../parser/testdata/toastcore-census.json).
Review a changed report before updating that artifact; ordinary tests never
rewrite it. JSON comparison tolerates platform checkout line endings.

The census sorts objects by ID and visits verbs in index order. It includes
numbered and anonymous objects, attempts every `HasProgram` verb, includes empty
programs, and counts never-programmed verbs separately. A parser rejection is
recorded and traversal continues. Each rejection has its object ID, verb index
and name, stage, `ParseError.Line` when available, and whitespace-normalized
`ParseError.Detail`. The current error API exposes no column or offset.
Families are sorted by stage/detail and contain counts plus at most three first
identities; the full failure array retains every record.

Database readers retain the input verb-program count as `DeclaredPrograms` for
formats 4, 5, and 17. The census asserts that attempts equal that declared count
and accepted plus rejected equals attempts. An unattached or otherwise lost
program cannot disappear silently from this reconciliation. The tracked fixture
has no such count exceptions.

Results for `db/format/testdata/toastcore.db` at the implementation base
`96a463f7d2eb5623e407b3139d4bbd4fb1877dde`:

| Measurement | Attempted | Successful | Failed |
| --- | ---: | ---: | ---: |
| Source parser acceptance | 1950 | 1950 | 0 |
| Canonical formatted-source reparsing | 1950 | 1950 | 0 |
| Position-free IR preservation | 1950 | 1950 | 0 |
| Formatter idempotence | 1950 | 1950 | 0 |

Four verbs are never programmed. The single empty program is
`#10`, verb index `17`, `special_action`; it contributes to all four measurements.
There are no rejection families in this fixture inventory. Synthetic tests
exercise continued traversal after rejection and after the 50th success, empty
versus absent programs, anonymous objects, ordering, complete failure storage,
bounded family examples, and diagnostic field propagation.

Source acceptance is measured before formatting. Accepted programs then undergo
canonical formatting and reparsing; only successful reparses enter the separate
IR comparison and second-format comparison. The IR comparison uses the existing
parser test normalization that removes source positions. It measures structural
preservation and does not execute the programs or validate builtin availability.
These results describe Barn's parser on this fixture; oracle acceptance and
broader MOO conformance remain separate verification surfaces.
