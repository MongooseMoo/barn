# bench_differ 2026-09-01 01:59:35

- db: `C:\Users\Q\code\moo-conformance-tests\src\moo_conformance\_db\Test.db` sha256 `1a3f23ebb549e02ccf5341668425118fcdc935b977096add87bc2a8ef29d408e`
- toast: `/root/src/toaststunt/build-release/moo` (WSL Debian)
- barn: linux/amd64 cross-build sha256 `a6b7fd611cb1a36b65742fe0adc4bb75b1f7dc9fcf4f72137dfcbaeaa2675292` from `C:\Users\Q\code\barn-prop` @ 2c9ce9c
- repeats: 5 (interleaved); timing = in-MOO ftime(1) bookends around eval()
- lane wall clock: toast 6.6s, barn 3.6s

| workload | toast ms (med/min) | barn ms (med/min) | barn/toast | n | values |
|---|---|---|---|---|---|
| list_index_1M | 27.16 / 26.92 | 53.98 / 52.44 | 1.99x | 5/5 | match |
| noop | 0.00 / 0.00 | 0.01 / 0.00 | 1.84x | 5/5 | match |
| prop_access_1M | 59.57 / 59.31 | 102.37 / 100.87 | 1.72x | 5/5 | match |
| builtin_abs_200k | 8.32 / 8.27 | 13.17 / 12.99 | 1.58x | 5/5 | match |
| float_arith_5M | 68.52 / 68.43 | 107.65 / 104.08 | 1.57x | 5/5 | match |
| int_arith_5M | 65.04 / 64.86 | 99.61 / 96.88 | 1.53x | 5/5 | match |
| nested_loop_2500x2500 | 81.35 / 81.12 | 121.42 / 118.81 | 1.49x | 5/5 | match |
| builtin_tostr_1M | 153.67 / 152.55 | 149.83 / 145.40 | 0.98x | 5/5 | match |
| string_concat_50k | 19.92 / 19.82 | 2.70 / 2.62 | 0.14x | 5/5 | match |
| list_append_30k | 778.21 / 777.42 | 2.34 / 2.25 | 0.00x | 5/5 | match |
