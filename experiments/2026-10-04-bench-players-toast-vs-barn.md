# Toast and Barn on the real Mongoose command mix over TCP (issue #265)

One run per engine. Treat differences under about 10% as noise until repeated.

## Setup

- Driver: `scripts/bench_players.py`, closed loop per connection, 2 s warm-up, 8 s measure.
- Mix: look 35 / say 30 / i 10 / @who 10 / home 15.
- Fixture: `mongoose.db.new`, 106,067,660 bytes, SHA-256
  `489ff8d14884392dcfba6cd88d407e53a2140c03d4f0cc4b1b19cf2afd8a031e`. Each engine ran on
  its own disposable copy; the control account was created offline in the copy.
- Toast: `/root/src/toaststunt-mongoose/build-release/moo` (PROMOTE_NUMBERS build,
  reports `2.7.3_5`) in WSL Debian, reached at the WSL address.
- Barn: `bin/barn.exe` built from `a593f27`, Windows/amd64, 32 logical CPUs,
  `profiles/barn/mongoose-outbound-on.conf`, on 127.0.0.1.
- The engines therefore ran on different operating systems on the same machine.
- The run directory's `files/sqlite/` starts empty on both engines. The 256 MB
  `sound.sqlite` is not present, so `say` does not include the full-table scan it
  performs against the deployed sound database.

## Commands

```
python scripts/bench_players.py --engine toast --players 1,16
python scripts/bench_players.py --engine barn --players 1,16 --barn-exe bin/barn.exe
```

## Results

| Engine | Players | Completed/s | p50 | p99 | p99.9 | max | Failed |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Toast | 1 | 105.8 | 4.93 ms | 162.51 ms | 1,608.89 ms | 1,608.89 ms | 0 |
| Barn | 1 | 129.3 | 7.39 ms | 14.85 ms | 27.69 ms | 27.77 ms | 0 |
| Toast | 16 | 361.6 | 22.67 ms | 511.11 ms | 1,888.32 ms | 5,570.25 ms | 0 |
| Barn | 16 | 228.2 | 51.25 ms | 276.83 ms | 1,367.91 ms | 1,404.97 ms | 0 |

Average latency by command:

| Command | Toast 1p | Barn 1p | Toast 16p | Barn 16p |
| --- | ---: | ---: | ---: | ---: |
| look | 7.80 ms | 10.72 ms | 38.19 ms | 56.33 ms |
| say | 16.10 ms | 7.90 ms | 67.51 ms | 88.33 ms |
| inventory | 6.93 ms | 6.17 ms | 23.77 ms | 44.65 ms |
| @who | 4.07 ms | 5.07 ms | 36.61 ms | 62.50 ms |
| home | 7.26 ms | 3.36 ms | 21.36 ms | 48.84 ms |

Barn's 16-player level also reported `BROKEN: home: no terminal acknowledgement within
30.0s` for one command after the measurement window.

## Reading

- At one player Barn completes 22% more commands and its tail is far shorter (p99
  14.85 ms against 162.51 ms).
- At sixteen players Toast completes 58% more commands than Barn. Barn's per-command
  latency grows 5 to 15 times from one player to sixteen; Toast's grows 3 to 9 times on
  a single thread.
- Barn does not turn its additional cores into throughput on this mix.
