package builtins

import (
	"bufio"
	"io"
	"strings"
	"sync"

	"barn/task"
	"barn/types"
)

// ProcessStdin owns suspended read_stdin() waiters and reads from the server's
// process stdin only while at least one task is waiting.
type ProcessStdin struct {
	reader *bufio.Reader

	mu      sync.Mutex
	waiters []*task.Task
	reading bool
}

func NewProcessStdin(input io.Reader) *ProcessStdin {
	return &ProcessStdin{reader: bufio.NewReader(input)}
}

func (p *ProcessStdin) ReadLineAsync(t *task.Task) bool {
	if p == nil || p.reader == nil || t == nil {
		return false
	}

	p.mu.Lock()
	p.waiters = append(p.waiters, t)
	if !p.reading {
		p.reading = true
		go p.readOne()
	}
	p.mu.Unlock()
	return true
}

func (p *ProcessStdin) readOne() {
	line, err := p.reader.ReadString('\n')
	value := readStdinValue(line, err)

	var waiter *task.Task
	p.mu.Lock()
	if len(p.waiters) > 0 {
		last := len(p.waiters) - 1
		waiter = p.waiters[last]
		p.waiters = p.waiters[:last]
	}
	if len(p.waiters) > 0 {
		go p.readOne()
	} else {
		p.reading = false
	}
	p.mu.Unlock()

	if waiter != nil {
		waiter.Resume(value)
	}
}

func readStdinValue(line string, err error) types.Value {
	if err != nil && line == "" {
		return types.NewErr(types.E_INVARG)
	}
	line = strings.ReplaceAll(line, "\n", "X")
	if strings.HasPrefix(line, "a") {
		return types.NewErr(types.E_NACC)
	}
	return types.NewStr(line)
}
