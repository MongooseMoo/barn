# Current campaign checkpoint 41

- PR 48 second commit `fc85ab4` is pushed and accepted by GitHub; files and commit boundaries are clean and exact.
- Managed run `31018225886` emitted quality plus all four intended Toast profiles.
- Quality passed. The new `64bit-example-outbound-on` job `92347843179` failed in the Toast build before admission or packaged execution.
- Downloaded failure artifact proves the one-line source-config assertion and provenance patch worked exactly: only `src/extensions.cc` changed from `#define EXAMPLE 0` to `1`.
- The pinned example extension itself does not compile under the pinned current C++ build: `panic` is undeclared at line 89 and two `void*` to `stdin_waiter*` conversions fail at lines 120 and 135.
- This is not an assertion failure and not a conformance case failure. It proves the proposed fourth profile is not a valid pinned Toast profile without further Toast source/compiler compatibility changes.
- Architectural decision under adversary review: do not silently patch more Toast source. Determine whether `read_stdin` identities should become explicit pinned-Toast absence contracts or whether an authorized and pinned Toast implementation change is required.
- Remaining three Toast profiles are still running; preserve their evidence to reveal any other corrected-case defects. Exact-union ledger cannot pass while the fourth profile fails.
- Next action: inspect complete managed results, receive adversary recommendation, then make the narrow oracle-faithful correction and rerun exact-head CI. PR remains draft and unmerged.
