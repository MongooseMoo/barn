package server

import (
	"bufio"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"barn/builtins"
	dbstore "barn/db/store"
	runtime "barn/scheduler"
)

func TestAddTLSListenerReportsMetadata(t *testing.T) {
	certPath, keyPath := writeSelfSignedCertificate(t)
	cm := NewConnectionManager(0)

	desc, err := cm.AddListener(builtins.ListenerSpec{
		Protocol:           builtins.ListenerProtocolTLS,
		Port:               0,
		Interface:          "127.0.0.1",
		TLSCertificatePath: certPath,
		TLSKeyPath:         keyPath,
	})
	if err != nil {
		t.Fatalf("add tls listener: %v", err)
	}
	defer func() { _ = cm.RemoveListener(desc) }()

	if desc.Protocol != builtins.ListenerProtocolTLS || desc.Port != 0 {
		t.Fatalf("unexpected descriptor: %+v", desc)
	}

	infos := cm.ListenerInfos()
	if len(infos) != 1 {
		t.Fatalf("got %d listener infos, want 1", len(infos))
	}
	info := infos[0]
	if info.Protocol != builtins.ListenerProtocolTLS ||
		info.Port != 0 ||
		!info.TLS ||
		info.Interface != "127.0.0.1" {
		t.Fatalf("unexpected listener info: %+v", info)
	}
	cm.mu.Lock()
	boundPort := cm.listeners[listenerKeyFromDescriptor(desc)].boundPort
	cm.mu.Unlock()
	if boundPort <= 0 {
		t.Fatalf("runtime TLS bound port = %d, want nonzero OS port", boundPort)
	}
}

func TestRuntimeTLSListenerZeroLifecycle(t *testing.T) {
	certPath, keyPath := writeSelfSignedCertificate(t)
	cm := NewConnectionManager(0)
	t.Cleanup(cm.CloseListeners)
	spec := builtins.ListenerSpec{
		Protocol:           builtins.ListenerProtocolTLS,
		Port:               0,
		Interface:          "127.0.0.1",
		TLSCertificatePath: certPath,
		TLSKeyPath:         keyPath,
	}
	want := builtins.ListenerDescriptor{
		Protocol: builtins.ListenerProtocolTLS,
		Port:     0,
	}

	desc, err := cm.AddListener(spec)
	if err != nil {
		t.Fatalf("AddListener(TLS port 0): %v", err)
	}
	if desc != want {
		t.Errorf("AddListener(TLS port 0) descriptor = %+v, want %+v", desc, want)
	}

	infos := cm.ListenerInfos()
	if len(infos) != 1 {
		t.Fatalf("ListenerInfos() count = %d, want 1", len(infos))
	}
	if infos[0].Protocol != want.Protocol || infos[0].Port != want.Port {
		t.Errorf("ListenerInfos()[0] descriptor = %s://:%d, want %s://:%d",
			infos[0].Protocol, infos[0].Port, want.Protocol, want.Port)
	}

	boundAddr := runtimeListenerBoundAddress(t, cm, desc)

	duplicateDesc, err := cm.AddListener(spec)
	if err == nil {
		t.Errorf("AddListener(duplicate TLS descriptor 0) = %+v, want error", duplicateDesc)
		if cleanupErr := cm.RemoveListener(duplicateDesc); cleanupErr != nil {
			t.Fatalf("remove unexpectedly accepted duplicate TLS listener: %v", cleanupErr)
		}
	}
	if infos := cm.ListenerInfos(); len(infos) != 1 {
		t.Errorf("ListenerInfos() count after TLS duplicate = %d, want 1", len(infos))
	}

	if err := cm.RemoveListener(want); err != nil {
		t.Errorf("RemoveListener(TLS descriptor 0): %v", err)
		if cleanupErr := cm.RemoveListener(desc); cleanupErr != nil {
			t.Fatalf("remove actual TLS descriptor %+v after failed descriptor-0 removal: %v", desc, cleanupErr)
		}
	}
	if infos := cm.ListenerInfos(); len(infos) != 0 {
		t.Errorf("ListenerInfos() after TLS removal = %+v, want none", infos)
	}

	rebound, err := net.Listen("tcp4", boundAddr)
	if err != nil {
		t.Fatalf("bind released runtime TLS socket %s: %v", boundAddr, err)
	}
	if err := rebound.Close(); err != nil {
		t.Fatalf("close rebound TLS socket: %v", err)
	}
}

func TestAddTLSListenerRequiresCertificateAndKey(t *testing.T) {
	cm := NewConnectionManager(0)
	_, err := cm.AddListener(builtins.ListenerSpec{
		Protocol:  builtins.ListenerProtocolTLS,
		Port:      0,
		Interface: "127.0.0.1",
	})
	if err == nil {
		t.Fatalf("added tls listener without certificate and key")
	}
}

func TestTLSLineTransportReadsAfterHandshake(t *testing.T) {
	cert := selfSignedCertificate(t)
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	lineCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		tlsConn := tls.Server(serverConn, &tls.Config{Certificates: []tls.Certificate{cert.Certificate}})
		if err := tlsConn.Handshake(); err != nil {
			errCh <- err
			return
		}
		transport := NewTCPTransport(tlsConn)
		line, err := transport.ReadLine()
		if err != nil {
			errCh <- err
			return
		}
		lineCh <- line
	}()

	client := tls.Client(clientConn, &tls.Config{InsecureSkipVerify: true})
	if err := client.Handshake(); err != nil {
		t.Fatalf("client handshake: %v", err)
	}
	if _, err := client.Write([]byte("hello tls\r\n")); err != nil {
		t.Fatalf("client write: %v", err)
	}

	select {
	case line := <-lineCh:
		if line != "hello tls" {
			t.Fatalf("got line %q, want %q", line, "hello tls")
		}
	case err := <-errCh:
		t.Fatalf("server error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for TLS line")
	}
}

func TestTLSListenerLoginAndEval(t *testing.T) {
	certPath, keyPath := writeSelfSignedCertificate(t)
	store := dbstore.NewStore()
	system := addTestObject(t, store, 0, dbstore.FlagWizard)
	addTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagProgrammer|dbstore.FlagWizard)
	addTestVerb(store, system, "do_login_command",
		`if (length(args) > 0 && args[1] == "connect")`,
		"  return #2;",
		"else",
		"  return #-1;",
		"endif",
	)

	scheduler := runtime.NewScheduler(store)
	input := NewInputProcessor(store, scheduler)
	cm := NewConnectionManager(0)
	input.SetConnectionManager(cm)
	input.Start()
	defer input.Stop()
	defer scheduler.Stop()

	err := cm.BindListeners([]builtins.ListenerSpec{{
		Protocol:           builtins.ListenerProtocolTLS,
		Port:               0,
		Interface:          "127.0.0.1",
		TLSCertificatePath: certPath,
		TLSKeyPath:         keyPath,
	}})
	if err != nil {
		t.Fatalf("bind tls listener: %v", err)
	}
	defer cm.CloseListeners()
	cm.StartAccepting()

	port := cm.ListenerInfos()[0].Port
	client, err := tls.Dial("tcp", net.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", port)), &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		t.Fatalf("tls dial: %v", err)
	}
	defer client.Close()
	if err := client.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}
	reader := bufio.NewReader(client)

	if _, err := client.Write([]byte("connect test\r\n")); err != nil {
		t.Fatalf("write login: %v", err)
	}
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read login response: %v", err)
	}
	if strings.TrimSpace(line) != "*** Connected ***" {
		t.Fatalf("login response %q, want connected message", line)
	}

	if _, err := client.Write([]byte("eval return 3;\r\n")); err != nil {
		t.Fatalf("write eval: %v", err)
	}
	line, err = reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read eval response: %v", err)
	}
	if strings.TrimSpace(line) != "{1, 3}" {
		t.Fatalf("eval response %q, want {1, 3}", line)
	}
}

func writeSelfSignedCertificate(t *testing.T) (string, string) {
	t.Helper()
	cert := selfSignedCertificate(t)
	dir := t.TempDir()
	certPath := filepath.Join(dir, "server.crt")
	keyPath := filepath.Join(dir, "server.key")

	if err := os.WriteFile(certPath, cert.CertificatePEM, 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, cert.PrivateKeyPEM, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	return certPath, keyPath
}

type testCertificate struct {
	tls.Certificate
	CertificatePEM []byte
	PrivateKeyPEM  []byte
}

func selfSignedCertificate(t *testing.T) testCertificate {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate private key: %v", err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("parse key pair: %v", err)
	}
	return testCertificate{
		Certificate:    cert,
		CertificatePEM: certPEM,
		PrivateKeyPEM:  keyPEM,
	}
}
