package listener

import "testing"

func TestProtocolValues(t *testing.T) {
	tests := map[Protocol]string{
		ProtocolTCP:             "tcp",
		ProtocolTLS:             "tls",
		ProtocolWebSocket:       "ws",
		ProtocolSecureWebSocket: "wss",
	}
	for protocol, want := range tests {
		if string(protocol) != want {
			t.Errorf("protocol = %q, want %q", protocol, want)
		}
	}
}

func TestDefaultSpecs(t *testing.T) {
	specs := DefaultSpecs(7777)
	if len(specs) != 1 || specs[0] != (Spec{Protocol: ProtocolTCP, Port: 7777}) {
		t.Fatalf("DefaultSpecs(7777) = %+v", specs)
	}
}

func TestParseSpec(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want Spec
	}{
		{
			name: "TCP",
			raw:  "tcp://127.0.0.1:7777",
			want: Spec{Protocol: ProtocolTCP, Interface: "127.0.0.1", Port: 7777},
		},
		{
			name: "WebSocket default path",
			raw:  "ws://:7779",
			want: Spec{Protocol: ProtocolWebSocket, Port: 7779, Path: "/"},
		},
		{
			name: "secure WebSocket",
			raw:  "wss://:7780/moo?cert=server.crt&key=server.key",
			want: Spec{
				Protocol:           ProtocolSecureWebSocket,
				Port:               7780,
				Path:               "/moo",
				TLSCertificatePath: "server.crt",
				TLSKeyPath:         "server.key",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ParseSpec(test.raw)
			if err != nil {
				t.Fatalf("ParseSpec(%q): %v", test.raw, err)
			}
			if got != test.want {
				t.Fatalf("ParseSpec(%q) = %+v, want %+v", test.raw, got, test.want)
			}
		})
	}
}

func TestParseSpecRejectsInvalidConfiguration(t *testing.T) {
	tests := []string{
		"ftp://:7777",
		"tcp://",
		"tcp://:70000",
		"tcp://:7777/path",
		"tcp://:7777?cert=server.crt&key=server.key",
		"tls://:7778",
	}
	for _, raw := range tests {
		t.Run(raw, func(t *testing.T) {
			if _, err := ParseSpec(raw); err == nil {
				t.Fatalf("ParseSpec(%q) succeeded", raw)
			}
		})
	}
}

func TestSpecValidate(t *testing.T) {
	valid := Spec{Protocol: ProtocolWebSocket, Port: 7777, Path: "/moo"}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid spec: %v", err)
	}

	invalid := valid
	invalid.Path = "moo"
	if err := invalid.Validate(); err == nil {
		t.Fatal("validated WebSocket path without leading slash")
	}
}
