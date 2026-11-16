// moo_client sends commands to a MOO server and prints the raw output.
//
//	moo_client -p 7777 -c 'connect wizard' -c 'look'
//	moo_client --file commands.txt --event-log events.jsonl
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/MongooseMoo/barn/internal/cli"
)

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

func main() {
	os.Exit(cli.Execute(newRootCommand(), os.Args[1:], os.Stdout, os.Stderr))
}

// clientOptions are what the flags of one invocation set.
type clientOptions struct {
	commands                    []string
	port                        int
	host, file                  string
	timeout, bannerWait         int
	interCmd, maxDuration       int
	eventLogPath, loginReady    string
	promptLogin, stopAfterLogin bool
}

func newRootCommand() *cobra.Command {
	var o clientOptions
	root := cli.NewRoot("moo_client", "Send commands to a MOO server and print what comes back")
	root.Long = `Connect to a MOO server, send each command, and print the raw output until the
connection has been idle for --timeout seconds. Progress goes to stderr, server
output to stdout.`
	root.Example = `  moo_client -p 7777 -c 'connect wizard' -c 'look'
  moo_client --host mud.example --file commands.txt --event-log events.jsonl`
	root.Args = cobra.NoArgs
	flags := root.Flags()
	flags.StringArrayVarP(&o.commands, "cmd", "c", nil, "`Command` to send, repeatable")
	flags.IntVarP(&o.port, "port", "p", 7777, "MOO server `port`")
	flags.StringVar(&o.host, "host", "localhost", "MOO server `host`")
	flags.StringVarP(&o.file, "file", "f", "", "`File` of commands, one per line (blank lines and # comments skipped)")
	flags.IntVar(&o.timeout, "timeout", 3, "`Seconds` of idle silence before exiting")
	flags.IntVar(&o.bannerWait, "banner-wait", 0, "`Milliseconds` to wait for the connect banner before sending the first command")
	flags.IntVar(&o.interCmd, "inter-cmd", 300, "`Milliseconds` to wait between commands")
	flags.StringVar(&o.eventLogPath, "event-log", "", "Write timestamped client events as JSONL, without command text, to this `file`")
	flags.IntVar(&o.maxDuration, "max-duration", 0, "Maximum connection duration in `seconds` (0 waits for idle timeout)")
	flags.BoolVar(&o.promptLogin, "login-prompts", false, "Respond to account prompts instead of fixed login delays")
	flags.BoolVar(&o.stopAfterLogin, "stop-after-login", false, "Close after the welcome marker; failed hooks retain the receive timeout")
	flags.StringVar(&o.loginReady, "login-ready", "MESSAGE OF THE DAY:", "`Marker` indicating completion of the welcome hook")
	root.RunE = cli.RunE(func(*cobra.Command, []string) error { return runClient(o) })
	return root
}

func runClient(o clientOptions) error {
	commands, port, host, file := o.commands, o.port, o.host, o.file
	timeout, bannerWait, interCmd, maxDuration := o.timeout, o.bannerWait, o.interCmd, o.maxDuration
	promptLogin, stopAfterLogin, loginReady := o.promptLogin, o.stopAfterLogin, o.loginReady

	started := time.Now()
	events, err := newEventLog(o.eventLogPath, started)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening event log: %v\n", err)
		return cli.Exit(1)
	}
	defer func() { _ = events.close() }()

	// Load commands from file if specified
	if file != "" {
		fileCommands, err := loadCommandsFromFile(file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading commands file: %v\n", err)
			return cli.Exit(1)
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
		return cli.Exit(1)
	}
	defer func() { _ = conn.Close() }()
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
		return cli.Exit(1)
	}
	return nil
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
		_ = conn.SetReadDeadline(time.Now().Add(idle))
		n, err := conn.Read(buf)
		if n > 0 {
			events.record(clientEvent{Event: "receive", Bytes: n, Text: string(buf[:n])}, time.Now())
			_, _ = os.Stdout.Write(buf[:n])
			prompts.feed(string(buf[:n]))
		}
		if err != nil {
			// Idle timeout or EOF/close: we are done. Any bytes already read
			// above have been printed.
			return
		}
	}
}
