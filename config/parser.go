package config

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// LoadFile parses a Barn .conf file.
func LoadFile(path string) (Options, error) {
	file, err := os.Open(path)
	if err != nil {
		return Options{}, err
	}
	defer file.Close()

	return Parse(file, path)
}

// Parse reads KEY = VALUE option assignments from r.
func Parse(r io.Reader, source string) (Options, error) {
	options := DefaultOptions()
	seen := map[string]bool{}

	scanner := bufio.NewScanner(r)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return Options{}, parseError(source, lineNumber, "expected KEY = VALUE")
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			return Options{}, parseError(source, lineNumber, "expected non-empty key and value")
		}
		if seen[key] {
			return Options{}, parseError(source, lineNumber, fmt.Sprintf("duplicate option %s", key))
		}
		seen[key] = true

		parsed, err := parseBool01(value)
		if err != nil {
			return Options{}, parseError(source, lineNumber, fmt.Sprintf("%s must be 0 or 1", key))
		}

		switch key {
		case "OUTBOUND_NETWORK":
			options.OutboundNetwork = parsed
		case "PROMOTE_NUMBERS":
			options.PromoteNumbers = parsed
		default:
			return Options{}, parseError(source, lineNumber, fmt.Sprintf("unknown option %s", key))
		}
	}
	if err := scanner.Err(); err != nil {
		return Options{}, err
	}
	if err := options.Validate(); err != nil {
		return Options{}, err
	}
	return options, nil
}

func parseBool01(value string) (bool, error) {
	switch value {
	case "0":
		return false, nil
	case "1":
		return true, nil
	default:
		return false, fmt.Errorf("invalid boolean")
	}
}

func parseError(source string, lineNumber int, message string) error {
	if source == "" {
		source = "<input>"
	}
	return fmt.Errorf("%s:%d: %s", source, lineNumber, message)
}
