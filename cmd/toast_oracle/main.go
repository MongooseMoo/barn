package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <moo-expression>\n", os.Args[0])
		os.Exit(1)
	}

	expr := os.Args[1]
	result, err := evaluateExpression(expr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(result)
}

func evaluateExpression(expr string) (string, error) {
	// Build the command to run toast_moo in emergency mode
	// Run toast_moo.exe directly with -e flag for emergency mode
	cmd := exec.Command("C:/Users/Q/code/barn/toast_moo.exe", "-e", "toastcore.db", "NUL")
	cmd.Dir = "C:/Users/Q/code/barn"

	// Prepare stdin with the MOO expression
	input := fmt.Sprintf(";%s\nquit\n", expr)
	cmd.Stdin = strings.NewReader(input)

	// Capture combined stdout and stderr (Toast writes to stderr)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	// Run the command
	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("failed to run toast_moo: %w\noutput: %s", err, output.String())
	}

	// Parse the output to extract the result
	result, err := parseToastOutput(output.String(), expr)
	if err != nil {
		return "", fmt.Errorf("failed to parse output: %w\noutput:\n%s", err, output.String())
	}

	return result, nil
}

func parseToastOutput(output, expr string) (string, error) {
	// Emergency mode output looks like:
	// (some banner lines)
	// (#2): => RESULT
	// (#2): Bye.  (saving database)

	scanner := bufio.NewScanner(strings.NewReader(output))

	for scanner.Scan() {
		line := scanner.Text()

		// Look for the result line: "(#2): => RESULT"
		if strings.HasPrefix(line, "(#2): => ") {
			// Extract result after "(#2): => "
			result := strings.TrimPrefix(line, "(#2): => ")
			return result, nil
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("scanner error: %w", err)
	}

	return "", fmt.Errorf("could not find result in output")
}
