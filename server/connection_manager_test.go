package server

import (
	"barn/builtins"
	"net"
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
		Protocol: "ws",
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
