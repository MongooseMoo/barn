package leakcheck

import (
	"os"
	"runtime"
	"strconv"
	"strings"
)

// Process describes the current PID only. Unavailable readings are explicit;
// shared cgroup and tmpfs use is collected by the Linux campaign driver instead.
type Process struct {
	Available bool     `json:"available"`
	RSSBytes  uint64   `json:"rss_bytes"`
	FDs       int      `json:"fds"`
	Threads   uint64   `json:"threads"`
	Errors    []string `json:"errors,omitempty"`
}

// ReadProcess captures Linux-owned process resources, without attributing runner
// memory to this workload. Other platforms report unavailable, never zero-use.
func ReadProcess() Process {
	var p Process
	if runtime.GOOS != "linux" {
		p.Errors = []string{"process resource capture requires Linux /proc"}
		return p
	}
	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		p.Errors = append(p.Errors, err.Error())
	} else {
		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 2 || (fields[0] != "VmRSS:" && fields[0] != "Threads:") {
				continue
			}
			v, err := strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				p.Errors = append(p.Errors, err.Error())
				continue
			}
			if fields[0] == "VmRSS:" {
				p.RSSBytes = v * 1024
			} else {
				p.Threads = v
			}
		}
	}
	fds, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		p.Errors = append(p.Errors, err.Error())
	} else {
		p.FDs = len(fds)
	}
	p.Available = len(p.Errors) == 0
	return p
}
