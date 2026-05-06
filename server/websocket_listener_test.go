package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"barn/builtins"
	"barn/db"
	"barn/types"

	"github.com/coder/websocket"
)

const (
	websocketTestTimeout         = 2 * time.Second
	websocketPingStabilityWait   = 50 * time.Millisecond
	websocketShutdownTestTimeout = 500 * time.Millisecond
	websocketShutdownTestPoll    = 5 * time.Millisecond
)

func TestWebSocketListenerReportsMetadataAndRoundTrip(t *testing.T) {
	cm := NewConnectionManager(nil, 0)

	desc, err := cm.AddListener(builtins.ListenerSpec{
		Protocol:      "ws",
		Object:        7,
		Port:          0,
		Interface:     "127.0.0.1",
		Path:          "/moo",
		PrintMessages: true,
	})
	if err != nil {
		t.Fatalf("add ws listener: %v", err)
	}

	if desc.Protocol != "ws" || desc.Port <= 0 || desc.Path != "/moo" {
		t.Fatalf("unexpected descriptor: %+v", desc)
	}

	infos := cm.ListenerInfos()
	if len(infos) != 1 {
		t.Fatalf("got %d listener infos, want 1", len(infos))
	}
	info := infos[0]
	if info.Protocol != "ws" ||
		info.Object != 7 ||
		info.Port != desc.Port ||
		info.Path != "/moo" ||
		!info.PrintMessages ||
		info.TLS ||
		info.Interface != "127.0.0.1" {
		t.Fatalf("unexpected listener info: %+v", info)
	}

	if err := cm.RemoveListener(desc); err != nil {
		t.Fatalf("remove ws listener: %v", err)
	}
}

func TestWebSocketListenerLoginAndEval(t *testing.T) {
	h := startWebSocketHarness(t, "/moo")
	ctx, cancel := context.WithTimeout(context.Background(), websocketTestTimeout)
	defer cancel()

	client, _, err := websocket.Dial(ctx, h.url, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer client.Close(websocket.StatusNormalClosure, "")

	loginWebSocket(t, client)
	if got := readWebSocketText(t, client); got != "*** Connected ***" {
		t.Fatalf("login response %q, want connected message", got)
	}

	writeWebSocketText(t, client, "eval return 3;")
	if got := readWebSocketText(t, client); got != "{1, 3}" {
		t.Fatalf("eval response %q, want {1, 3}", got)
	}
}

func TestWebSocketInputIsOneMessagePerLine(t *testing.T) {
	h := startWebSocketHarness(t, "/moo")
	ctx, cancel := context.WithTimeout(context.Background(), websocketTestTimeout)
	defer cancel()

	client, _, err := websocket.Dial(ctx, h.url, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer client.Close(websocket.StatusNormalClosure, "")
	loginWebSocket(t, client)
	_ = readWebSocketText(t, client)

	writeWebSocketText(t, client, "eval return 1;")
	writeWebSocketText(t, client, "eval return 2;")
	if got := readWebSocketText(t, client); got != "{1, 1}" {
		t.Fatalf("first eval response %q, want {1, 1}", got)
	}
	if got := readWebSocketText(t, client); got != "{1, 2}" {
		t.Fatalf("second eval response %q, want {1, 2}", got)
	}
}

func TestWebSocketRejectsEmbeddedNewline(t *testing.T) {
	h := startWebSocketHarness(t, "/moo")
	ctx, cancel := context.WithTimeout(context.Background(), websocketTestTimeout)
	defer cancel()

	client, _, err := websocket.Dial(ctx, h.url, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer client.Close(websocket.StatusNormalClosure, "")
	loginWebSocket(t, client)
	_ = readWebSocketText(t, client)

	writeWebSocketText(t, client, "eval return 1;\nreturn 2;")
	_, _, err = client.Read(ctx)
	if err == nil {
		t.Fatalf("read after embedded newline succeeded, want close")
	}
	if status := websocket.CloseStatus(err); status != websocket.StatusPolicyViolation {
		t.Fatalf("close status %v, want policy violation", status)
	}
}

func TestWebSocketRejectsBinaryInput(t *testing.T) {
	h := startWebSocketHarness(t, "/moo")
	ctx, cancel := context.WithTimeout(context.Background(), websocketTestTimeout)
	defer cancel()

	client, _, err := websocket.Dial(ctx, h.url, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer client.Close(websocket.StatusNormalClosure, "")
	loginWebSocket(t, client)
	_ = readWebSocketText(t, client)

	if err := client.Write(ctx, websocket.MessageBinary, []byte{0x01, 0x02}); err != nil {
		t.Fatalf("write binary message: %v", err)
	}
	_, _, err = client.Read(ctx)
	if err == nil {
		t.Fatalf("read after binary message succeeded, want close")
	}
	if status := websocket.CloseStatus(err); status != websocket.StatusUnsupportedData {
		t.Fatalf("close status %v, want unsupported data", status)
	}
}

func TestWebSocketPingDoesNotSurfaceAsInput(t *testing.T) {
	h := startWebSocketHarness(t, "/moo")
	ctx, cancel := context.WithTimeout(context.Background(), websocketTestTimeout)
	defer cancel()

	client, _, err := websocket.Dial(ctx, h.url, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer client.Close(websocket.StatusNormalClosure, "")
	loginWebSocket(t, client)
	_ = readWebSocketText(t, client)

	closeCtx := client.CloseRead(context.Background())
	pingCtx, pingCancel := context.WithTimeout(context.Background(), websocketTestTimeout)
	defer pingCancel()
	if err := client.Ping(pingCtx); err != nil {
		t.Fatalf("ping websocket: %v", err)
	}
	select {
	case <-closeCtx.Done():
		t.Fatalf("websocket closed after ping")
	case <-time.After(websocketPingStabilityWait):
	}
}

func TestWebSocketHTTPPolicy(t *testing.T) {
	h := startWebSocketHarness(t, "/moo")

	resp, err := http.Get(strings.Replace(h.url, "ws://", "http://", 1))
	if err != nil {
		t.Fatalf("plain http get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUpgradeRequired {
		t.Fatalf("plain http status %d, want 426", resp.StatusCode)
	}

	ctx, cancel := context.WithTimeout(context.Background(), websocketTestTimeout)
	defer cancel()
	_, resp, err = websocket.Dial(ctx, strings.Replace(h.url, "/moo", "/other", 1), nil)
	if err == nil {
		t.Fatalf("websocket dial to wrong path succeeded")
	}
	if resp == nil || resp.StatusCode != http.StatusNotFound {
		t.Fatalf("wrong-path status %v, want 404", responseStatus(resp))
	}

	client, _, err := websocket.Dial(ctx, h.url, &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": []string{"https://example.invalid"}},
	})
	if err != nil {
		t.Fatalf("dial with cross origin header: %v", err)
	}
	defer client.Close(websocket.StatusNormalClosure, "")
}

func TestWebSocketShutdownClosesActiveConnection(t *testing.T) {
	h := startWebSocketHarness(t, "/moo")
	h.cm.shutdownTimeout = websocketShutdownTestTimeout
	h.cm.shutdownPoll = websocketShutdownTestPoll
	ctx, cancel := context.WithTimeout(context.Background(), websocketTestTimeout)
	defer cancel()

	client, _, err := websocket.Dial(ctx, h.url, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer client.Close(websocket.StatusNormalClosure, "")
	loginWebSocket(t, client)
	_ = readWebSocketText(t, client)

	h.cm.Shutdown()
	_, _, err = client.Read(ctx)
	if err == nil {
		t.Fatalf("read after connection manager shutdown succeeded, want close")
	}
}

func TestWebSocketPreLoginTimeoutClosesConnection(t *testing.T) {
	h := startWebSocketHarnessWithConnectTimeout(t, "/moo", 1)
	ctx, cancel := context.WithTimeout(context.Background(), websocketTestTimeout)
	defer cancel()

	client, _, err := websocket.Dial(ctx, h.url, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer client.Close(websocket.StatusNormalClosure, "")

	messageType, payload, err := client.Read(ctx)
	if err != nil {
		t.Fatalf("read timeout message: %v", err)
	}
	if messageType != websocket.MessageText || string(payload) != "*** Timed-out waiting for login. ***" {
		t.Fatalf("timeout message type=%v payload=%q", messageType, string(payload))
	}
	_, _, err = client.Read(ctx)
	if err == nil {
		t.Fatalf("read after pre-login timeout succeeded, want close")
	}
}

func TestWSSListenerLoginAndEval(t *testing.T) {
	h := startSecureWebSocketHarness(t, "/moo")
	ctx, cancel := context.WithTimeout(context.Background(), websocketTestTimeout)
	defer cancel()

	client, _, err := websocket.Dial(ctx, h.url, insecureWebSocketDialOptions())
	if err != nil {
		t.Fatalf("dial secure websocket: %v", err)
	}
	defer client.Close(websocket.StatusNormalClosure, "")

	loginWebSocket(t, client)
	if got := readWebSocketText(t, client); got != "*** Connected ***" {
		t.Fatalf("login response %q, want connected message", got)
	}

	writeWebSocketText(t, client, "eval return 5;")
	if got := readWebSocketText(t, client); got != "{1, 5}" {
		t.Fatalf("eval response %q, want {1, 5}", got)
	}
}

func TestWSSHTTPPolicy(t *testing.T) {
	h := startSecureWebSocketHarness(t, "/moo")
	httpClient := &http.Client{Transport: insecureHTTPTransport()}

	resp, err := httpClient.Get(strings.Replace(h.url, "wss://", "https://", 1))
	if err != nil {
		t.Fatalf("plain https get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUpgradeRequired {
		t.Fatalf("plain https status %d, want 426", resp.StatusCode)
	}

	ctx, cancel := context.WithTimeout(context.Background(), websocketTestTimeout)
	defer cancel()
	_, resp, err = websocket.Dial(ctx, strings.Replace(h.url, "/moo", "/other", 1), insecureWebSocketDialOptions())
	if err == nil {
		t.Fatalf("secure websocket dial to wrong path succeeded")
	}
	if resp == nil || resp.StatusCode != http.StatusNotFound {
		t.Fatalf("wrong-path status %v, want 404", responseStatus(resp))
	}

	client, _, err := websocket.Dial(ctx, h.url, &websocket.DialOptions{
		HTTPClient: insecureHTTPClient(),
		HTTPHeader: http.Header{"Origin": []string{"https://example.invalid"}},
	})
	if err != nil {
		t.Fatalf("dial secure websocket with cross origin header: %v", err)
	}
	defer client.Close(websocket.StatusNormalClosure, "")
}

func TestWSSHandshakeFailureDoesNotCreateBarnConnection(t *testing.T) {
	h := startSecureWebSocketHarness(t, "/moo")
	port := h.cm.ListenerInfos()[0].Port

	conn, err := tls.Dial("tcp", net.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", port)), &tls.Config{ServerName: "localhost"})
	if err == nil {
		_ = conn.Close()
		t.Fatalf("TLS handshake unexpectedly succeeded")
	}

	h.cm.mu.Lock()
	connections := len(h.cm.connections)
	h.cm.mu.Unlock()
	if connections != 0 {
		t.Fatalf("got %d Barn connections after failed TLS handshake, want 0", connections)
	}
}

func TestWSSShutdownClosesActiveConnection(t *testing.T) {
	h := startSecureWebSocketHarness(t, "/moo")
	h.cm.shutdownTimeout = websocketShutdownTestTimeout
	h.cm.shutdownPoll = websocketShutdownTestPoll
	ctx, cancel := context.WithTimeout(context.Background(), websocketTestTimeout)
	defer cancel()

	client, _, err := websocket.Dial(ctx, h.url, insecureWebSocketDialOptions())
	if err != nil {
		t.Fatalf("dial secure websocket: %v", err)
	}
	defer client.Close(websocket.StatusNormalClosure, "")
	loginWebSocket(t, client)
	_ = readWebSocketText(t, client)

	h.cm.Shutdown()
	_, _, err = client.Read(ctx)
	if err == nil {
		t.Fatalf("read after connection manager shutdown succeeded, want close")
	}
}

type websocketHarness struct {
	cm  *ConnectionManager
	url string
}

func startWebSocketHarness(t *testing.T, path string) websocketHarness {
	t.Helper()
	return startWebSocketHarnessWithConnectTimeout(t, path, 0)
}

func startWebSocketHarnessWithConnectTimeout(t *testing.T, path string, connectTimeout int64) websocketHarness {
	t.Helper()
	return startWebSocketHarnessWithSpec(t, "ws", builtins.ListenerSpec{
		Protocol:  "ws",
		Port:      0,
		Interface: "127.0.0.1",
		Path:      path,
	}, connectTimeout)
}

func startSecureWebSocketHarness(t *testing.T, path string) websocketHarness {
	t.Helper()
	certPath, keyPath := writeSelfSignedCertificate(t)
	return startWebSocketHarnessWithSpec(t, "wss", builtins.ListenerSpec{
		Protocol:           "wss",
		Port:               0,
		Interface:          "127.0.0.1",
		Path:               path,
		TLSCertificatePath: certPath,
		TLSKeyPath:         keyPath,
	}, 0)
}

func startWebSocketHarnessWithSpec(t *testing.T, scheme string, spec builtins.ListenerSpec, connectTimeout int64) websocketHarness {
	t.Helper()

	store := db.NewStore()
	system := addTestObject(t, store, 0, db.FlagWizard)
	addTestObject(t, store, 2, db.FlagUser|db.FlagProgrammer|db.FlagWizard)
	addTestVerb(system, "do_login_command", "return #2;")
	if connectTimeout > 0 {
		serverOptions := addTestObject(t, store, 9, db.FlagWizard)
		serverOptions.Properties["connect_timeout"] = &db.Property{
			Name:  "connect_timeout",
			Value: types.NewInt(connectTimeout),
			Owner: 2,
			Perms: db.PropRead | db.PropWrite,
		}
		system.Properties["server_options"] = &db.Property{
			Name:  "server_options",
			Value: types.NewObj(9),
			Owner: 2,
			Perms: db.PropRead | db.PropWrite,
		}
	}

	scheduler := NewScheduler(store)
	srv := &Server{scheduler: scheduler}
	cm := NewConnectionManager(srv, 0)
	scheduler.SetConnectionManager(cm)
	scheduler.Start()
	t.Cleanup(scheduler.Stop)

	err := cm.StartListeners([]builtins.ListenerSpec{spec})
	if err != nil {
		t.Fatalf("start %s listener: %v", spec.Protocol, err)
	}
	t.Cleanup(func() { closeAllListeners(cm) })

	port := cm.ListenerInfos()[0].Port
	return websocketHarness{
		cm:  cm,
		url: scheme + "://" + net.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", port)) + spec.Path,
	}
}

func insecureWebSocketDialOptions() *websocket.DialOptions {
	return &websocket.DialOptions{HTTPClient: insecureHTTPClient()}
}

func insecureHTTPClient() *http.Client {
	return &http.Client{Transport: insecureHTTPTransport()}
}

func insecureHTTPTransport() *http.Transport {
	return &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
}

func readWebSocketText(t *testing.T, conn *websocket.Conn) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), websocketTestTimeout)
	defer cancel()

	messageType, payload, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read websocket message: %v", err)
	}
	if messageType != websocket.MessageText {
		t.Fatalf("got message type %v, want text", messageType)
	}
	return string(payload)
}

func loginWebSocket(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	writeWebSocketText(t, conn, "connect test")
}

func writeWebSocketText(t *testing.T, conn *websocket.Conn, message string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), websocketTestTimeout)
	defer cancel()

	if err := conn.Write(ctx, websocket.MessageText, []byte(message)); err != nil {
		t.Fatalf("write websocket message: %v", err)
	}
}

func responseStatus(resp *http.Response) string {
	if resp == nil {
		return "<nil>"
	}
	return resp.Status
}
