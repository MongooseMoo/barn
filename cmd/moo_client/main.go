package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"
)

type arrayFlags []string

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

	flag.Var(&commands, "cmd", "Command to send (can be specified multiple times)")
	flag.IntVar(&port, "port", 7777, "MOO server port")
	flag.StringVar(&host, "host", "localhost", "MOO server host")
	flag.StringVar(&file, "file", "", "File containing commands (one per line)")
	flag.IntVar(&timeout, "timeout", 3, "Seconds to wait after last command")
	flag.Parse()

	// Load commands from file if specified
	if file != "" {
		fileCommands, err := loadCommandsFromFile(file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading commands file: %v\n", err)
			os.Exit(1)
		}
		commands = append(commands, fileCommands...)
	}

	if len(commands) == 0 {
		fmt.Fprintf(os.Stderr, "No commands specified. Use -cmd or -file.\n")
		flag.Usage()
		os.Exit(1)
	}

	// Connect to MOO server
	address := fmt.Sprintf("%s:%d", host, port)
	fmt.Fprintf(os.Stderr, "Connecting to %s...\n", address)

	conn, err := net.Dial("tcp", address)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Connection failed: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	fmt.Fprintf(os.Stderr, "Connected.\n")

	// Start goroutine to read and print all server output
	done := make(chan bool)
	go readOutput(conn, done)

	// Send commands with delays
	writer := bufio.NewWriter(conn)
	for i, cmd := range commands {
		fmt.Fprintf(os.Stderr, "Sending command %d: %s\n", i+1, cmd)
		_, err := writer.WriteString(cmd + "\n")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error sending command: %v\n", err)
			os.Exit(1)
		}
		writer.Flush()

		// Small delay between commands
		if i < len(commands)-1 {
			time.Sleep(100 * time.Millisecond)
		}
	}

	// Wait for timeout after last command to collect output
	fmt.Fprintf(os.Stderr, "Waiting %d seconds for output...\n", timeout)
	time.Sleep(time.Duration(timeout) * time.Second)

	// Close connection (this will cause readOutput to finish)
	conn.Close()
	<-done

	fmt.Fprintf(os.Stderr, "Done.\n")
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

func readOutput(conn net.Conn, done chan bool) {
	defer func() { done <- true }()

	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				fmt.Fprintf(os.Stderr, "Read error: %v\n", err)
			}
			return
		}
		fmt.Print(line)
	}
}
