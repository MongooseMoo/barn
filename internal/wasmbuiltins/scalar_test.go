package wasmbuiltins

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/types"
)

func scalarManager(t *testing.T) (*Manager, string) {
	t.Helper()
	dir := t.TempDir()
	data, err := os.ReadFile("testdata/scalar.wasm")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "scalar.wasm"), data, 0600); err != nil {
		t.Fatal(err)
	}
	m, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close() })
	return m, dir
}
func TestNamedScalarComponentConcurrencyAndLimits(t *testing.T) {
	m, dir := scalarManager(t)
	if _, err := m.Load(context.Background(), "scalar"); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for principal := int64(0); principal < 4; principal++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				r := m.Call(context.Background(), principal, "scalar.add", []types.Value{types.NewInt(20), types.NewInt(22)})
				if r.IsError() || r.Val.Int() != 42 {
					t.Errorf("named concurrent guest: %+v", r)
					return
				}
			}
		}()
	}
	wg.Wait()
	for _, name := range []string{"grow", "table-grow"} {
		r := m.Call(context.Background(), 0, "scalar."+name, []types.Value{types.NewInt(1 << 20)})
		if r.IsError() || r.Val.Int() != -1 {
			t.Fatalf("unbounded %s: %+v", name, r)
		}
	}
	for _, tc := range []struct {
		name string
		code types.ErrorCode
	}{{"non-finite", types.E_FLOAT}, {"oversized", types.E_QUOTA}} {
		r := m.Call(context.Background(), 0, "scalar."+tc.name, nil)
		if r.Error != tc.code {
			t.Fatalf("%s: %+v", tc.name, r)
		}
	}
	// The normalization is itself a standard component with the same world.
	m.mu.Lock()
	data := m.modules["scalar"].data
	m.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resolve, err := decodeWIT(ctx, data)
	if err != nil || len(resolve.Worlds) != 1 || resolve.Worlds[0].Exports.Len() != 5 {
		t.Fatalf("normalized contract: %v", err)
	}
	if err = os.WriteFile(filepath.Join(dir, "normalized.wasm"), data, 0600); err != nil {
		t.Fatal(err)
	}
}
func TestModuleAndFileAdmissionLimits(t *testing.T) {
	m, dir := scalarManager(t)
	data, err := os.ReadFile(filepath.Join(dir, "scalar.wasm"))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < MaxModules; i++ {
		name := fmt.Sprintf("m%d", i)
		if err = os.WriteFile(filepath.Join(dir, name+".wasm"), data, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err = m.Load(context.Background(), name); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = m.Load(context.Background(), "scalar"); err == nil {
		t.Fatal("module count exceeded")
	}
	if _, err = m.Load(context.Background(), "m0"); err != nil {
		t.Fatalf("replacement at capacity: %v", err)
	}
	m.Unload("m0")
	f, err := os.Create(filepath.Join(dir, "big.wasm"))
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Truncate(MaxComponentBytes + 1); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Load(context.Background(), "big"); err == nil {
		t.Fatal("oversized file admitted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = m.Load(ctx, "scalar"); err != context.Canceled {
		t.Fatalf("cancelled load: %v", err)
	}
}
func TestPerPrincipalGuestAdmission(t *testing.T) {
	m, _ := fixtureManager(t)
	if _, err := m.Load(context.Background(), "example"); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if r := m.Call(context.Background(), 7, "example.spin", nil); r.Error != types.E_QUOTA {
				t.Errorf("spin: %+v", r)
			}
		}()
	}
	deadline := time.Now().Add(time.Second)
	for {
		m.mu.Lock()
		active := m.principals[7]
		m.mu.Unlock()
		if active == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("guests never admitted")
		}
		time.Sleep(time.Millisecond)
	}
	if r := m.Call(context.Background(), 7, "example.add", []types.Value{types.NewInt(1), types.NewInt(2)}); r.Error != types.E_QUOTA {
		t.Fatalf("principal exceeded capacity: %+v", r)
	}
	if r := m.Call(context.Background(), 8, "example.add", []types.Value{types.NewInt(1), types.NewInt(2)}); r.IsError() || r.Val.Int() != 3 {
		t.Fatalf("one principal monopolized guests: %+v", r)
	}
	wg.Wait()
}
