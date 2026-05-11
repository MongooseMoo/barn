package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	telnetIAC  = 255
	telnetSB   = 250
	telnetSE   = 240
	telnetWILL = 251
	telnetWONT = 252
	telnetDO   = 253
	telnetDONT = 254
)

type runManifest struct {
	RunID     string `json:"run_id"`
	BarnPort  int    `json:"barn_port"`
	ToastPort int    `json:"toast_port"`
}

type target struct {
	Name string
	Host string
	Port int
}

type event struct {
	TimestampUTC string `json:"timestamp_utc"`
	Server       string `json:"server"`
	Connection   string `json:"connection"`
	Source       string `json:"source"`
	Event        string `json:"event"`
	Address      string `json:"address,omitempty"`
	RawBase64    string `json:"raw_base64,omitempty"`
	Text         string `json:"text,omitempty"`
	Error        string `json:"error,omitempty"`
}

type captureResult struct {
	target target
	events []event
	err    error
}

func main() {
	var root string
	var runDir string
	var host string
	var readTimeout time.Duration
	var connectTimeout time.Duration
	var output string
	var configPath string
	var configSection string
	var sendBoth string

	flag.StringVar(&root, "root", ".tmp/mongoose-oracle", "managed oracle artifact root")
	flag.StringVar(&runDir, "run-dir", "", "managed run directory; defaults to root/current-run.txt")
	flag.StringVar(&host, "host", "127.0.0.1", "host used for both managed targets")
	flag.DurationVar(&connectTimeout, "connect-timeout", 3*time.Second, "TCP connect timeout")
	flag.DurationVar(&readTimeout, "read-timeout", 2*time.Second, "banner read deadline after connect")
	flag.StringVar(&output, "out", "", "JSONL transcript path; defaults to run/transcripts/bridge-banners.jsonl")
	flag.StringVar(&configPath, "config", "", "optional bridge.conf path used for a local connect command")
	flag.StringVar(&configSection, "config-section", "", "bridge.conf section containing connect command")
	flag.StringVar(&sendBoth, "send-both", "", "literal line to send to both targets after connect")
	flag.Parse()

	resolvedRunDir, err := resolveRunDir(root, runDir)
	if err != nil {
		fatal(err)
	}
	manifest, err := loadManifest(resolvedRunDir)
	if err != nil {
		fatal(err)
	}
	if manifest.BarnPort == 0 || manifest.ToastPort == 0 {
		fatal(fmt.Errorf("manifest %s has missing barn_port or toast_port", filepath.Join(resolvedRunDir, "manifest.json")))
	}

	if output == "" {
		output = filepath.Join(resolvedRunDir, "transcripts", "bridge-banners.jsonl")
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		fatal(err)
	}

	connectCommand, err := selectedConnectCommand(configPath, configSection)
	if err != nil {
		fatal(err)
	}
	if sendBoth != "" && connectCommand != "" {
		fatal(fmt.Errorf("use either -send-both or -config/-config-section, not both"))
	}
	lineToSend := sendBoth
	redactSentLine := false
	if connectCommand != "" {
		lineToSend = connectCommand
		redactSentLine = true
	}

	targets := []target{
		{Name: "toast", Host: host, Port: manifest.ToastPort},
		{Name: "barn", Host: host, Port: manifest.BarnPort},
	}
	results := captureTargets(targets, connectTimeout, readTimeout, lineToSend, redactSentLine)
	events := flattenEvents(results)
	if err := writeJSONLines(output, events); err != nil {
		fatal(err)
	}
	for _, evt := range events {
		line, err := json.Marshal(evt)
		if err != nil {
			fatal(err)
		}
		fmt.Println(string(line))
	}

	var failures []string
	for _, result := range results {
		if result.err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", result.target.Name, result.err))
		}
	}
	if len(failures) > 0 {
		fatal(fmt.Errorf("bridge banner capture failed: %s", strings.Join(failures, "; ")))
	}
}

func resolveRunDir(root, runDir string) (string, error) {
	if runDir != "" {
		return filepath.Abs(runDir)
	}
	currentPath := filepath.Join(root, "current-run.txt")
	content, err := os.ReadFile(currentPath)
	if err != nil {
		return "", fmt.Errorf("read current run file %s: %w", currentPath, err)
	}
	current := strings.TrimSpace(string(content))
	if current == "" {
		return "", fmt.Errorf("current run file %s is empty", currentPath)
	}
	return filepath.Abs(current)
}

func loadManifest(runDir string) (runManifest, error) {
	path := filepath.Join(runDir, "manifest.json")
	content, err := os.ReadFile(path)
	if err != nil {
		return runManifest{}, fmt.Errorf("read manifest %s: %w", path, err)
	}
	var manifest runManifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		return runManifest{}, fmt.Errorf("parse manifest %s: %w", path, err)
	}
	return manifest, nil
}

func captureTargets(targets []target, connectTimeout, readTimeout time.Duration, lineToSend string, redactSentLine bool) []captureResult {
	results := make([]captureResult, len(targets))
	var wg sync.WaitGroup
	for i, tgt := range targets {
		wg.Add(1)
		go func(i int, tgt target) {
			defer wg.Done()
			events, err := captureBanner(tgt, connectTimeout, readTimeout, lineToSend, redactSentLine)
			results[i] = captureResult{target: tgt, events: events, err: err}
		}(i, tgt)
	}
	wg.Wait()
	return results
}

func captureBanner(target target, connectTimeout, readTimeout time.Duration, lineToSend string, redactSentLine bool) ([]event, error) {
	connectionID := fmt.Sprintf("%s-%s", target.Name, time.Now().UTC().Format("20060102T150405.000000000Z"))
	address := fmt.Sprintf("%s:%d", target.Host, target.Port)
	events := []event{newEvent(target.Name, connectionID, "bridge", "connect_start", address, nil, "")}

	dialer := net.Dialer{Timeout: connectTimeout}
	conn, err := dialer.Dial("tcp", address)
	if err != nil {
		events = append(events, newErrorEvent(target.Name, connectionID, "bridge", "connect_error", address, err))
		return events, err
	}
	defer conn.Close()
	events = append(events, newEvent(target.Name, connectionID, "bridge", "connect_ok", address, nil, ""))

	if lineToSend != "" {
		if _, err := conn.Write([]byte(lineToSend + "\r\n")); err != nil {
			events = append(events, newErrorEvent(target.Name, connectionID, "socket", "send_error", address, err))
			return events, err
		}
		text := lineToSend
		if redactSentLine {
			text = "[redacted config connect command]"
		}
		events = append(events, newEvent(target.Name, connectionID, "socket", "send_line", address, nil, text))
	}

	if err := conn.SetReadDeadline(time.Now().Add(readTimeout)); err != nil {
		events = append(events, newErrorEvent(target.Name, connectionID, "bridge", "deadline_error", address, err))
		return events, err
	}

	buf := make([]byte, 4096)
	readAny := false
	for {
		n, err := conn.Read(buf)
		if n > 0 {
			readAny = true
			raw := append([]byte(nil), buf[:n]...)
			events = append(events, newEvent(target.Name, connectionID, "socket", "banner_chunk", address, raw, normalizeTelnetText(raw)))
		}
		if err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				events = append(events, newEvent(target.Name, connectionID, "bridge", "read_timeout", address, nil, ""))
				break
			}
			events = append(events, newErrorEvent(target.Name, connectionID, "socket", "read_error", address, err))
			return events, err
		}
	}
	if !readAny {
		events = append(events, newEvent(target.Name, connectionID, "bridge", "empty_banner", address, nil, ""))
	}
	return events, nil
}

func newEvent(server, connection, source, kind, address string, raw []byte, text string) event {
	evt := event{
		TimestampUTC: time.Now().UTC().Format(time.RFC3339Nano),
		Server:       server,
		Connection:   connection,
		Source:       source,
		Event:        kind,
		Address:      address,
		Text:         text,
	}
	if len(raw) > 0 {
		evt.RawBase64 = base64.StdEncoding.EncodeToString(raw)
	}
	return evt
}

func newErrorEvent(server, connection, source, kind, address string, err error) event {
	evt := newEvent(server, connection, source, kind, address, nil, "")
	evt.Error = err.Error()
	return evt
}

func flattenEvents(results []captureResult) []event {
	var events []event
	for _, result := range results {
		events = append(events, result.events...)
	}
	return events
}

func writeJSONLines(path string, events []event) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create transcript %s: %w", path, err)
	}
	defer file.Close()
	for _, evt := range events {
		line, err := json.Marshal(evt)
		if err != nil {
			return err
		}
		if _, err := file.Write(append(line, '\n')); err != nil {
			return fmt.Errorf("write transcript %s: %w", path, err)
		}
	}
	return nil
}

func normalizeTelnetText(raw []byte) string {
	text := make([]byte, 0, len(raw))
	for i := 0; i < len(raw); {
		if raw[i] != telnetIAC {
			text = append(text, raw[i])
			i++
			continue
		}
		if i+1 >= len(raw) {
			break
		}
		cmd := raw[i+1]
		switch cmd {
		case telnetIAC:
			text = append(text, telnetIAC)
			i += 2
		case telnetSB:
			i += 2
			for i < len(raw)-1 {
				if raw[i] == telnetIAC && raw[i+1] == telnetSE {
					i += 2
					break
				}
				i++
			}
		case telnetWILL, telnetWONT, telnetDO, telnetDONT:
			if i+2 >= len(raw) {
				i = len(raw)
			} else {
				i += 3
			}
		default:
			i += 2
		}
	}
	return strings.ToValidUTF8(string(text), string(utf8.RuneError))
}

func selectedConnectCommand(configPath, section string) (string, error) {
	if configPath == "" {
		return "", nil
	}
	if section == "" {
		return "", fmt.Errorf("-config-section is required with -config")
	}
	values, err := parseINISection(configPath, section)
	if err != nil {
		return "", err
	}
	connect := strings.TrimSpace(values["connect"])
	if connect == "" {
		return "", fmt.Errorf("section %q in %s has no connect value", section, configPath)
	}
	return connect, nil
}

func parseINISection(path, section string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open config %s: %w", path, err)
	}
	defer file.Close()

	values := map[string]string{}
	current := ""
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.Contains(line, "]") {
			current = strings.TrimSpace(line[1:strings.Index(line, "]")])
			continue
		}
		if current != section {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		values[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(value)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("section %q not found in %s", section, path)
	}
	return values, nil
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "mongoose_bridge: %v\n", err)
	os.Exit(1)
}
