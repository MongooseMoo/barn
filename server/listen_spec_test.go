package server

import (
	"github.com/MongooseMoo/barn/builtins"
	"testing"
)

func TestParseListenSpecTCP(t *testing.T) {
	spec, err := ParseListenSpec("tcp://127.0.0.1:7777")
	if err != nil {
		t.Fatalf("parse tcp spec: %v", err)
	}
	if spec.Protocol != builtins.ListenerProtocolTCP ||
		spec.Interface != "127.0.0.1" ||
		spec.Port != 7777 ||
		spec.Path != "" {
		t.Fatalf("unexpected spec: %+v", spec)
	}
}

func TestParseListenSpecWebSocketDefaultsPath(t *testing.T) {
	spec, err := ParseListenSpec("ws://:7779")
	if err != nil {
		t.Fatalf("parse ws spec: %v", err)
	}
	if spec.Protocol != builtins.ListenerProtocolWebSocket ||
		spec.Interface != "" ||
		spec.Port != 7779 ||
		spec.Path != "/" {
		t.Fatalf("unexpected spec: %+v", spec)
	}
}

func TestParseListenSpecTLSRequiresCertificateAndKey(t *testing.T) {
	_, err := ParseListenSpec("tls://:7778")
	if err == nil {
		t.Fatalf("parsed tls spec without certificate and key")
	}
}

func TestParseListenSpecWSS(t *testing.T) {
	spec, err := ParseListenSpec("wss://:7780/moo?cert=server.crt&key=server.key")
	if err != nil {
		t.Fatalf("parse wss spec: %v", err)
	}
	if spec.Protocol != builtins.ListenerProtocolSecureWebSocket ||
		spec.Port != 7780 ||
		spec.Path != "/moo" ||
		spec.TLSCertificatePath != "server.crt" ||
		spec.TLSKeyPath != "server.key" {
		t.Fatalf("unexpected spec: %+v", spec)
	}
}

func TestParseListenSpecRejectsInvalidPort(t *testing.T) {
	_, err := ParseListenSpec("tcp://:70000")
	if err == nil {
		t.Fatalf("parsed listener spec with invalid port")
	}
}
