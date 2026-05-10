package main

import (
	"bytes"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	iac  = 255
	sb   = 250
	se   = 240
	will = 251
	wont = 252
	do   = 253
	dont = 254
	gmcp = 201
)

type endpoint struct {
	name string
	host string
	port int
}

type result struct {
	name       string
	transcript string
	err        error
}

func main() {
	var host string
	var barnPort int
	var toastPort int
	var configPath string
	var account string
	var outDir string
	var pause time.Duration

	flag.StringVar(&host, "host", "127.0.0.1", "server host")
	flag.IntVar(&barnPort, "barn-port", 17880, "Barn port")
	flag.IntVar(&toastPort, "toast-port", 17881, "Toast port")
	flag.StringVar(&configPath, "config", filepath.Join("..", "mongoose", "bridge.conf"), "bridge.conf path")
	flag.StringVar(&account, "account", "mongoose-codex", "bridge.conf account section")
	flag.StringVar(&outDir, "out", filepath.Join(".tmp", "mongoose-oracle", "probe"), "transcript output directory")
	flag.DurationVar(&pause, "pause", 600*time.Millisecond, "pause after each command")
	flag.Parse()

	login, err := readConnectCommand(configPath, account)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read login: %v\n", err)
		os.Exit(1)
	}

	commands := []command{
		{text: login, label: "<login redacted>"},
		{text: "look"},
		{text: "@who"},
		{text: "smile"},
		{text: "wave"},
		{text: "say Mongoose parity probe from local Barn/Toast copies."},
		{text: "north"},
		{text: "look"},
		{text: "south"},
		{text: "look"},
		{text: "east"},
		{text: "look"},
		{text: "west"},
		{text: "look"},
		{text: "@quit"},
	}

	endpoints := []endpoint{
		{name: "barn", host: host, port: barnPort},
		{name: "toast", host: host, port: toastPort},
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "create output dir: %v\n", err)
		os.Exit(1)
	}

	var failed bool
	for _, ep := range endpoints {
		res := runProbe(ep, commands, pause)
		path := filepath.Join(outDir, res.name+".txt")
		if writeErr := os.WriteFile(path, []byte(res.transcript), 0o600); writeErr != nil {
			fmt.Fprintf(os.Stderr, "%s write transcript: %v\n", res.name, writeErr)
			failed = true
			continue
		}
		if res.err != nil {
			fmt.Fprintf(os.Stderr, "%s probe error: %v\n", res.name, res.err)
			failed = true
		}
		fmt.Printf("%s transcript: %s\n", res.name, path)
	}

	if failed {
		os.Exit(1)
	}
}

type command struct {
	text  string
	label string
}

func readConnectCommand(path string, section string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	current := ""
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			current = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
			continue
		}
		if current != section {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) != "connect" {
			continue
		}
		connect := strings.TrimSpace(value)
		if connect == "" {
			return "", fmt.Errorf("empty connect command in [%s]", section)
		}
		return connect, nil
	}
	return "", fmt.Errorf("connect command not found in [%s]", section)
}

func runProbe(ep endpoint, commands []command, pause time.Duration) result {
	var transcript strings.Builder
	fmt.Fprintf(&transcript, "# %s %s:%d\n\n", ep.name, ep.host, ep.port)

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", ep.host, ep.port), 5*time.Second)
	if err != nil {
		return result{name: ep.name, transcript: transcript.String(), err: err}
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(45 * time.Second)); err != nil {
		return result{name: ep.name, transcript: transcript.String(), err: err}
	}

	reader := newTelnetReader(conn)
	drain := func(label string, wait time.Duration) {
		text := reader.readAvailable(wait)
		if strings.TrimSpace(text) == "" {
			return
		}
		fmt.Fprintf(&transcript, "<<< %s\n%s\n", label, scrub(text))
	}

	drain("banner", 1500*time.Millisecond)
	for _, cmd := range commands {
		label := cmd.text
		if cmd.label != "" {
			label = cmd.label
		}
		fmt.Fprintf(&transcript, ">>> %s\n", label)
		if _, err := conn.Write([]byte(cmd.text + "\r\n")); err != nil {
			return result{name: ep.name, transcript: transcript.String(), err: err}
		}
		drain(label, pause)
	}
	drain("final", 1200*time.Millisecond)

	return result{name: ep.name, transcript: transcript.String()}
}

type telnetReader struct {
	conn net.Conn
	buf  bytes.Buffer
}

func newTelnetReader(conn net.Conn) *telnetReader {
	return &telnetReader{conn: conn}
}

func (r *telnetReader) readAvailable(wait time.Duration) string {
	var out strings.Builder
	deadline := time.Now().Add(wait)
	tmp := make([]byte, 4096)
	for {
		_ = r.conn.SetReadDeadline(deadline)
		n, err := r.conn.Read(tmp)
		if n > 0 {
			out.WriteString(stripTelnet(tmp[:n]))
			deadline = time.Now().Add(80 * time.Millisecond)
			continue
		}
		if err != nil {
			break
		}
	}
	return out.String()
}

func stripTelnet(data []byte) string {
	var out []byte
	for i := 0; i < len(data); {
		if data[i] != iac {
			out = append(out, data[i])
			i++
			continue
		}
		if i+1 >= len(data) {
			break
		}
		cmd := data[i+1]
		switch cmd {
		case iac:
			out = append(out, iac)
			i += 2
		case sb:
			i += 2
			for i+1 < len(data) {
				if data[i] == iac && data[i+1] == se {
					i += 2
					break
				}
				i++
			}
		case will, wont, do, dont:
			i += 3
		default:
			i += 2
		}
	}
	return string(out)
}

var connectRedactor = regexp.MustCompile(`(?im)^connect\s+\S+\s+\S+.*$`)

func scrub(text string) string {
	return connectRedactor.ReplaceAllString(text, "connect <redacted>")
}
