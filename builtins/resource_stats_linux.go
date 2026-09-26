//go:build linux

package builtins

import (
	"fmt"
	"os"
	"syscall"
)

func readProcessUsage() (processUsage, error) {
	var load [3]float64
	f, err := os.Open("/proc/loadavg")
	if err != nil {
		return processUsage{}, err
	}
	_, scanErr := fmt.Fscan(f, &load[0], &load[1], &load[2])
	closeErr := f.Close()
	if scanErr != nil {
		return processUsage{}, scanErr
	}
	if closeErr != nil {
		return processUsage{}, closeErr
	}
	var r syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &r); err != nil {
		return processUsage{}, err
	}
	seconds := func(v syscall.Timeval) float64 { return float64(v.Sec) + float64(v.Usec)/1e6 }
	return processUsage{[]float64{load[0], load[1], load[2]}, seconds(r.Utime), seconds(r.Stime),
		r.Minflt, r.Majflt, r.Inblock, r.Oublock, r.Nvcsw, r.Nivcsw, r.Nsignals}, nil
}

func readProcessMemory() (processMemory, error) {
	f, err := os.Open("/proc/self/statm")
	if err != nil {
		return processMemory{}, err
	}
	defer f.Close()
	var m processMemory
	var ignored int64
	_, err = fmt.Fscan(f, &m.total, &m.resident, &m.shared, &m.text, &ignored, &m.data)
	return m, err
}
