package server

import (
	"barn/builtins"
	"barn/db"
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
)

func TestAddTLSListenerReportsMetadata(t *testing.T) {
	certPath, keyPath := writeSelfSignedCertificate(t)
	cm := NewConnectionManager(nil, 0)

	desc, err := cm.AddListener(builtins.ListenerSpec{
		Protocol:           "tls",
		Port:               0,
		Interface:          "127.0.0.1",
		TLSCertificatePath: certPath,
		TLSKeyPath:         keyPath,
	})
	if err != nil {
		t.Fatalf("add tls listener: %v", err)
	}
	defer func() { _ = cm.RemoveListener(desc) }()

	if desc.Protocol != "tls" || desc.Port <= 0 {
		t.Fatalf("unexpected descriptor: %+v", desc)
	}

	infos := cm.ListenerInfos()
	if len(infos) != 1 {
		t.Fatalf("got %d listener infos, want 1", len(infos))
	}
	info := infos[0]
	if info.Protocol != "tls" ||
		info.Port != desc.Port ||
		!info.TLS ||
		info.Interface != "127.0.0.1" {
		t.Fatalf("unexpected listener info: %+v", info)
	}
}

func TestAddTLSListenerRequiresCertificateAndKey(t *testing.T) {
	cm := NewConnectionManager(nil, 0)
	_, err := cm.AddListener(builtins.ListenerSpec{
		Protocol:  "tls",
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
	store := db.NewStore()
	system := addTestObject(t, store, 0, db.FlagWizard)
	addTestObject(t, store, 2, db.FlagUser|db.FlagProgrammer|db.FlagWizard)
	addTestVerb(system, "do_login_command",
		`if (length(args) > 0 && args[1] == "connect")`,
		"  return #2;",
		"else",
		"  return #-1;",
		"endif",
	)

	scheduler := NewScheduler(store)
	srv := &Server{scheduler: scheduler}
	cm := NewConnectionManager(srv, 0)
	scheduler.SetConnectionManager(cm)
	scheduler.Start()
	defer scheduler.Stop()

	err := cm.StartListeners([]builtins.ListenerSpec{{
		Protocol:           "tls",
		Port:               0,
		Interface:          "127.0.0.1",
		TLSCertificatePath: certPath,
		TLSKeyPath:         keyPath,
	}})
	if err != nil {
		t.Fatalf("start tls listener: %v", err)
	}
	defer closeAllListeners(cm)

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
