package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/internal/listener"
)

func TestHTTPEndpointHeaderTimeoutPreservesLongResponse(t *testing.T) {
	started, respond := make(chan struct{}), make(chan struct{})
	release := sync.OnceFunc(func() { close(respond) })
	var logs bytes.Buffer
	endpoint, err := func() (*httpEndpoint, error) {
		previous := slog.Default()
		slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
		defer slog.SetDefault(previous)
		return startHTTPEndpoint("header-test", "127.0.0.1:0", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			close(started)
			<-respond
			fmt.Fprint(w, "profile complete")
		}))
	}()
	if err != nil {
		t.Fatal(err)
	}
	defer endpoint.shutdown()
	defer release()
	var bound struct{ Addr string }
	if err := json.Unmarshal(logs.Bytes(), &bound); err != nil || bound.Addr == "" {
		t.Fatalf("endpoint address: %q, %v", &logs, err)
	}
	transport := &http.Transport{Proxy: nil}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: listener.HTTPReadHeaderTimeout + 5*time.Second}
	result := make(chan error, 1)
	go func() {
		response, err := client.Get("http://" + bound.Addr + "/profile")
		if err == nil {
			var body []byte
			body, err = io.ReadAll(response.Body)
			_ = response.Body.Close()
			if err == nil && (response.StatusCode != http.StatusOK || string(body) != "profile complete") {
				err = fmt.Errorf("response = %d %q", response.StatusCode, body)
			}
		}
		result <- err
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("complete request did not reach handler")
	}
	peer, err := net.DialTimeout("tcp", bound.Addr, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	if err := peer.SetDeadline(time.Now().Add(listener.HTTPReadHeaderTimeout + 3*time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(peer, "GET / HTTP/1.1\r\nHost:"); err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(io.Discard, peer); err != nil {
		t.Fatalf("incomplete headers retained connection: %v", err)
	}
	// The complete request has now outlived the header allowance. Finishing its
	// response must still work, as it does for a long diagnostic profile.
	release()
	if err := <-result; err != nil {
		t.Fatalf("long response: %v", err)
	}
}
