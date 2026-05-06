package server

import (
	"barn/builtins"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

func DefaultListenSpecs(port int) []builtins.ListenerSpec {
	return []builtins.ListenerSpec{{
		Protocol: builtins.ListenerProtocolTCP,
		Port:     int64(port),
	}}
}

func ParseListenSpec(raw string) (builtins.ListenerSpec, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return builtins.ListenerSpec{}, fmt.Errorf("parse listener spec %q: %w", raw, err)
	}
	protocol := strings.ToLower(parsed.Scheme)
	switch protocol {
	case builtins.ListenerProtocolTCP, "tls", "ws", "wss":
	default:
		return builtins.ListenerSpec{}, fmt.Errorf("unsupported listener protocol %q", protocol)
	}
	if parsed.Host == "" {
		return builtins.ListenerSpec{}, fmt.Errorf("listener spec %q missing host/port", raw)
	}

	host, portText, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		return builtins.ListenerSpec{}, fmt.Errorf("listener spec %q requires host:port: %w", raw, err)
	}
	port, err := strconv.ParseInt(portText, 10, 64)
	if err != nil || port < 0 || port > 65535 {
		return builtins.ListenerSpec{}, fmt.Errorf("listener spec %q has invalid port", raw)
	}

	spec := builtins.ListenerSpec{
		Protocol:  protocol,
		Port:      port,
		Interface: host,
		Path:      parsed.EscapedPath(),
	}

	query := parsed.Query()
	spec.TLSCertificatePath = query.Get("cert")
	spec.TLSKeyPath = query.Get("key")

	if spec.Path == "" && (protocol == "ws" || protocol == "wss") {
		spec.Path = "/"
	}
	if spec.Path != "" && protocol != "ws" && protocol != "wss" {
		return builtins.ListenerSpec{}, fmt.Errorf("listener spec %q path is only valid for ws/wss", raw)
	}
	if (protocol == "tls" || protocol == "wss") && (spec.TLSCertificatePath == "" || spec.TLSKeyPath == "") {
		return builtins.ListenerSpec{}, fmt.Errorf("listener spec %q requires cert and key", raw)
	}
	if (protocol == builtins.ListenerProtocolTCP || protocol == "ws") && (spec.TLSCertificatePath != "" || spec.TLSKeyPath != "") {
		return builtins.ListenerSpec{}, fmt.Errorf("listener spec %q includes TLS options for non-TLS protocol", raw)
	}

	return spec, nil
}
