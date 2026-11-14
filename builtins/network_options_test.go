package builtins

import (
	"net"
	"testing"

	"github.com/MongooseMoo/barn/config"
	"github.com/MongooseMoo/barn/internal/listener"
	"github.com/MongooseMoo/barn/types"
)

func wizardNetworkCtx(cm ConnectionManager) *Execution {
	ctx := ctxWithConnManager(cm)
	ctx.IsWizard = true
	ctx.RuntimeOptions = config.Options{OutboundNetwork: true}
	return ctx
}

func optionMap(pairs ...types.Value) types.Value {
	entries := make([][2]types.Value, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		entries = append(entries, [2]types.Value{pairs[i], pairs[i+1]})
	}
	return types.NewMap(entries)
}

// Toast server.cc bf_listen looks the TLS keys up with maplookup(..., 0),
// which ignores case, and makes the listener TLS when "TLS" is true.
func TestListenTLSKeyInAnyCaseMakesTLSListener(t *testing.T) {
	cm := &stubConnManager{}
	result := builtinListen(wizardNetworkCtx(cm), []types.Value{
		types.NewObj(0), types.NewInt(7000),
		optionMap(types.NewStr("TLS"), types.NewInt(1),
			types.NewStr("Certificate"), types.NewStr("c.pem"),
			types.NewStr("KEY"), types.NewStr("k.pem")),
	})
	if !result.IsNormal() {
		t.Fatalf("listen raised %s", result.Error)
	}
	if cm.added.Protocol != listener.ProtocolTLS || cm.added.TLSCertificatePath != "c.pem" || cm.added.TLSKeyPath != "k.pem" {
		t.Fatalf("listener spec = %+v, want TLS with c.pem/k.pem", cm.added)
	}
}

// Toast keeps certificate and key paths but uses them only when TLS is on.
func TestListenCertificateAndKeyWithoutTLSAreIgnored(t *testing.T) {
	cm := &stubConnManager{}
	result := builtinListen(wizardNetworkCtx(cm), []types.Value{
		types.NewObj(0), types.NewInt(7000),
		optionMap(types.NewStr("certificate"), types.NewStr("c.pem"),
			types.NewStr("key"), types.NewStr("k.pem")),
	})
	if !result.IsNormal() {
		t.Fatalf("listen raised %s", result.Error)
	}
	if cm.added.Protocol != listener.ProtocolTCP || cm.added.TLSCertificatePath != "" || cm.added.TLSKeyPath != "" {
		t.Fatalf("listener spec = %+v, want plain TCP without paths", cm.added)
	}
}

// Toast raises (E_INVARG, message, 0) for a non-string path; with both bad,
// the key message is the one left, because the key is examined last.
func TestListenNonStringCertificateOrKeyRaisesInvargMessage(t *testing.T) {
	cases := []struct {
		name    string
		options types.Value
		message string
	}{
		{"certificate", optionMap(types.NewStr("tls"), types.NewInt(1), types.NewStr("certificate"), types.NewInt(1), types.NewStr("key"), types.NewStr("k.pem")), "Certificate path should be a string"},
		{"key", optionMap(types.NewStr("tls"), types.NewInt(1), types.NewStr("certificate"), types.NewStr("c.pem"), types.NewStr("key"), types.NewList(nil)), "Private key path should be a string"},
		{"both", optionMap(types.NewStr("key"), types.NewInt(2), types.NewStr("certificate"), types.NewInt(1)), "Private key path should be a string"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := builtinListen(wizardNetworkCtx(&stubConnManager{}), []types.Value{types.NewObj(0), types.NewInt(7000), tc.options})
			want := types.NewList([]types.Value{types.NewErr(types.E_INVARG), types.NewStr(tc.message), types.NewInt(0)})
			if result.Flow != types.FlowException || result.Error != types.E_INVARG || !result.Val.Equal(want) {
				t.Fatalf("result = %+v, want raise %s", result, want.String())
			}
		})
	}
}

// Toast server.cc bf_open_network_connection passes the ipv6 option (any case)
// to getaddrinfo as the address family.
func TestOpenNetworkConnectionPassesIPv6Option(t *testing.T) {
	cm := &stubConnManager{}
	ctx := wizardNetworkCtx(cm)
	if result := builtinOpenNetworkConnection(ctx, []types.Value{types.NewStr("::1"), types.NewInt(7000), optionMap(types.NewStr("IPv6"), types.NewInt(1))}); !result.IsNormal() {
		t.Fatalf("open_network_connection raised %s", result.Error)
	}
	if !cm.dialIPv6 {
		t.Fatal("ipv6 option was not passed to the connection manager")
	}
	if result := builtinOpenNetworkConnection(ctx, []types.Value{types.NewStr("127.0.0.1"), types.NewInt(7000)}); !result.IsNormal() {
		t.Fatalf("open_network_connection raised %s", result.Error)
	}
	if cm.dialIPv6 {
		t.Fatal("ipv6 requested without the option")
	}
}

// Toast network.cc open_connection raises (E_INVARG, "getaddrinfo error",
// detail) when the host does not resolve in the requested family.
func TestOpenNetworkConnectionLookupFailureRaisesGetaddrinfoError(t *testing.T) {
	cm := &stubConnManager{dialErr: &HostLookupError{Detail: "no suitable address found", Err: &net.AddrError{Err: "no suitable address found", Addr: "::1"}}}
	result := builtinOpenNetworkConnection(wizardNetworkCtx(cm), []types.Value{types.NewStr("::1"), types.NewInt(7000)})
	want := types.NewList([]types.Value{types.NewErr(types.E_INVARG), types.NewStr("getaddrinfo error"), types.NewStr("no suitable address found")})
	if result.Flow != types.FlowException || result.Error != types.E_INVARG || !result.Val.Equal(want) {
		t.Fatalf("result = %+v, want raise %s", result, want.String())
	}
}
