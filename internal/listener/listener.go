// Package listener owns the shared listener configuration vocabulary.
package listener

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/MongooseMoo/barn/types"
)

// Protocol identifies a listener transport protocol.
type Protocol string

const (
	ProtocolTCP             Protocol = "tcp"
	ProtocolTLS             Protocol = "tls"
	ProtocolWebSocket       Protocol = "ws"
	ProtocolSecureWebSocket Protocol = "wss"
)

// Spec describes a listener to create.
type Spec struct {
	Protocol           Protocol
	Object             types.ObjID
	Port               int64
	Interface          string
	IPv6               bool
	Path               string
	PrintMessages      bool
	TLSCertificatePath string
	TLSKeyPath         string
}

// Descriptor is the stable identity used to address a listener.
type Descriptor struct {
	Protocol Protocol
	Port     int64
	IPv6     bool
	Path     string
}

// Info describes a currently active listener.
type Info struct {
	Object        types.ObjID
	Port          int64
	Protocol      Protocol
	Path          string
	PrintMessages bool
	IPv6          bool
	Interface     string
	TLS           bool
}

// NormalizeProtocol applies the default protocol and canonical casing.
func NormalizeProtocol(protocol Protocol) Protocol {
	if protocol == "" {
		return ProtocolTCP
	}
	return Protocol(strings.ToLower(string(protocol)))
}

// IsSupportedProtocol reports whether protocol is part of the listener vocabulary.
func IsSupportedProtocol(protocol Protocol) bool {
	switch NormalizeProtocol(protocol) {
	case ProtocolTCP, ProtocolTLS, ProtocolWebSocket, ProtocolSecureWebSocket:
		return true
	default:
		return false
	}
}

// Validate checks the transport-independent constraints of a listener spec.
func (spec Spec) Validate() error {
	protocol := NormalizeProtocol(spec.Protocol)
	if !IsSupportedProtocol(protocol) {
		return fmt.Errorf("unsupported listener protocol %q", protocol)
	}
	if spec.Port < 0 || spec.Port > 65535 {
		return fmt.Errorf("invalid listener port %d", spec.Port)
	}

	isWebSocket := protocol == ProtocolWebSocket || protocol == ProtocolSecureWebSocket
	if spec.Path != "" && !isWebSocket {
		return fmt.Errorf("listener path is only valid for ws/wss")
	}
	if isWebSocket && spec.Path != "" && !strings.HasPrefix(spec.Path, "/") {
		return fmt.Errorf("websocket listener path must start with /")
	}

	isTLS := protocol == ProtocolTLS || protocol == ProtocolSecureWebSocket
	hasCertificate := spec.TLSCertificatePath != ""
	hasKey := spec.TLSKeyPath != ""
	if isTLS && (!hasCertificate || !hasKey) {
		return fmt.Errorf("%s listener requires certificate and key", protocol)
	}
	if !isTLS && (hasCertificate || hasKey) {
		return fmt.Errorf("%s listener does not accept TLS options", protocol)
	}
	return nil
}

// DefaultSpecs returns the default TCP listener for port.
func DefaultSpecs(port int) []Spec {
	return []Spec{{Protocol: ProtocolTCP, Port: int64(port)}}
}

// ParseSpec parses and validates a listener URL.
func ParseSpec(raw string) (Spec, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return Spec{}, fmt.Errorf("parse listener spec %q: %w", raw, err)
	}

	protocol := NormalizeProtocol(Protocol(parsed.Scheme))
	if !IsSupportedProtocol(protocol) {
		return Spec{}, fmt.Errorf("unsupported listener protocol %q", protocol)
	}
	if parsed.Host == "" {
		return Spec{}, fmt.Errorf("listener spec %q missing host/port", raw)
	}

	host, portText, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		return Spec{}, fmt.Errorf("listener spec %q requires host:port: %w", raw, err)
	}
	port, err := strconv.ParseInt(portText, 10, 64)
	if err != nil {
		return Spec{}, fmt.Errorf("listener spec %q has invalid port", raw)
	}

	spec := Spec{
		Protocol:  protocol,
		Port:      port,
		Interface: host,
		Path:      parsed.EscapedPath(),
	}
	query := parsed.Query()
	spec.TLSCertificatePath = query.Get("cert")
	spec.TLSKeyPath = query.Get("key")
	if spec.Path == "" && (protocol == ProtocolWebSocket || protocol == ProtocolSecureWebSocket) {
		spec.Path = "/"
	}
	if err := spec.Validate(); err != nil {
		return Spec{}, fmt.Errorf("listener spec %q: %w", raw, err)
	}
	return spec, nil
}
