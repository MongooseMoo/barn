# MOO dump compatibility and parser reuse

Research for #515, 2026-10-25. This is a source inventory, not a successful-load or round-trip verdict. No historical corpus or server run was performed. Versions are format/language versions, not server release numbers. A core such as LambdaCore or JHCore is database content; its name does not identify a server dialect.

## Evidence pins

| Source | Inspected revision | Relevant source |
|---|---|---|
| Barn | `61543c82463f11bf5632f75cb55e9d4b77e2501d` | `db/format/reader.go:198–225`, version readers, object/task readers |
| ToastStunt local source | `eaaa1972f0993a1247f787dbf2dd5a01702ef442` | [version enum][toast-version], [database reader][toast-file], [value codec][toast-io], [activation reader][toast-execute] |
| LambdaMOO | `dc11b0fe6104019176c3f3ac1ef181bc664b7d2f` | [maintainer's variant/cross-load documentation][lambda-change] |
| lambdamoo-db-py | `d82a2a7f186d4a12b067216359bebd97d757709e` | [reader][py-reader], [writer][py-writer], [dependencies][py-project] |
| mooR | `da27013c879ce76f7a877b8d376e6f5e463de966` | [reader][moor-reader], [version model][moor-lib] |
| toaststunt-db-extractor | `184a12a736bd2386347e14a4495153cbb94f540f` | [native parser][extract-reader], [value parser][extract-value], [line reader][extract-lines] |

Local tracked ToastStunt and Python source had no modified tracked files in `git status --short`; unrelated untracked files were not used. Remote source was read through GitHub contents API. Links pin the inspected heads. Earliest producer release/commit for each historical transition has not been recovered; the ToastStunt enum is retrospective evidence, not evidence that all versions originated in ToastStunt.

## Historical matrix

The enum in `src/include/version.h:45–101` gives the following transitions. Layout descriptions additionally use `src/db_file.cc:233–386,837–1015`. All use the legacy line-oriented text family unless stated otherwise; exact encoding and number range require producer/build provenance. `L` means the old relationship links and object-first section order; `N` means typed relationships with object-first order; `A` means tasks/connections precede objects, with anonymous-object batches. These letters are shorthand in this report, not proposed code abstractions.

| Version | Recorded transition | Object layout | Language/task significance | Barn recognition at inspected revision |
|---|---|---|---|---|
| Prehistory (0) | Before format headers | L | Toast falls back when header scan fails; old empty temporary-register encoding in `db_io.cc:162–164` | No |
| 1 | Exceptions | L | `try/except/finally/endtry` keywords | No |
| 2 | Break/continue | L | New keywords | No |
| 3 | Float | L | FLOAT/INT/E_FLOAT; per-frame language version | No |
| 4 | Builtin override fix | L | Historical builtin continuation handling changes | Yes, dispatch only |
| 5 | Next-generation objects | N | Typed location/contents/parents/children replace legacy links | Yes, dispatch only |
| 6 | Task local | N | Additional VM task-local value | No |
| 7 | Map | N | MAP values | No |
| 8 | File I/O | N | E_FILE keyword | No |
| 9 | Exec | N | E_EXEC keyword | No |
| 10 | Interrupt | N | E_INTRPT; task reader includes interrupted section | No |
| 11 | Typed `this` | N | Activation reads typed `this` | No |
| 12 | Map iterator | N | Saved iterator value | No |
| 13 | Anonymous objects | A | Pending finalizations; typed activation verb location | No |
| 14 | Waifs | A | Waif values/references | No |
| 15 | Last move | A plus last_move | Extra object field | No |
| 16 | Threading | A plus last_move | Activation threaded field | No |
| 17 | Boolean | A plus last_move | Boolean value tag | Yes, dispatch only |

The four distinct compatibility questions are whether bytes are recognized, records decoded, values/graph preserved, and executable state restored. Export and verb compilation are separate questions again. Barn's `reader.go:207–214` recognizes 4/5/17 by substring and `writer.go:59–64` emits 17. That establishes neither full historical task decoding nor producer-language compilation. No row above has a complete executable-checkpoint verdict from this research.

Toast's actual compatibility branches explain why broad dispatch is insufficient:

- `db_file.cc:863–1015` changes section ordering at 13 and object decoding at 5; anonymous batches are terminated separately.
- `db_file.cc:333–340` gates last_move at 15.
- `execute.cc:3846–3853` gates typed `this` at 11, typed verb location at 13, and threading at 16. `execute.cc:4026` distinguishes pre-float activation programs.
- `tasks.cc:2116–2145` gates interrupted tasks at 10. Builtin continuation decoding is delegated from `execute.cc:4089`; a generic value reader is not sufficient to restore it.
- `db_io.cc:77–105,308–325` reads decimal integers with `strtoll`, floats with `strtod`, and writes double precision using `DBL_DIG + 4` significant digits. This describes the inspected Toast codec, not every historic producer's numeric width.

## Same version, different build

LambdaMOO's pinned `ChangeLog.txt:2996–3057` explicitly documents multiple variant producers of version 4, including Codepoint and optional waifs, without a differentiating header. The relevant provenance fields are integer width (`INT_TYPE_BITSIZE`), floating representation (`FLOATING_TYPE`, hardware mantissa/exponent), Unicode strings/identifiers, bitwise operators, waifs, and XML builtins. These affect range, precision, syntax, value types or runtime capability. A successful parse can still narrow floats or leave verbs dependent on unavailable builtins.

Consequently, a version-only compatibility table cannot promise every historical file. Preserve original bytes and record producer build information when known. Do not automatically transcode a version-4 string just because a library assumes Latin-1 or UTF-8. Unknown provenance must remain unknown.

## Corpus inventory

These are available files, not newly passed compatibility cases. SHA-256 values below were obtained directly from the checked-out files. The six small files match the local ToastStunt `test/tests` files byte-for-byte by SHA-256; this establishes current source correspondence, not their original generating executable/build.

| Barn `db/format/testdata/` file | Header | SHA-256 |
|---|---|---|
| Broken1.db | 5 | `717c20dda9c4946dc0caef003b95568cf9491f17286967a1eabde010a753ebaf` |
| Broken2.db | 5 | `5a893db5e0de7ebc3bcbbbcef7922244b81af88cf01513d74df60ede2cdeba76` |
| Broken3.db | 5 | `1ab32463f4626ae7dec1b00d374a6be50bada0b07e686f2325fb510f0f9b89f5` |
| Broken4.db | 5 | `d85b526073b1e72d49fb6f6bcc132b1479e7670d59802eb5e2213a7bae0afe7a` |
| Broken5.db | 5 | `aaf30e8373d39568982f7ddf9e2e15c9b358d8562f779c9f3f81b559b1649b44` |
| Anon6.db | 17 | `85d323ce37e9ae5cebb625f435118d138f4d549c68c81cdd30cd8eef0c73e3b5` |
| toastcore.db | 17 | `8013b703c61a9894866f836f2b934eada7118cdf0b3cd56181e4bf9205b2f557` |

`startup_repair_reader_test.go:45–153` uses the Broken fixtures as startup-repair examples. They must not be represented as a corpus of normal historical checkpoints. The generating revision/build of toastcore.db remains unknown here.

Additional discovered candidates, not validated or copied: Python checkout `LambdaCore-latest.db`, `toast2.db`, `toastcore.db`, `tests/fixtures/{Minimal,Suspended,TypedMapKeys}.db`; extractor `test/fixtures/{native-small,tiny,corrupt}.db`. Filename discovery is not provenance. Private Mongoose snapshots were excluded from publication. No fixture with established original producer/build was collected for prehistory, versions 1–3 or 6–16, Codepoint's build combinations, or the other families below.

## Reuse assessment

| Implementation | What inspected code establishes | Preservation/integration limitation | Recommendation |
|---|---|---|---|
| Python moodb | `reader.py:156–164` dispatches 1–4 and 17; `34–51` reads bytes as Latin-1; `728–867` models version-dependent activations/tasks; `writer.py:215–248` writes old/current layouts | No dispatch for 5–16; builtin payload reader at 809–818 handles known forms, not arbitrary future builtins. Offline parse/write is not execution. Python >=3.10 with attrs/cattrs/click/parse/pytest dependencies; project declares MIT | Use as a separate inspection/comparison tool and source of fixtures. No evidence a Python runtime bridge would simplify Barn's Go checkpoint path |
| mooR textdump | `lib.rs:92–127` parses LambdaMOO/Toast version headers and a separate Moor header containing semver, compiler features and encoding; `read.rs:92–96` chooses Latin-1 for legacy headers | `read.rs:432–442` skips unsupported WAIFs; queued/suspended/interrupted tasks and connections are skipped at 881–996. `lib.rs:137–138` explicitly has no Toast output. Source declares AGPLv3; Rust crates are integrated with mooR compiler/value types | Useful comparative importer, not a faithful checkpoint replacement or drop-in Go package. Direct code reuse needs license/integration assessment |
| Sindome extractor | Native path emits JSONL and verb files (`parseNativeDatabase.ts:37–159`), reads typed fields including lastMove at 383–390, summarizes WAIF data (`parseNativeValue.ts:158–175`) | `skipToObjectTable:330–342` scans to a numeric line followed by #0; it does not decode task state. Native path expects zero anonymous batch at 89. UTF-8 line reader (`LineReader.ts:7–10`); JavaScript number conversions deserve range scrutiny before using for lossless comparison. Repository declares MIT | Useful extraction/reporting ideas, not a database writer or checkpoint-preserving replacement |

No suitable Go replacement was established. This is a comparison of three named candidates, not an exhaustive proof that none exists. The smallest design remains improvements inside `db/format` using Go's existing buffered I/O and conversion functions; no new package or schema framework is justified by this research.

## Other families and unresolved provenance

- Modern mooR textdump is an explicitly different header/value/compiler-feature family; its native persistent storage is outside this textdump comparison. A `Moor ...` header is not another LambdaMOO format number.
- [LPMOO's author documents][lpmoo] persistence using DGD binary state dumps. This is outside the line-oriented family. No DGD codec or migration fixture was inspected here.
- [Ben Jackson's WAIF manual][waif-manual] is a discovered source candidate. The parent's publication-time fetch failed; its contents and matching on-disk tags and snapshots remain unverified here. Do not equate all historical waif representations with Toast's tag 13.
- [GammaMOO's repository][gamma] is a discovered fork candidate; its codecs have not been inspected. EtaMOO and other historical patches remain inventory gaps. This report makes no support assertion for them.

## Prioritized next work

1. Complete #516's preservation audit on today's 4/5/17 paths. Source questions worth targeted producer evidence: `reader_task.go:115–132` unconditionally reads typed `this`, verb location and threading even though upstream gates them by version; `reader_object.go:316–337` stores anonymous objects separately while `readVerbCode:378–382` attaches programs only through `Objects`. These are source discrepancies to investigate, not a claimed oracle failure from this report.
2. Keep #513/#514 limited to measured low-level writer/allocation improvements. They do not need a new dependency or expanded compatibility promise.
3. For #517, retain explicit orchestration for legacy object-first and anonymous task-first layouts; share only operations actually identical today. Version thresholds above are evidence against flattening all historic readers into one guessed schema. Do not enable 6–16 just because object fields look similar.
4. Recover a small public corpus with producer commits/build options and original hashes, starting with a real v4 saved task and v5 saved task, then Codepoint Unicode/large-number/waif variants and v13/v15/v16 transitions. Use the documented managed WSL oracle for uncertain MOO behavior; retain native results. Do not manufacture a version by changing only a header.
5. Decide supported guarantees from that evidence: object migration, loss-aware import, same-dialect export, and executable checkpoint restoration need distinct results. Only then scope additional dialect work.

No new guards, tests, runtime instrumentation, parser changes or compatibility flags are proposed or implemented here. The remaining historical corpus work is substantive; this report should not be cited as universal coverage.

[toast-version]: https://github.com/lisdude/toaststunt/blob/eaaa1972f0993a1247f787dbf2dd5a01702ef442/src/include/version.h#L45-L101
[toast-file]: https://github.com/lisdude/toaststunt/blob/eaaa1972f0993a1247f787dbf2dd5a01702ef442/src/db_file.cc#L837-L1015
[toast-io]: https://github.com/lisdude/toaststunt/blob/eaaa1972f0993a1247f787dbf2dd5a01702ef442/src/db_io.cc#L77-L105
[toast-execute]: https://github.com/lisdude/toaststunt/blob/eaaa1972f0993a1247f787dbf2dd5a01702ef442/src/execute.cc#L3838-L3890
[lambda-change]: https://github.com/wrog/lambdamoo/blob/dc11b0fe6104019176c3f3ac1ef181bc664b7d2f/ChangeLog.txt#L2996-L3057
[py-reader]: https://github.com/MongooseMoo/lambdamoo-db-py/blob/d82a2a7f186d4a12b067216359bebd97d757709e/lambdamoo_db/reader.py
[py-writer]: https://github.com/MongooseMoo/lambdamoo-db-py/blob/d82a2a7f186d4a12b067216359bebd97d757709e/lambdamoo_db/writer.py
[py-project]: https://github.com/MongooseMoo/lambdamoo-db-py/blob/d82a2a7f186d4a12b067216359bebd97d757709e/pyproject.toml
[moor-reader]: https://github.com/timbran-project/moor/blob/da27013c879ce76f7a877b8d376e6f5e463de966/crates/textdump/src/read.rs
[moor-lib]: https://github.com/timbran-project/moor/blob/da27013c879ce76f7a877b8d376e6f5e463de966/crates/textdump/src/lib.rs
[extract-reader]: https://github.com/SindomeCorp/toaststunt-db-extractor/blob/184a12a736bd2386347e14a4495153cbb94f540f/src/parser/parseNativeDatabase.ts
[extract-value]: https://github.com/SindomeCorp/toaststunt-db-extractor/blob/184a12a736bd2386347e14a4495153cbb94f540f/src/parser/parseNativeValue.ts
[extract-lines]: https://github.com/SindomeCorp/toaststunt-db-extractor/blob/184a12a736bd2386347e14a4495153cbb94f540f/src/reader/LineReader.ts
[lpmoo]: https://www.mars.org/home/rob/proj/lpmoo/differences.html
[waif-manual]: https://www.ben.com/moo/waif-progman.html
[gamma]: https://github.com/daitangio/gammamoo
