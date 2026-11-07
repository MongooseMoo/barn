# Native byte-string iteration boundary fixture

Exact binary checkpoint from the managed canonical Toast producer
compiler/testdata/native-string-iteration.yaml. The source creates the two bytes
195 and 169 through chr(), then suspends inside their string iteration. Native
restart completes with seen={1,1}, done=1. The saved loop variable contains the
first single byte 195, which is invalid UTF-8 by itself.

The fixture is stored as ASCII base64 to preserve that invalid byte exactly.
Decoded SHA256:
c3e24d9ccbe800f3fc84db3b5b655c827204f0e501bbbe826d38ab14d427659c

Raw retained producer XML/admission are .tmp/native-string-toast.xml and
.tmp/native-string-toast-admission.json. The managed native run reports two
tests, zero failures/errors and a duration of approximately 24.16 seconds.

Barn's existing string iteration policy normalizes UTF-8 code points. The
converter must explicitly reject this active native byte-iteration state when
the existing normalized columns are not byte-equivalent; it must not alter
ordinary Barn strings or silently skip the second native byte. ASCII and
single-byte columns which match native iteration remain accepted.
