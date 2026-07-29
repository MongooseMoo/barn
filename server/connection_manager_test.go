package server

import (
	"barn/builtins"
	"barn/types"
	"bufio"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
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

func (t *recordingTransport) WriteLine(line string) error {
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

	desc, err := cm.registerListener(tcp, builtins.ListenerSpec{
		Protocol: builtins.ListenerProtocolTCP,
		Object:   5,
	}, false, nil)
	if err != nil {
		t.Fatalf("register tcp listener: %v", err)
	}

	err = cm.RemoveListener(builtins.ListenerDescriptor{
		Protocol: builtins.ListenerProtocolWebSocket,
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

	desc, err := cm.registerListener(tcp, builtins.ListenerSpec{
		Protocol: builtins.ListenerProtocolTCP,
		Object:   5,
	}, false, nil)
	if err != nil {
		t.Fatalf("register tcp listener: %v", err)
	}

	err = cm.RemoveListener(builtins.ListenerDescriptor{
		Protocol: builtins.ListenerProtocolTCP,
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

	_, err := cm.registerListener(first, builtins.ListenerSpec{
		Protocol: builtins.ListenerProtocolTCP,
		Object:   5,
	}, false, nil)
	if err != nil {
		t.Fatalf("register first listener: %v", err)
	}

	_, err = cm.registerListener(second, builtins.ListenerSpec{
		Protocol: builtins.ListenerProtocolTCP,
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

func TestBindListenersCreatesMultipleTCPListeners(t *testing.T) {
	cm := NewConnectionManager(0)
	err := cm.BindListeners([]builtins.ListenerSpec{
		{Protocol: builtins.ListenerProtocolTCP, Port: 0, Interface: "127.0.0.1"},
		{Protocol: builtins.ListenerProtocolTCP, Port: 0, Interface: "127.0.0.1"},
	})
	if err != nil {
		t.Fatalf("bind listeners: %v", err)
	}
	defer cm.CloseListeners()

	infos := cm.ListenerInfos()
	if len(infos) != 2 {
		t.Fatalf("got %d listeners, want 2", len(infos))
	}
	for _, info := range infos {
		if info.Protocol != builtins.ListenerProtocolTCP {
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

	desc, err := cm.registerListener(listener, builtins.ListenerSpec{
		Protocol: builtins.ListenerProtocolTCP,
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

	if _, err := cm.registerListener(primary, builtins.ListenerSpec{
		Protocol: builtins.ListenerProtocolTCP,
		Object:   5,
	}, true, nil); err != nil {
		t.Fatalf("register primary listener: %v", err)
	}
	if _, err := cm.registerListener(secondary, builtins.ListenerSpec{
		Protocol: builtins.ListenerProtocolTCP,
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

	if _, err := cm.registerListener(listener, builtins.ListenerSpec{
		Protocol: builtins.ListenerProtocolTCP,
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

func TestCloseConnectionsClosesAndWaitsForAcceptedSetup(t *testing.T) {
	cert := selfSignedCertificate(t)
	setupConn := newSetupBlockingConn()
	defer setupConn.release()
	listener := newSingleConnListener(setupConn)
	cm := NewConnectionManager(7777)
	cm.connectTimeout = time.Hour

	if _, err := cm.registerListener(listener, builtins.ListenerSpec{
		Protocol: builtins.ListenerProtocolTLS,
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
		protocol: builtins.ListenerProtocolWebSocket,
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
