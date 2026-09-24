package main

import (
	"fmt"
	"strings"
	"sync"
)

// promptBuffer retains partial prompts across network reads. Notifications are
// coalesced so an unsolicited output burst never blocks the socket reader.
type promptBuffer struct {
	mu      sync.Mutex
	text    string
	changed chan struct{}
}

func newPromptBuffer() *promptBuffer { return &promptBuffer{changed: make(chan struct{}, 1)} }

func (p *promptBuffer) feed(text string) {
	p.mu.Lock()
	p.text += text
	if len(p.text) > 65536 {
		p.text = p.text[len(p.text)-65536:]
	}
	p.mu.Unlock()
	select {
	case p.changed <- struct{}{}:
	default:
	}
}

func (p *promptBuffer) wait(done <-chan struct{}, patterns ...string) (string, error) {
	for {
		p.mu.Lock()
		for _, pattern := range patterns {
			if at := strings.Index(p.text, pattern); at >= 0 {
				p.text = p.text[at+len(pattern):]
				p.mu.Unlock()
				return pattern, nil
			}
		}
		p.mu.Unlock()
		select {
		case <-p.changed:
		case <-done:
			return "", fmt.Errorf("connection ended before expected login prompt")
		}
	}
}

// loginCommands consumes an optional PROXY prelude and two account commands.
// The password and username never appear in prompt events or errors.
func loginCommands(commands []string, ready string, prompts *promptBuffer, done <-chan struct{}, send func(int, string) error, phase func(string)) (int, error) {
	i := 0
	if len(commands) > 0 && strings.HasPrefix(commands[0], "PROXY ") {
		if err := send(0, commands[0]); err != nil {
			return 0, err
		}
		i++
	}
	if len(commands)-i < 2 {
		return i, fmt.Errorf("prompt login requires username and password commands")
	}
	for _, prompt := range []struct{ text, phase string }{{"Enter your username or email:", "username"}, {"Password", "password"}} {
		if _, err := prompts.wait(done, prompt.text); err != nil {
			return i, err
		}
		phase(prompt.phase)
		if err := send(i, commands[i]); err != nil {
			return i, err
		}
		i++
	}
	matched, err := prompts.wait(done, ready, "Confunc failed:", "Access Denied")
	if err != nil {
		return i, err
	}
	if matched == "Access Denied" {
		return i, fmt.Errorf("account login denied")
	}
	if matched == "Confunc failed:" {
		phase("login_hook_failed")
	} else {
		phase("login_ready")
	}
	return i, nil
}
