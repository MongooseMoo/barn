package server

import (
	"barn/builtins"
	"net"
	"testing"
)

type fakeListener struct {
	addr   net.Addr
	closed bool
}

func (l *fakeListener) Accept() (net.Conn, error) {
	return nil, net.ErrClosed
}

func (l *fakeListener) Close() error {
	l.closed = true
	return nil
}

func (l *fakeListener) Addr() net.Addr {
	return l.addr
}

func TestListenerDescriptorsUseProtocolPathKey(t *testing.T) {
	cm := NewConnectionManager(nil, 7777)
	tcp := &fakeListener{addr: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8888}}

	desc, err := cm.registerListener(tcp, builtins.ListenerSpec{
		Protocol: builtins.ListenerProtocolTCP,
		Object:   5,
	}, false)
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
	cm := NewConnectionManager(nil, 7777)
	first := &fakeListener{addr: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8888}}
	second := &fakeListener{addr: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8888}}

	_, err := cm.registerListener(first, builtins.ListenerSpec{
		Protocol: builtins.ListenerProtocolTCP,
		Object:   5,
	}, false)
	if err != nil {
		t.Fatalf("register first listener: %v", err)
	}

	_, err = cm.registerListener(second, builtins.ListenerSpec{
		Protocol: builtins.ListenerProtocolTCP,
		Object:   6,
	}, false)
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

func TestStartListenersCreatesMultipleTCPListeners(t *testing.T) {
	cm := NewConnectionManager(nil, 0)
	err := cm.StartListeners([]builtins.ListenerSpec{
		{Protocol: builtins.ListenerProtocolTCP, Port: 0, Interface: "127.0.0.1"},
		{Protocol: builtins.ListenerProtocolTCP, Port: 0, Interface: "127.0.0.1"},
	})
	if err != nil {
		t.Fatalf("start listeners: %v", err)
	}
	defer closeAllListeners(cm)

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

func closeAllListeners(cm *ConnectionManager) {
	cm.mu.Lock()
	records := make([]*listenerRecord, 0, len(cm.listeners))
	for _, record := range cm.listeners {
		records = append(records, record)
	}
	cm.mu.Unlock()
	for _, record := range records {
		_ = record.listener.Close()
	}
}
