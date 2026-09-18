package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type arrayFlags []string

type clientEvent struct {
	Event        string `json:"event"`
	ElapsedMS    int64  `json:"elapsed_ms"`
	CommandIndex int    `json:"command_index,omitempty"`
	Phase        string `json:"phase,omitempty"`
	Bytes        int    `json:"bytes,omitempty"`
	Text         string `json:"text,omitempty"`
}

type eventLog struct {
	start   time.Time
	file    *os.File
	encoder *json.Encoder
	mu      sync.Mutex
}

func newEventLog(path string, start time.Time) (*eventLog, error) {
	log := &eventLog{start: start}
	if path == "" {
		return log, nil
	}
	file, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	log.file = file
	log.encoder = json.NewEncoder(file)
	return log, nil
}

func (l *eventLog) record(event clientEvent, at time.Time) {
	if l.encoder == nil {
		return
	}
	event.ElapsedMS = at.Sub(l.start).Milliseconds()
	l.mu.Lock()
	defer l.mu.Unlock()
	_ = l.encoder.Encode(event)
}

func (l *eventLog) close() error {
	if l.file == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.file.Close()
}

func startMaxDuration(conn net.Conn, duration time.Duration, events *eventLog) *time.Timer {
	return time.AfterFunc(duration, func() {
		events.record(clientEvent{Event: "max_duration"}, time.Now())
		_ = conn.Close()
	})
}

func (a *arrayFlags) String() string {
	return strings.Join(*a, ", ")
}

func (a *arrayFlags) Set(value string) error {
	*a = append(*a, value)
	return nil
}

func main() {
	var commands arrayFlags
	var port int
	var host string
	var file string
	var timeout int
	var bannerWait int
	var interCmd int
	var eventLogPath string
	var maxDuration int
	var promptLogin, stopAfterLogin bool
	var loginReady string

	flag.Var(&commands, "cmd", "Command to send (can be specified multiple times)")
	flag.IntVar(&port, "port", 7777, "MOO server port")
	flag.StringVar(&host, "host", "localhost", "MOO server host")
	flag.StringVar(&file, "file", "", "File containing commands (one per line)")
	flag.IntVar(&timeout, "timeout", 3, "Seconds of idle silence before exiting")
	flag.IntVar(&bannerWait, "banner-wait", 0, "Milliseconds to wait for the connect banner before sending the first command")
	flag.IntVar(&interCmd, "inter-cmd", 300, "Milliseconds to wait between commands")
	flag.StringVar(&eventLogPath, "event-log", "", "Write timestamped client events as JSONL without command text")
	flag.IntVar(&maxDuration, "max-duration", 0, "Maximum connection duration in seconds (0 waits for idle timeout)")
	flag.BoolVar(&promptLogin, "login-prompts", false, "Respond to account prompts instead of fixed login delays")
	flag.BoolVar(&stopAfterLogin, "stop-after-login", false, "Close after the welcome marker; failed hooks retain the receive timeout")
	flag.StringVar(&loginReady, "login-ready", "MESSAGE OF THE DAY:", "Marker indicating completion of the welcome hook")
	flag.Parse()

	started := time.Now()
	events, err := newEventLog(eventLogPath, started)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening event log: %v\n", err)
		os.Exit(1)
	}
	defer events.close()

	// Load commands from file if specified
	if file != "" {
		fileCommands, err := loadCommandsFromFile(file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading commands file: %v\n", err)
			os.Exit(1)
		}
		commands = append(commands, fileCommands...)
	}

	// Connect to MOO server
	address := net.JoinHostPort(host, strconv.Itoa(port))
	fmt.Fprintf(os.Stderr, "Connecting to %s...\n", address)
	events.record(clientEvent{Event: "connect_start"}, time.Now())

	conn, err := net.Dial("tcp", address)
	if err != nil {
		events.record(clientEvent{Event: "connect_error"}, time.Now())
		fmt.Fprintf(os.Stderr, "Connection failed: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()
	if maxDuration > 0 {
		timer := startMaxDuration(conn, time.Duration(maxDuration)*time.Second, events)
		defer timer.Stop()
	}

	fmt.Fprintf(os.Stderr, "Connected.\n")
	events.record(clientEvent{Event: "connected"}, time.Now())

	// Reader runs in the background, printing raw bytes as they arrive so that
	// partial lines (e.g. a banner with no trailing newline, or a bare prompt)
	// are never lost. It signals done when the connection idles out or closes.
	done := make(chan struct{})
	prompts := newPromptBuffer()
	go readOutput(conn, done, time.Duration(timeout)*time.Second, events, prompts)

	// Optionally let the connect banner arrive before we start typing.
	if bannerWait > 0 && !promptLogin {
		time.Sleep(time.Duration(bannerWait) * time.Millisecond)
	}

	writer := bufio.NewWriter(conn)
	send := func(i int, cmd string) error {
		fmt.Fprintf(os.Stderr, ">> %s\n", cmd)
		events.record(clientEvent{Event: "send", CommandIndex: i + 1}, time.Now())
		if _, err := writer.WriteString(cmd + "\r\n"); err != nil {
			return err
		}
		return writer.Flush()
	}
	firstCommand := 0
	var loginErr error
	if promptLogin {
		loginReadyObserved := false
		firstCommand, loginErr = loginCommands(commands, loginReady, prompts, done, send, func(phase string) {
			loginReadyObserved = phase == "login_ready"
			events.record(clientEvent{Event: "prompt", Phase: phase}, time.Now())
		})
		if loginErr != nil || (stopAfterLogin && loginReadyObserved) {
			_ = conn.Close()
			firstCommand = len(commands)
		}
	}
	for i := firstCommand; i < len(commands); i++ {
		if err := send(i, commands[i]); err != nil {
			fmt.Fprintf(os.Stderr, "Error sending command: %v\n", err)
			break
		}
		if i < len(commands)-1 {
			time.Sleep(time.Duration(interCmd) * time.Millisecond)
		}
	}

	// Block until the reader has seen `timeout` seconds of silence (or the
	// server closed). We do NOT hard-close from here, so no in-flight bytes are
	// dropped.
	<-done
	events.record(clientEvent{Event: "done"}, time.Now())
	fmt.Fprintf(os.Stderr, "Done.\n")
	if loginErr != nil {
		fmt.Fprintf(os.Stderr, "Login probe failed: %v\n", loginErr)
		_ = events.close()
		os.Exit(1)
	}
}

func loadCommandsFromFile(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var commands []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			commands = append(commands, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return commands, nil
}

func readOutput(conn net.Conn, done chan struct{}, idle time.Duration, events *eventLog, prompts *promptBuffer) {
	defer close(done)

	buf := make([]byte, 4096)
	for {
		conn.SetReadDeadline(time.Now().Add(idle))
		n, err := conn.Read(buf)
		if n > 0 {
			events.record(clientEvent{Event: "receive", Bytes: n, Text: string(buf[:n])}, time.Now())
			os.Stdout.Write(buf[:n])
			prompts.feed(string(buf[:n]))
		}
		if err != nil {
			// Idle timeout or EOF/close: we are done. Any bytes already read
			// above have been printed.
			return
		}
	}
}
