# bench_differ 2026-09-01 02:00:17

- db: `C:\Users\Q\code\moo-conformance-tests\src\moo_conformance\_db\Test.db` sha256 `1a3f23ebb549e02ccf5341668425118fcdc935b977096add87bc2a8ef29d408e`
- toast: `/root/src/toaststunt/build-release/moo` (WSL Debian)
- barn: linux/amd64 cross-build sha256 `5e3d824d05e11f8c65c06bacdacbfff9f19ddcca31b9b553a1cedcb7d304e946` from `C:\Users\Q\code\barn-base` @ 5ebcc86
- repeats: 5 (interleaved); timing = in-MOO ftime(1) bookends around eval()
- lane wall clock: toast 6.6s, barn 3.7s

| workload | toast ms (med/min) | barn ms (med/min) | barn/toast | n | values |
|---|---|---|---|---|---|
| noop | 0.00 / 0.00 | 0.01 / 0.00 | 2.18x | 5/5 | match |
| prop_access_1M | 60.55 / 59.90 | 118.40 / 116.02 | 1.96x | 5/5 | match |
| list_index_1M | 28.01 / 27.22 | 53.33 / 53.11 | 1.90x | 5/5 | match |
| builtin_abs_200k | 8.31 / 8.29 | 14.83 / 14.31 | 1.78x | 5/5 | match |
| float_arith_5M | 69.58 / 68.47 | 106.33 / 102.85 | 1.53x | 5/5 | match |
| int_arith_5M | 66.15 / 65.11 | 97.09 / 95.18 | 1.47x | 5/5 | match |
| nested_loop_2500x2500 | 83.30 / 81.47 | 117.71 / 116.88 | 1.41x | 5/5 | match |
| builtin_tostr_1M | 156.24 / 152.62 | 166.56 / 163.78 | 1.07x | 5/5 | match |
| string_concat_50k | 20.25 / 19.79 | 2.67 / 2.61 | 0.13x | 5/5 | match |
| list_append_30k | 793.45 / 786.02 | 3.08 / 2.37 | 0.00x | 5/5 | match |
