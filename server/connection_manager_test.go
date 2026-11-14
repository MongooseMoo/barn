package server

import (
	"bufio"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"github.com/MongooseMoo/barn/builtins"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/engine"
	listenercfg "github.com/MongooseMoo/barn/internal/listener"
	"github.com/MongooseMoo/barn/types"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeListener struct {
	addr     net.Addr
	closed   bool
	mu       sync.Mutex
	accepts  int
	acceptCh chan struct{}
}

type failingListener struct {
	addr     net.Addr
	failures int
	accepts  int
	accepted chan time.Time
}

func (l *failingListener) Accept() (net.Conn, error) {
	l.accepts++
	if l.accepts <= l.failures {
		return nil, errors.New("temporary accept failure")
	}
	l.accepted <- time.Now()
	return nil, net.ErrClosed
}

func (l *failingListener) Close() error   { return nil }
func (l *failingListener) Addr() net.Addr { return l.addr }

func TestAcceptConnectionsBacksOffAfterFailures(t *testing.T) {
	listener := &failingListener{
		addr:     &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8888},
		failures: 3,
		accepted: make(chan time.Time, 1),
	}
	cm := &ConnectionManager{}
	started := time.Now()
	go cm.acceptConnections(&listenerRecord{listener: listener})

	select {
	case finished := <-listener.accepted:
		// Three failures should wait for 5ms, 10ms, and 20ms. Allow a
		// small margin so the assertion is not sensitive to timer precision.
		if elapsed := finished.Sub(started); elapsed < 30*time.Millisecond {
			t.Fatalf("fourth Accept called after %v, want at least 30ms of backoff", elapsed)
		}
	case <-time.After(time.Second):
		t.Fatal("accept loop did not retry after failures")
	}
}

func (l *fakeListener) Accept() (net.Conn, error) {
	l.mu.Lock()
	l.accepts++
	acceptCh := l.acceptCh
	l.mu.Unlock()
	if acceptCh != nil {
		select {
		case acceptCh <- struct{}{}:
		default:
		}
	}
	return nil, net.ErrClosed
}

func (l *fakeListener) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closed = true
	return nil
}

func (l *fakeListener) Addr() net.Addr {
	return l.addr
}

func (l *fakeListener) acceptCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.accepts
}

type blockingListener struct {
	addr    net.Addr
	entered chan struct{}
	release chan struct{}
	closed  bool
	mu      sync.Mutex
}

func newBlockingListener() *blockingListener {
	return &blockingListener{
		addr:    &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8888},
		entered: make(chan struct{}, 1),
		release: make(chan struct{}),
	}
}

func (l *blockingListener) Accept() (net.Conn, error) {
	select {
	case l.entered <- struct{}{}:
	default:
	}
	<-l.release
	return nil, net.ErrClosed
}

func (l *blockingListener) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closed = true
	return nil
}

func (l *blockingListener) Addr() net.Addr {
	return l.addr
}

func (l *blockingListener) releaseAccept() {
	close(l.release)
}

func (l *blockingListener) isClosed() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.closed
}

type singleConnListener struct {
	addr   net.Addr
	conn   net.Conn
	closed chan struct{}
	once   sync.Once
}

func newSingleConnListener(conn net.Conn) *singleConnListener {
	return &singleConnListener{
		addr:   &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8888},
		conn:   conn,
		closed: make(chan struct{}),
	}
}

func (l *singleConnListener) Accept() (net.Conn, error) {
	if l.conn != nil {
		conn := l.conn
		l.conn = nil
		return conn, nil
	}
	<-l.closed
	return nil, net.ErrClosed
}

func (l *singleConnListener) Close() error {
	l.once.Do(func() { close(l.closed) })
	return nil
}

func (l *singleConnListener) Addr() net.Addr {
	return l.addr
}

type setupBlockingConn struct {
	readStarted chan struct{}
	releaseRead chan struct{}
	releaseOnce sync.Once
	closed      bool
	mu          sync.Mutex
}

func newSetupBlockingConn() *setupBlockingConn {
	return &setupBlockingConn{
		readStarted: make(chan struct{}),
		releaseRead: make(chan struct{}),
	}
}

func (c *setupBlockingConn) Read([]byte) (int, error) {
	select {
	case <-c.readStarted:
	default:
		close(c.readStarted)
	}
	<-c.releaseRead
	return 0, net.ErrClosed
}

func (c *setupBlockingConn) Write(b []byte) (int, error) {
	return len(b), nil
}

func (c *setupBlockingConn) Close() error {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	return nil
}

func (c *setupBlockingConn) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 7777}
}

func (c *setupBlockingConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8889}
}

func (c *setupBlockingConn) SetDeadline(time.Time) error {
	return nil
}

func (c *setupBlockingConn) SetReadDeadline(time.Time) error {
	return nil
}

func (c *setupBlockingConn) SetWriteDeadline(time.Time) error {
	return nil
}

func (c *setupBlockingConn) release() {
	c.releaseOnce.Do(func() { close(c.releaseRead) })
}

func (c *setupBlockingConn) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

type blockingHijackResponseWriter struct {
	header      http.Header
	hijackOnce  sync.Once
	entered     chan struct{}
	release     chan struct{}
	releaseOnce sync.Once
}

func newBlockingHijackResponseWriter() *blockingHijackResponseWriter {
	return &blockingHijackResponseWriter{
		header:  make(http.Header),
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (w *blockingHijackResponseWriter) Header() http.Header {
	return w.header
}

func (w *blockingHijackResponseWriter) Write(b []byte) (int, error) {
	return len(b), nil
}

func (w *blockingHijackResponseWriter) WriteHeader(int) {}

func (w *blockingHijackResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	w.hijackOnce.Do(func() { close(w.entered) })
	<-w.release
	return nil, nil, net.ErrClosed
}

func (w *blockingHijackResponseWriter) releaseHijack() {
	w.releaseOnce.Do(func() { close(w.release) })
}

type recordingTransport struct {
	mu       sync.Mutex
	lines    []string
	closed   bool
	remote   string
	readLine chan string
}

func newRecordingTransport(remote string) *recordingTransport {
	return &recordingTransport{
		remote:   remote,
		readLine: make(chan string),
	}
}

func (t *recordingTransport) ReadLine() (string, error) {
	line, ok := <-t.readLine
	if !ok {
		return "", net.ErrClosed
	}
	return line, nil
}

func (t *recordingTransport) WriteOutput(line string, newline bool) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.lines = append(t.lines, line)
	return nil
}

func (t *recordingTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.closed {
		t.closed = true
		close(t.readLine)
	}
	return nil
}

func (t *recordingTransport) RemoteAddr() string {
	return t.remote
}

func (t *recordingTransport) writtenLines() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]string(nil), t.lines...)
}

func (t *recordingTransport) isClosed() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.closed
}

func TestCheckpointConnectionsPreservesPlayerListenerPairs(t *testing.T) {
	cm := NewConnectionManager(7777)

	unlogged := cm.NewConnectionFromTransport(newRecordingTransport("unlogged"))
	unlogged.SetListener(4, 7777, true)

	loggedIn := cm.NewConnectionFromTransport(newRecordingTransport("logged-in"))
	loggedIn.SetListener(5, 8888, false)
	loggedIn.SetPlayer(9)

	got := cm.CheckpointConnections()
	if len(got) != 2 {
		t.Fatalf("checkpoint connection count = %d, want 2", len(got))
	}
	if got[0].Player != types.ObjID(-unlogged.ID) || got[0].Listener != 4 {
		t.Fatalf("unlogged checkpoint connection = %#v, want player %d listener 4", got[0], -unlogged.ID)
	}
	if got[1].Player != 9 || got[1].Listener != 5 {
		t.Fatalf("logged-in checkpoint connection = %#v, want player 9 listener 5", got[1])
	}
}

func TestListenerDescriptorsUseProtocolPathKey(t *testing.T) {
	cm := NewConnectionManager(7777)
	tcp := &fakeListener{addr: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8888}}

	desc, err := cm.registerListener(tcp, listenercfg.Spec{
		Protocol: listenercfg.ProtocolTCP,
		Object:   5,
	}, false, nil)
	if err != nil {
		t.Fatalf("register tcp listener: %v", err)
	}

	err = cm.RemoveListener(listenercfg.Descriptor{
		Protocol: listenercfg.ProtocolWebSocket,
		Port:     8888,
		Path:     "/",
	})
	if err == nil {
		t.Fatalf("removed tcp listener with ws descriptor")
	}
	if tcp.closed {
		t.Fatalf("wrong descriptor closed tcp listener")
	}

	if err := cm.RemoveListener(desc); err != nil {
		t.Fatalf("remove tcp listener: %v", err)
	}
	if !tcp.closed {
		t.Fatalf("tcp listener was not closed")
	}
}

func TestListenerDescriptorsUseIPv6Key(t *testing.T) {
	cm := NewConnectionManager(7777)
	tcp := &fakeListener{addr: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8888}}

	desc, err := cm.registerListener(tcp, listenercfg.Spec{
		Protocol: listenercfg.ProtocolTCP,
		Object:   5,
	}, false, nil)
	if err != nil {
		t.Fatalf("register tcp listener: %v", err)
	}

	err = cm.RemoveListener(listenercfg.Descriptor{
		Protocol: listenercfg.ProtocolTCP,
		Port:     8888,
		IPv6:     true,
	})
	if err == nil {
		t.Fatalf("removed ipv4 listener with ipv6 descriptor")
	}
	if tcp.closed {
		t.Fatalf("wrong descriptor closed tcp listener")
	}

	if err := cm.RemoveListener(desc); err != nil {
		t.Fatalf("remove tcp listener: %v", err)
	}
	if !tcp.closed {
		t.Fatalf("tcp listener was not closed")
	}
}

func TestRegisterListenerRejectsDuplicateDescriptor(t *testing.T) {
	cm := NewConnectionManager(7777)
	first := &fakeListener{addr: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8888}}
	second := &fakeListener{addr: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8888}}

	_, err := cm.registerListener(first, listenercfg.Spec{
		Protocol: listenercfg.ProtocolTCP,
		Object:   5,
	}, false, nil)
	if err != nil {
		t.Fatalf("register first listener: %v", err)
	}

	_, err = cm.registerListener(second, listenercfg.Spec{
		Protocol: listenercfg.ProtocolTCP,
		Object:   6,
	}, false, nil)
	if err == nil {
		t.Fatalf("registered duplicate listener descriptor")
	}
	if !second.closed {
		t.Fatalf("duplicate listener was not closed")
	}
	if first.closed {
		t.Fatalf("first listener was closed by duplicate registration")
	}
}

func TestRuntimeListenerZeroPreservesMOODescriptorAndReleasesBoundSocket(t *testing.T) {
	cm := NewConnectionManager(0)
	t.Cleanup(cm.CloseListeners)

	desc, err := cm.AddListener(listenercfg.Spec{
		Protocol:  listenercfg.ProtocolTCP,
		Object:    5,
		Port:      0,
		Interface: "127.0.0.1",
	})
	if err != nil {
		t.Fatalf("AddListener(port 0): %v", err)
	}
	if desc.Port != 0 {
		t.Errorf("AddListener(port 0) descriptor port = %d, want 0", desc.Port)
	}

	infos := cm.ListenerInfos()
	if len(infos) != 1 {
		t.Fatalf("ListenerInfos() count = %d, want 1", len(infos))
	}
	if infos[0].Port != 0 {
		t.Errorf("ListenerInfos()[0].Port = %d, want descriptor 0", infos[0].Port)
	}

	cm.mu.Lock()
	var boundAddr string
	for _, record := range cm.listeners {
		boundAddr = record.listener.Addr().String()
	}
	cm.mu.Unlock()
	_, boundPort, err := net.SplitHostPort(boundAddr)
	if err != nil || boundPort == "0" {
		t.Fatalf("runtime listener bound address = %q, want nonzero OS port", boundAddr)
	}

	if err := cm.RemoveListener(listenercfg.Descriptor{
		Protocol: listenercfg.ProtocolTCP,
		Port:     0,
	}); err != nil {
		t.Fatalf("RemoveListener(descriptor 0): %v", err)
	}
	if infos := cm.ListenerInfos(); len(infos) != 0 {
		t.Fatalf("ListenerInfos() after unlisten = %+v, want none", infos)
	}

	rebound, err := net.Listen("tcp4", boundAddr)
	if err != nil {
		t.Fatalf("bind released runtime socket %s: %v", boundAddr, err)
	}
	if err := rebound.Close(); err != nil {
		t.Fatalf("close rebound socket: %v", err)
	}
}

func TestRuntimeListenerZeroRejectsDuplicateDescriptorAndClosesDuplicate(t *testing.T) {
	cm := NewConnectionManager(0)
	first := &fakeListener{addr: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 41001}}
	duplicate := &fakeListener{addr: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 41002}}
	spec := listenercfg.Spec{
		Protocol: listenercfg.ProtocolTCP,
		Object:   5,
		Port:     0,
	}

	desc, err := cm.registerListener(first, spec, false, nil)
	if err != nil {
		t.Fatalf("register first descriptor 0: %v", err)
	}
	t.Cleanup(func() { _ = cm.RemoveListener(desc) })

	if _, err := cm.registerListener(duplicate, spec, false, nil); err == nil {
		t.Fatalf("register duplicate descriptor 0 succeeded")
	}
	if !duplicate.closed {
		t.Errorf("duplicate descriptor socket was not closed")
	}
	if first.closed {
		t.Errorf("duplicate registration closed the original socket")
	}
}

func TestRuntimeWebSocketListenerZeroLifecycle(t *testing.T) {
	cm := NewConnectionManager(0)
	t.Cleanup(cm.CloseListeners)
	spec := listenercfg.Spec{
		Protocol:  listenercfg.ProtocolWebSocket,
		Object:    5,
		Port:      0,
		Interface: "127.0.0.1",
		Path:      "/moo",
	}
	want := listenercfg.Descriptor{
		Protocol: listenercfg.ProtocolWebSocket,
		Port:     0,
		Path:     "/moo",
	}

	desc, err := cm.AddListener(spec)
	if err != nil {
		t.Fatalf("AddListener(WebSocket port 0): %v", err)
	}
	if desc != want {
		t.Errorf("AddListener(WebSocket port 0) descriptor = %+v, want %+v", desc, want)
	}

	infos := cm.ListenerInfos()
	if len(infos) != 1 {
		t.Fatalf("ListenerInfos() count = %d, want 1", len(infos))
	}
	if infos[0].Protocol != want.Protocol || infos[0].Port != want.Port || infos[0].Path != want.Path {
		t.Errorf("ListenerInfos()[0] descriptor = %s://:%d%s, want %s://:%d%s",
			infos[0].Protocol, infos[0].Port, infos[0].Path,
			want.Protocol, want.Port, want.Path)
	}

	boundAddr := runtimeListenerBoundAddress(t, cm, desc)
	waitForWebSocketListener(t, boundAddr, want.Path)

	duplicateDesc, err := cm.AddListener(spec)
	if err == nil {
		t.Errorf("AddListener(duplicate WebSocket descriptor 0) = %+v, want error", duplicateDesc)
		duplicateAddr := runtimeListenerBoundAddress(t, cm, duplicateDesc)
		waitForWebSocketListener(t, duplicateAddr, want.Path)
		if cleanupErr := cm.RemoveListener(duplicateDesc); cleanupErr != nil {
			t.Fatalf("remove unexpectedly accepted duplicate WebSocket listener: %v", cleanupErr)
		}
	}
	if infos := cm.ListenerInfos(); len(infos) != 1 {
		t.Errorf("ListenerInfos() count after duplicate = %d, want 1", len(infos))
	}

	if err := cm.RemoveListener(want); err != nil {
		t.Errorf("RemoveListener(WebSocket descriptor 0): %v", err)
		if cleanupErr := cm.RemoveListener(desc); cleanupErr != nil {
			t.Fatalf("remove actual WebSocket descriptor %+v after failed descriptor-0 removal: %v", desc, cleanupErr)
		}
	}
	if infos := cm.ListenerInfos(); len(infos) != 0 {
		t.Errorf("ListenerInfos() after WebSocket removal = %+v, want none", infos)
	}

	rebound, err := net.Listen("tcp4", boundAddr)
	if err != nil {
		t.Fatalf("bind released runtime WebSocket socket %s: %v", boundAddr, err)
	}
	if err := rebound.Close(); err != nil {
		t.Fatalf("close rebound WebSocket socket: %v", err)
	}
}

func runtimeListenerBoundAddress(t *testing.T, cm *ConnectionManager, desc listenercfg.Descriptor) string {
	t.Helper()
	cm.mu.Lock()
	record := cm.listeners[listenerKeyFromDescriptor(desc)]
	cm.mu.Unlock()
	if record == nil {
		t.Fatalf("listener record for descriptor %+v is missing", desc)
	}
	boundAddr := record.listener.Addr().String()
	_, boundPort, err := net.SplitHostPort(boundAddr)
	if err != nil || boundPort == "0" {
		t.Fatalf("listener bound address = %q, want nonzero OS port", boundAddr)
	}
	return boundAddr
}

func waitForWebSocketListener(t *testing.T, boundAddr, path string) {
	t.Helper()
	transport := &http.Transport{Proxy: nil}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 250 * time.Millisecond}
	deadline := time.Now().Add(2 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		response, err := client.Get("http://" + boundAddr + path)
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode != http.StatusUpgradeRequired {
				t.Fatalf("GET WebSocket listener status = %d, want %d", response.StatusCode, http.StatusUpgradeRequired)
			}
			return
		}
		lastErr = err
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("WebSocket listener %s%s did not become ready: %v", boundAddr, path, lastErr)
}

func TestBindListenersCreatesMultipleTCPListeners(t *testing.T) {
	cm := NewConnectionManager(0)
	err := cm.BindListeners([]listenercfg.Spec{
		{Protocol: listenercfg.ProtocolTCP, Port: 0, Interface: "127.0.0.1"},
		{Protocol: listenercfg.ProtocolTCP, Port: 0, Interface: "127.0.0.1"},
	})
	if err != nil {
		t.Fatalf("bind listeners: %v", err)
	}
	defer cm.CloseListeners()
	if got := cm.GetListenPort(); got <= 0 {
		t.Fatalf("GetListenPort() = %d for startup port 0, want bound OS port", got)
	}

	infos := cm.ListenerInfos()
	if len(infos) != 2 {
		t.Fatalf("got %d listeners, want 2", len(infos))
	}
	for _, info := range infos {
		if info.Protocol != listenercfg.ProtocolTCP {
			t.Fatalf("unexpected protocol in listener info: %+v", info)
		}
		if info.Port <= 0 {
			t.Fatalf("listener did not bind a port: %+v", info)
		}
	}
}

func TestRegisterListenerDoesNotAcceptUntilStartAccepting(t *testing.T) {
	cm := NewConnectionManager(7777)
	listener := &fakeListener{
		addr:     &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8888},
		acceptCh: make(chan struct{}, 1),
	}

	desc, err := cm.registerListener(listener, listenercfg.Spec{
		Protocol: listenercfg.ProtocolTCP,
		Object:   5,
	}, false, nil)
	if err != nil {
		t.Fatalf("register listener: %v", err)
	}
	defer func() { _ = cm.RemoveListener(desc) }()

	select {
	case <-listener.acceptCh:
		t.Fatalf("listener accepted before StartAccepting")
	default:
	}
	if got := listener.acceptCount(); got != 0 {
		t.Fatalf("accept count = %d before StartAccepting, want 0", got)
	}

	cm.StartAccepting()

	select {
	case <-listener.acceptCh:
	case <-time.After(time.Second):
		t.Fatalf("listener did not start accepting")
	}
}

func TestCloseListenersClosesPrimaryListeners(t *testing.T) {
	cm := NewConnectionManager(7777)
	primary := &fakeListener{addr: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8888}}
	secondary := &fakeListener{addr: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 9999}}

	if _, err := cm.registerListener(primary, listenercfg.Spec{
		Protocol: listenercfg.ProtocolTCP,
		Object:   5,
	}, true, nil); err != nil {
		t.Fatalf("register primary listener: %v", err)
	}
	if _, err := cm.registerListener(secondary, listenercfg.Spec{
		Protocol: listenercfg.ProtocolTCP,
		Object:   6,
	}, false, nil); err != nil {
		t.Fatalf("register secondary listener: %v", err)
	}

	cm.CloseListeners()

	if !primary.closed {
		t.Fatalf("primary listener was not closed")
	}
	if !secondary.closed {
		t.Fatalf("secondary listener was not closed")
	}
	if infos := cm.ListenerInfos(); len(infos) != 0 {
		t.Fatalf("listeners after CloseListeners = %+v, want none", infos)
	}
}

func TestCloseListenersWaitsForAcceptLoops(t *testing.T) {
	cm := NewConnectionManager(7777)
	listener := newBlockingListener()

	if _, err := cm.registerListener(listener, listenercfg.Spec{
		Protocol: listenercfg.ProtocolTCP,
		Object:   5,
	}, true, nil); err != nil {
		t.Fatalf("register listener: %v", err)
	}
	cm.StartAccepting()

	select {
	case <-listener.entered:
	case <-time.After(time.Second):
		t.Fatalf("listener did not start accepting")
	}

	done := make(chan struct{})
	go func() {
		cm.CloseListeners()
		close(done)
	}()

	select {
	case <-done:
		t.Fatalf("CloseListeners returned before accept loop exited")
	case <-time.After(20 * time.Millisecond):
	}
	if !listener.isClosed() {
		t.Fatalf("listener was not closed")
	}

	listener.releaseAccept()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("CloseListeners did not return after accept loop exited")
	}
}

func TestCloseConnectionsSendsShutdownBannerAndClosesTransports(t *testing.T) {
	cm := NewConnectionManager(7777)
	firstTransport := newRecordingTransport("first")
	secondTransport := newRecordingTransport("second")

	cm.NewConnectionFromTransport(firstTransport)
	cm.NewConnectionFromTransport(secondTransport)

	cm.CloseConnections("Maintenance")

	want := []string{"*** Shutting down: Maintenance ***"}
	if got := firstTransport.writtenLines(); len(got) != 1 || got[0] != want[0] {
		t.Fatalf("first lines = %+v, want %+v", got, want)
	}
	if got := secondTransport.writtenLines(); len(got) != 1 || got[0] != want[0] {
		t.Fatalf("second lines = %+v, want %+v", got, want)
	}
	if !firstTransport.isClosed() {
		t.Fatalf("first transport was not closed")
	}
	if !secondTransport.isClosed() {
		t.Fatalf("second transport was not closed")
	}
}

func TestAcceptedConnectionIsPublishedWithListenerMetadata(t *testing.T) {
	cm := NewConnectionManager(7777)
	record := &listenerRecord{
		object:         5,
		descriptorPort: 0,
		printMessages:  true,
	}

	conn := cm.newConnectionFromTransport(newRecordingTransport("accepted"), record)
	published := cm.GetConnection(types.ObjID(-conn.ID))
	if published != conn {
		t.Fatalf("published connection = %v, want %v", published, conn)
	}
	port, set := published.ListenerPort()
	if !set || port != 0 {
		t.Fatalf("published listener port = (%d, %t), want (0, true)", port, set)
	}
	if got := conn.ListenerObject(); got != record.object {
		t.Fatalf("published listener object = %d, want %d", got, record.object)
	}
	if !conn.PrintMessages() {
		t.Fatal("published connection lost print-messages metadata")
	}
}

// Toast's SIGALRM runs the head of its sorted real-time timer list: a connect
// timer only when it is due strictly before the checkpoint timer.
func TestExpireEarliestOutboundConnectFiresOnlyTimerBeforeCheckpoint(t *testing.T) {
	cm := NewConnectionManager(7777)
	now := time.Now()
	var canceled []string
	add := func(name string, deadline time.Time) {
		cm.pendingDials[&pendingDial{deadline: deadline, cancel: func() { canceled = append(canceled, name) }}] = struct{}{}
	}
	add("late", now.Add(20*time.Second))
	add("early", now.Add(10*time.Second))

	if cm.ExpireEarliestOutboundConnect(now.Add(10 * time.Second)) {
		t.Fatal("connect due with the checkpoint timer fired before it")
	}
	if !cm.ExpireEarliestOutboundConnect(now.Add(time.Hour)) {
		t.Fatal("connect due before the checkpoint timer did not fire")
	}
	if !cm.ExpireEarliestOutboundConnect(time.Time{}) {
		t.Fatal("connect did not fire with no checkpoint timer armed")
	}
	if cm.ExpireEarliestOutboundConnect(time.Time{}) {
		t.Fatal("fired with no pending connect")
	}
	if len(canceled) != 2 || canceled[0] != "early" || canceled[1] != "late" {
		t.Fatalf("canceled = %v, want [early late]", canceled)
	}
}

// Toast network.cc open_connection resolves the host with AF_INET, or AF_INET6
// with the ipv6 option, and fails in getaddrinfo when the host has no address
// in that family.
func TestOpenNetworkConnectionResolvesOneAddressFamily(t *testing.T) {
	cm := NewConnectionManager(7777)
	for _, tc := range []struct {
		host string
		ipv6 bool
	}{
		{"::1", false},
		{"127.0.0.1", true},
	} {
		_, err := cm.OpenNetworkConnection(builtins.OutboundConnection{Host: tc.host, Port: 1, Timeout: time.Second, IPv6: tc.ipv6})
		var lookupErr *builtins.HostLookupError
		if !errors.As(err, &lookupErr) {
			t.Errorf("OpenNetworkConnection(%q, ipv6=%v) error = %v, want a lookup error", tc.host, tc.ipv6, err)
		}
	}
}

// outboundPeer is a loopback server that is not a ConnectionManager listener:
// the "other host" an outbound connection reaches.
type outboundPeer struct {
	addr     *net.TCPAddr
	accepted chan net.Conn
}

func startOutboundPeer(t *testing.T, wrap func(net.Listener) net.Listener) *outboundPeer {
	t.Helper()
	socket, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	if wrap != nil {
		socket = wrap(socket)
	}
	t.Cleanup(func() { _ = socket.Close() })
	peer := &outboundPeer{addr: socket.Addr().(*net.TCPAddr), accepted: make(chan net.Conn, 1)}
	go func() {
		conn, err := socket.Accept()
		if err != nil {
			return
		}
		if tlsConn, ok := conn.(*tls.Conn); ok {
			// A TLS server conn handshakes on first use; drive it now so the
			// client's handshake can finish (or fail) without waiting for I/O.
			_ = tlsConn.Handshake()
		}
		peer.accepted <- conn
	}()
	return peer
}

func (p *outboundPeer) accept(t *testing.T) net.Conn {
	t.Helper()
	select {
	case conn := <-p.accepted:
		t.Cleanup(func() { _ = conn.Close() })
		return conn
	case <-time.After(5 * time.Second):
		t.Fatal("peer accepted nothing")
		return nil
	}
}

// managerWithHandler records each connection the manager hands to its
// connection handler and reads its first line.
func managerWithHandler(t *testing.T) (*ConnectionManager, chan *Connection, chan string) {
	t.Helper()
	cm := NewConnectionManager(7777)
	handled := make(chan *Connection, 1)
	lines := make(chan string, 1)
	cm.setConnectionHandler(func(conn *Connection) {
		handled <- conn
		line, err := conn.ReadLine()
		if err == nil {
			lines <- line
		}
		<-conn.ctx.Done()
	})
	t.Cleanup(func() {
		cm.mu.Lock()
		conns := make([]*Connection, 0, len(cm.connections))
		for _, conn := range cm.connections {
			conns = append(conns, conn)
		}
		cm.mu.Unlock()
		for _, conn := range conns {
			_ = conn.Close()
		}
		cm.connectionWG.Wait()
	})
	return cm, handled, lines
}

func handledConnection(t *testing.T, handled chan *Connection) *Connection {
	t.Helper()
	select {
	case conn := <-handled:
		return conn
	case <-time.After(5 * time.Second):
		t.Fatal("outbound connection never reached the connection handler")
		return nil
	}
}

// Toast network_open_connection makes the dialled socket its own connection,
// with listener #0, no initial login line and no listen address; a connection
// to any host works, not only to this server's own listeners.
func TestOpenNetworkConnectionMakesTheDialledSocketItsOwnConnection(t *testing.T) {
	peer := startOutboundPeer(t, nil)
	cm, handled, lines := managerWithHandler(t)

	handle, err := cm.OpenNetworkConnection(builtins.OutboundConnection{Host: "127.0.0.1", Port: int64(peer.addr.Port), Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("OpenNetworkConnection: %v", err)
	}
	remote := peer.accept(t)
	conn := handledConnection(t, handled)
	if handle != types.ObjID(-conn.ID) {
		t.Fatalf("handle = %d, want the dialled connection %d", handle, -conn.ID)
	}
	if !conn.IsOutbound() || !conn.noInitialInput || conn.ListenerObject() != 0 {
		t.Fatalf("outbound=%v noInitialInput=%v listener=%d, want outbound, no initial input, #0", conn.IsOutbound(), conn.noInitialInput, conn.ListenerObject())
	}
	if conn.RemoteAddr() != peer.addr.String() {
		t.Fatalf("remote address = %q, want peer %q", conn.RemoteAddr(), peer.addr.String())
	}
	if got := cm.GetConnection(handle); got != conn {
		t.Fatalf("GetConnection(%d) = %v, want the dialled connection", handle, got)
	}

	if err := conn.Send("to the peer"); err != nil {
		t.Fatalf("send: %v", err)
	}
	_ = remote.SetReadDeadline(time.Now().Add(5 * time.Second))
	got, err := bufio.NewReader(remote).ReadString('\n')
	if err != nil || strings.TrimRight(got, "\r\n") != "to the peer" {
		t.Fatalf("peer read %q, %v; want the sent line", got, err)
	}
	if _, err := remote.Write([]byte("from the peer\r\n")); err != nil {
		t.Fatalf("peer write: %v", err)
	}
	select {
	case line := <-lines:
		if line != "from the peer" {
			t.Fatalf("connection read %q, want %q", line, "from the peer")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("connection read nothing from the peer")
	}
}

// Toast queues no initial login line for an outbound connection without a
// listener object, and a line from the remote end goes to #0's
// do_login_command (server.cc server_new_connection, server_receive_line).
// Nor does such a connection time out waiting for login (main loop skips
// h->outbound): a one-second connect_timeout does not close it.
func TestOutboundConnectionWithoutListenerGetsNoInitialLineAndNoTimeout(t *testing.T) {
	store := dbstore.NewStore()
	system := addTestObject(t, store, 0, dbstore.FlagWizard)
	addTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagProgrammer|dbstore.FlagWizard)
	options := addTestObject(t, store, 3, 0)
	txn := store.DirectTxn()
	if code := txn.DefineProperty(system, "got", dbstore.NewProperty(types.NewList(nil), 2, dbstore.PropRead|dbstore.PropWrite, false, true)); code != types.E_NONE {
		t.Fatal(code)
	}
	if code := txn.DefineProperty(system, "server_options", dbstore.NewProperty(types.NewObj(options), 2, dbstore.PropRead, false, true)); code != types.E_NONE {
		t.Fatal(code)
	}
	if code := txn.DefineProperty(options, "connect_timeout", dbstore.NewProperty(types.NewInt(1), 2, dbstore.PropRead, false, true)); code != types.E_NONE {
		t.Fatal(code)
	}
	addTestVerb(store, system, "do_login_command", "#0.got = {@#0.got, argstr};", "return 0;")

	rt := engine.NewRuntime(store)
	input := NewInputProcessor(store, rt)
	cm := NewConnectionManager(0)
	input.SetConnectionManager(cm)
	input.Start()
	defer input.Stop()
	defer rt.Stop()

	peer := startOutboundPeer(t, nil)
	handle, err := cm.OpenNetworkConnection(builtins.OutboundConnection{Host: "127.0.0.1", Port: int64(peer.addr.Port), Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("OpenNetworkConnection: %v", err)
	}
	remote := peer.accept(t)
	defer func() {
		if conn, ok := cm.GetConnection(handle).(*Connection); ok {
			_ = conn.Close()
		}
	}()

	got := func() types.Value {
		value, code := store.DirectTxn().PropertyValue(system, "got")
		if code != types.E_NONE {
			t.Fatalf("read #0.got: %s", code)
		}
		return value
	}
	time.Sleep(2500 * time.Millisecond)
	if value := got(); value.Len() != 0 {
		t.Fatalf("do_login_command calls before any input = %s, want none", value.String())
	}
	if cm.GetConnection(handle) == nil {
		t.Fatal("outbound connection was closed by connect_timeout")
	}

	if _, err := remote.Write([]byte("hello from the peer\r\n")); err != nil {
		t.Fatalf("peer write: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if value := got(); value.Len() == 1 {
			if value.Get(1).Str() != "hello from the peer" {
				t.Fatalf("do_login_command argstr = %s, want the peer's line", value.String())
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("do_login_command calls = %s, want the peer's line", got().String())
}

// With the listener option the connection's listener is that object and it
// gets the initial empty login line (Toast server_new_connection); its print
// messages come from a listen()ing listener of that object, else off.
func TestOpenNetworkConnectionListenerOption(t *testing.T) {
	peer := startOutboundPeer(t, nil)
	cm, handled, _ := managerWithHandler(t)

	_, err := cm.OpenNetworkConnection(builtins.OutboundConnection{Host: "127.0.0.1", Port: int64(peer.addr.Port), Timeout: 5 * time.Second, HasListener: true, Listener: 42})
	if err != nil {
		t.Fatalf("OpenNetworkConnection: %v", err)
	}
	peer.accept(t)
	conn := handledConnection(t, handled)
	if conn.ListenerObject() != 42 || conn.noInitialInput || conn.PrintMessages() {
		t.Fatalf("listener=%d noInitialInput=%v printMessages=%v, want #42 with initial input and no messages", conn.ListenerObject(), conn.noInitialInput, conn.PrintMessages())
	}
}

func tlsPeer(t *testing.T, cert tls.Certificate) *outboundPeer {
	return startOutboundPeer(t, func(socket net.Listener) net.Listener {
		return tls.NewListener(socket, &tls.Config{Certificates: []tls.Certificate{cert}})
	})
}

func trustOnly(cert testCertificate) *x509.CertPool {
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(cert.CertificatePEM)
	return pool
}

// The tls option makes a TLS client of the outbound connection; lines then
// travel inside TLS.
func TestOpenNetworkConnectionTLSHandshakeAndLine(t *testing.T) {
	cert := selfSignedCertificate(t)
	peer := tlsPeer(t, cert.Certificate)
	cm, handled, _ := managerWithHandler(t)
	cm.outboundTLSRoots = trustOnly(cert)

	if _, err := cm.OpenNetworkConnection(builtins.OutboundConnection{Host: "127.0.0.1", Port: int64(peer.addr.Port), Timeout: 5 * time.Second, TLS: true}); err != nil {
		t.Fatalf("OpenNetworkConnection with TLS: %v", err)
	}
	remote := peer.accept(t)
	conn := handledConnection(t, handled)
	if err := conn.Send("over tls"); err != nil {
		t.Fatalf("send: %v", err)
	}
	_ = remote.SetReadDeadline(time.Now().Add(5 * time.Second))
	got, err := bufio.NewReader(remote).ReadString('\n')
	if err != nil || strings.TrimRight(got, "\r\n") != "over tls" {
		t.Fatalf("TLS peer read %q, %v; want the sent line", got, err)
	}
}

// Barn verifies the chain and the host name (Go's default); Toast verifies
// the chain only. An untrusted chain or a certificate for another name fails
// with *builtins.TLSHandshakeError and opens no connection.
func TestOpenNetworkConnectionTLSVerifiesChainAndHostName(t *testing.T) {
	cert := selfSignedCertificate(t)
	other := certificateForNames(t, "other.example")
	for _, tc := range []struct {
		name  string
		serve tls.Certificate
		roots *x509.CertPool
	}{
		{"untrusted chain", cert.Certificate, nil},
		{"wrong host name", other.Certificate, trustOnly(other)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			peer := tlsPeer(t, tc.serve)
			cm, _, _ := managerWithHandler(t)
			cm.outboundTLSRoots = tc.roots
			_, err := cm.OpenNetworkConnection(builtins.OutboundConnection{Host: "127.0.0.1", Port: int64(peer.addr.Port), Timeout: 5 * time.Second, TLS: true})
			var handshakeErr *builtins.TLSHandshakeError
			if !errors.As(err, &handshakeErr) {
				t.Fatalf("error = %v, want a TLS handshake error", err)
			}
			cm.mu.Lock()
			count := len(cm.connections)
			cm.mu.Unlock()
			if count != 0 {
				t.Fatalf("%d connections registered after a failed handshake", count)
			}
		})
	}
}

func TestCloseConnectionsClosesAndWaitsForAcceptedSetup(t *testing.T) {
	cert := selfSignedCertificate(t)
	setupConn := newSetupBlockingConn()
	defer setupConn.release()
	listener := newSingleConnListener(setupConn)
	cm := NewConnectionManager(7777)
	cm.connectTimeout = time.Hour

	if _, err := cm.registerListener(listener, listenercfg.Spec{
		Protocol: listenercfg.ProtocolTLS,
		Object:   5,
	}, true, &tls.Config{Certificates: []tls.Certificate{cert.Certificate}}); err != nil {
		t.Fatalf("register listener: %v", err)
	}
	cm.StartAccepting()
	select {
	case <-setupConn.readStarted:
	case <-time.After(time.Second):
		t.Fatalf("accepted connection did not enter TLS setup")
	}

	cm.CloseListeners()
	done := make(chan struct{})
	go func() {
		cm.CloseConnections("Maintenance")
		close(done)
	}()

	select {
	case <-done:
		t.Fatalf("CloseConnections returned before setup goroutine exited")
	case <-time.After(20 * time.Millisecond):
	}
	if !setupConn.isClosed() {
		t.Fatalf("setup connection was not closed")
	}

	setupConn.release()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("CloseConnections did not return after setup goroutine exited")
	}
}

func TestCloseConnectionsWaitsForWebSocketSetup(t *testing.T) {
	cm := NewConnectionManager(7777)
	record := &listenerRecord{
		protocol: listenercfg.ProtocolWebSocket,
		path:     "/",
	}
	response := newBlockingHijackResponseWriter()
	defer response.releaseHijack()
	request := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	request.Header.Set("Connection", "Upgrade")
	request.Header.Set("Upgrade", "websocket")
	request.Header.Set("Sec-WebSocket-Version", "13")
	request.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")

	handlerDone := make(chan struct{})
	go func() {
		cm.handleWebSocketRequest(record, response, request)
		close(handlerDone)
	}()

	select {
	case <-response.entered:
	case <-time.After(time.Second):
		t.Fatalf("WebSocket setup did not enter hijack")
	}

	done := make(chan struct{})
	go func() {
		cm.CloseConnections("Maintenance")
		close(done)
	}()

	select {
	case <-done:
		t.Fatalf("CloseConnections returned before WebSocket setup exited")
	case <-time.After(20 * time.Millisecond):
	}

	response.releaseHijack()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("CloseConnections did not return after WebSocket setup exited")
	}
	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		t.Fatalf("WebSocket handler did not return after hijack release")
	}
}
