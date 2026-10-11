package wasmbuiltins

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/types"
)

func fixtureManager(t *testing.T) (*Manager, string) {
	t.Helper()
	dir := t.TempDir()
	data, err := os.ReadFile("testdata/example.wasm")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "example.wasm"), data, 0600); err != nil {
		t.Fatal(err)
	}
	m, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := m.Close(); err != nil {
			t.Error(err)
		}
	})
	return m, dir
}
func expectCall(t *testing.T, m *Manager, name string, want types.Value, args ...types.Value) {
	t.Helper()
	r := m.Call(context.Background(), 0, "example."+name, args)
	if r.IsError() || !r.Val.Equal(want) {
		t.Fatalf("%s: got %+v, want %v", name, r, want)
	}
}

func TestStandardWITComponent(t *testing.T) {
	m, _ := fixtureManager(t)
	info, err := m.Load(context.Background(), "example")
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Exports) != 11 || info.Generation != 1 || len(info.SHA256) != 64 {
		t.Fatalf("metadata: %+v", info)
	}
	expectCall(t, m, "add", types.NewInt(42), types.NewInt(19), types.NewInt(23))
	expectCall(t, m, "greet", types.NewStr("Hello, 雪!"), types.NewStr("雪"))
	list := types.NewList([]types.Value{types.NewInt(-7), types.NewInt(42)})
	expectCall(t, m, "echo", list, list)
	record := types.NewMap([][2]types.Value{{types.NewStr("text"), types.NewStr("hi")}, {types.NewStr("count"), types.NewInt(3)}})
	expectCall(t, m, "record-echo", record, record)
	expectCall(t, m, "color-echo", types.NewStr("green"), types.NewStr("green"))
	for _, choice := range []types.Value{types.NewList([]types.Value{types.NewStr("empty"), types.NewInt(0)}), types.NewList([]types.Value{types.NewStr("text"), types.NewStr("hello")}), types.NewList([]types.Value{types.NewStr("number"), types.NewInt(-9)})} {
		expectCall(t, m, "choice-echo", choice, choice)
	}
	for _, option := range []types.Value{types.NewList(nil), types.NewList([]types.Value{types.NewInt(8)})} {
		expectCall(t, m, "option-echo", option, option)
	}
	expectCall(t, m, "checked", types.NewList([]types.Value{types.NewStr("ok"), types.NewInt(9)}), types.NewInt(9))
	expectCall(t, m, "checked", types.NewList([]types.Value{types.NewStr("err"), types.NewStr("negative")}), types.NewInt(-1))
	// Guest globals cannot survive a call or become hidden transaction state.
	expectCall(t, m, "counter", types.NewInt(1))
	expectCall(t, m, "counter", types.NewInt(1))
}

func TestReplacementUnloadAndFailedAdmission(t *testing.T) {
	m, dir := fixtureManager(t)
	if _, err := m.Load(context.Background(), "example"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "example.wasm"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "example.wasm"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Load(context.Background(), "example"); err == nil {
		t.Fatal("invalid replacement admitted")
	}
	expectCall(t, m, "add", types.NewInt(3), types.NewInt(1), types.NewInt(2))
	if err = os.WriteFile(filepath.Join(dir, "example.wasm"), data, 0600); err != nil {
		t.Fatal(err)
	}
	info, err := m.Load(context.Background(), "example")
	if err != nil {
		t.Fatal(err)
	}
	if info.Generation != 2 {
		t.Fatalf("generation %d", info.Generation)
	}
	if !m.Unload("example") || m.Unload("example") {
		t.Fatal("unload state")
	}
	if len(m.List()) != 0 {
		t.Fatal("unload metadata")
	}
	if r := m.Call(context.Background(), 0, "example.add", nil); r.Error != types.E_INVARG {
		t.Fatalf("call after unload: %+v", r)
	}
	if err = m.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Load(context.Background(), "example"); err == nil {
		t.Fatal("closed manager reopened")
	}
}

func TestAdmissionAndGuestBudgets(t *testing.T) {
	m, _ := fixtureManager(t)
	if _, err := m.Load(context.Background(), "example"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		args []types.Value
		code types.ErrorCode
	}{
		{"add", nil, types.E_ARGS},
		{"add", []types.Value{types.NewStr("1"), types.NewInt(2)}, types.E_TYPE},
		{"record-echo", []types.Value{types.NewMap([][2]types.Value{{types.NewStr("text"), types.NewStr("x")}, {types.NewStr("count"), types.NewInt(-1)}})}, types.E_RANGE},
		{"color-echo", []types.Value{types.NewStr("purple")}, types.E_INVARG},
		{"trap", nil, types.E_INVARG},
		{"spin", nil, types.E_QUOTA},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := m.Call(context.Background(), 0, "example."+tc.name, tc.args)
			if r.Error != tc.code {
				t.Fatalf("got %+v, want %v", r, tc.code)
			}
		})
	}
	for _, name := range []string{"../example", "example.wasm", "EXAMPLE", "a/b", ""} {
		if _, err := m.Load(context.Background(), name); err == nil {
			t.Fatalf("path admitted: %q", name)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if r := m.Call(ctx, 0, "example.add", []types.Value{types.NewInt(1), types.NewInt(2)}); r.Error != types.E_QUOTA {
		t.Fatalf("cancel: %+v", r)
	}
}

func TestConcurrentCallsAndGenerationRetirement(t *testing.T) {
	m, _ := fixtureManager(t)
	if _, err := m.Load(context.Background(), "example"); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				r := m.Call(context.Background(), int64(i), "example.counter", nil)
				if r.Error == types.E_NONE && r.Val.Int() != 1 {
					t.Errorf("shared guest state: %+v", r)
				}
				if r.Error != types.E_NONE && r.Error != types.E_INVARG {
					t.Errorf("call: %+v", r)
				}
			}
		}()
	}
	for i := 0; i < 4; i++ {
		m.Unload("example")
		if _, err := m.Load(context.Background(), "example"); err != nil {
			t.Fatal(err)
		}
	}
	wg.Wait()
	// Pin a generation with a real lease, unload it, then allow the deadline to
	// release it. Retirement must not interrupt an already admitted guest.
	done := make(chan types.Result, 1)
	go func() { done <- m.Call(context.Background(), 0, "example.spin", nil) }()
	deadline := time.Now().Add(time.Second)
	for {
		m.mu.Lock()
		g := m.modules["example"]
		leased := g != nil && g.refs != 0
		m.mu.Unlock()
		if leased {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("call never acquired lease")
		}
		time.Sleep(time.Millisecond)
	}
	if !m.Unload("example") {
		t.Fatal("unload")
	}
	if r := <-done; r.Error != types.E_QUOTA {
		t.Fatalf("retired call: %+v", r)
	}
}
