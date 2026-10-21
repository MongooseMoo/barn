package app

import (
	"bytes"
	"context"
	"log/slog"
	"net"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunReservesGamePortBeforeHTTPEndpoints(t *testing.T) {
	for _, endpoint := range []string{"debug", "operator"} {
		t.Run(endpoint, func(t *testing.T) {
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			addr := ln.Addr().String()
			if err := ln.Close(); err != nil {
				t.Fatal(err)
			}
			var logs bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
			defer slog.SetDefault(previous)
			cfg := DefaultConfig()
			cfg.DatabasePath = filepath.Join(t.TempDir(), "missing.db")
			cfg.Listen = []string{"tcp://" + addr}
			cfg.DebugAddr, cfg.OperatorAddr = "off", "off"
			if endpoint == "debug" {
				cfg.DebugAddr = addr
			} else {
				cfg.OperatorAddr = addr
			}
			// Force the exact allocation collision rather than waiting for the
			// kernel's ephemeral HTTP port choice to hit the released game port.
			err = Run(context.Background(), cfg, &bytes.Buffer{}, &bytes.Buffer{})
			if err == nil || !strings.Contains(err.Error(), "load database") {
				t.Fatalf("Run = %v, want database load failure", err)
			}
			if !strings.Contains(logs.String(), endpoint+" endpoint unavailable") {
				t.Fatalf("HTTP endpoint took the game port before database loading: %s", &logs)
			}
			// A failure before Start must release the reserved socket, too.
			ln, err = net.Listen("tcp", addr)
			if err != nil {
				t.Fatalf("game reservation leaked after load failure: %v", err)
			}
			ln.Close()
		})
	}
}
