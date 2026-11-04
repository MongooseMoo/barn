# Profile-Guided Builds

Go automatically applies the checked-in `cmd/barn/default.pgo` when building
the Barn command. The profile combines representative real-Mongoose workloads
at low and high concurrency with the `BenchmarkVM` interpreter loops and the
`BenchmarkVerbDispatch` verb and protected-builtin calls. Keep a profile-free
baseline available with `go build -pgo=off ./cmd/barn`.

## Refreshing the profile

Refresh the profile only after collecting current CPU profiles for all four
workload shapes, then merge them and re-run the performance gate:

```bash
go test -pgo=off ./vm -run='^$' -bench='^BenchmarkVM$' -count=3 -cpuprofile=benchvm.cpu
go test -pgo=off ./engine -run='^$' -bench='^BenchmarkVerbDispatch$' -benchtime=5s -count=2 -cpuprofile=verbdispatch.cpu
go tool pprof -proto -output=cmd/barn/default.pgo mongoose-1p.cpu mongoose-16p.cpu benchvm.cpu verbdispatch.cpu
go test -pgo=off ./vm -run='^$' -bench='^BenchmarkVM$' -benchmem -count=10 > before.txt
go test -pgo="$PWD/cmd/barn/default.pgo" ./vm -run='^$' -bench='^BenchmarkVM$' -benchmem -count=10 > after.txt
benchstat before.txt after.txt
```

`go test` applies `default.pgo` only to a main package, so the second run names
the profile; without `-pgo` it would also run without one.

The Mongoose profiles come from `BARN_MONGOOSE_CPUPROFILE` on the real-workload
harness at 1 and 16 players; a profile from any edited hot function goes stale
when its lines move, which can slow an unchanged path by a few percent.
`BenchmarkVM` makes no verb calls, so a profile without
`BenchmarkVerbDispatch` leaves the call path cold; check verb calls with
bench_differ as well as the `BenchmarkVM` gate above.

Commit a refreshed profile only when the geomean improves with p < 0.05 and no
benchmark regresses by more than 3%.

## Deployment architecture level

`make build-linux-amd64` produces the BMI2/AVX2 `GOAMD64=v3` binary used for
deployment, and bench_differ uses the same architecture level. Generic release
builds retain Go's `GOAMD64=v1` default.
