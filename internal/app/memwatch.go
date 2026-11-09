package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	rtmetrics "runtime/metrics"
	"runtime/pprof"
	"sync/atomic"
	"time"

	"github.com/MongooseMoo/barn/engine"
	"github.com/MongooseMoo/barn/internal/mooprof"
)

const (
	memWatchInterval   = 5 * time.Second
	memWatchProfileFor = 5 * time.Second
	memWatchTaskCap    = 20
)

// heapTrip reports whether live heap has reached threshold and the next
// threshold: the first doubling above live, so one event per doubling.
func heapTrip(live, threshold uint64) (bool, uint64) {
	if live < threshold {
		return false, threshold
	}
	for threshold <= live {
		threshold *= 2
	}
	return true, threshold
}

// liveHeapBytes reads the heap marked live by the last GC; unlike heap-object
// bytes it does not count garbage awaiting the next cycle (GOGC is 400).
func liveHeapBytes() uint64 {
	sample := []rtmetrics.Sample{{Name: "/gc/heap/live:bytes"}}
	rtmetrics.Read(sample)
	return sample[0].Value.Uint64()
}

// startMemWatch polls live heap every memWatchInterval against the reading at
// start (taken after LoadDatabase's GC). At 2x, 4x, 8x... it logs "heap growth"
// with the unfinished tasks and, when logDir is set, writes heap, goroutine and
// MOO profiles there. It stops when ctx ends.
func startMemWatch(ctx context.Context, rt *engine.Runtime, logDir string) {
	baseline := liveHeapBytes()
	var profiling atomic.Bool
	go func() {
		ticker := time.NewTicker(memWatchInterval)
		defer ticker.Stop()
		threshold := 2 * baseline
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			live := liveHeapBytes()
			if threshold == 0 { // no GC had run at start; the first reading is the baseline
				baseline, threshold = live, 2*live
				continue
			}
			trip, next := heapTrip(live, threshold)
			threshold = next
			if !trip {
				continue
			}
			tasks := rt.UnfinishedTaskSummaries()
			total := len(tasks)
			if len(tasks) > memWatchTaskCap {
				tasks = tasks[:memWatchTaskCap]
			}
			slog.Warn("heap growth",
				slog.Uint64("live_bytes", live),
				slog.Uint64("baseline_bytes", baseline),
				slog.Int64("tasks_live", rt.LiveTaskCount()),
				slog.Int("unfinished_tasks", total),
				slog.Any("tasks", tasks))
			if logDir != "" && profiling.CompareAndSwap(false, true) {
				go func() {
					defer profiling.Store(false)
					writeHeapGrowthProfiles(ctx, logDir)
				}()
			}
		}
	}()
}

// writeHeapGrowthProfiles writes heap and goroutine profiles after a GC, then a
// memWatchProfileFor MOO profile, and logs the written paths.
func writeHeapGrowthProfiles(ctx context.Context, logDir string) {
	stamp := time.Now().Unix()
	path := func(kind string) string { return filepath.Join(logDir, fmt.Sprintf("%s-%d.pprof", kind, stamp)) }
	var written []string
	write := func(file string, writeTo func(*os.File) error) {
		f, err := os.Create(file)
		if err == nil {
			err = writeTo(f)
			if closeErr := f.Close(); err == nil {
				err = closeErr
			}
		}
		if err != nil {
			slog.Warn("write heap growth profile", slog.String("path", file), slog.String("err", err.Error()))
			return
		}
		written = append(written, file)
	}

	runtime.GC()
	write(path("heap"), func(f *os.File) error { return pprof.Lookup("heap").WriteTo(f, 0) })
	write(path("goroutine"), func(f *os.File) error { return pprof.Lookup("goroutine").WriteTo(f, 0) })

	mooNote := ""
	if session, err := mooprof.Start(); err != nil {
		mooNote = "skipped: " + err.Error()
	} else {
		timer := time.NewTimer(memWatchProfileFor)
		select {
		case <-timer.C:
			prof := session.Stop()
			write(path("moo"), func(f *os.File) error { return prof.Write(f) })
		case <-ctx.Done():
			timer.Stop()
			session.Stop()
			mooNote = "skipped: server stopping"
		}
	}
	slog.Warn("heap growth profiles", slog.Any("paths", written), slog.String("moo_profile", mooNote))
}
