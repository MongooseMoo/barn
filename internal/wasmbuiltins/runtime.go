package wasmbuiltins

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"regexp"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/MongooseMoo/barn/metrics"
	"github.com/MongooseMoo/barn/types"
	"github.com/samyfodil/wazy"
	"github.com/samyfodil/wazy/component"
	"go.bytecodealliance.org/wit"
)

const (
	MaxModules        = 32
	MaxComponentBytes = 16 << 20
	MemoryPages       = 64 // Four MiB per instance; admission also bounds core instances.
	CallTimeout       = 250 * time.Millisecond
)

var moduleName = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

// Manager owns a session's components. A module name selects an operator-owned
// file, never a path supplied by MOO. Every call uses fresh guest state.
type Manager struct {
	mu         sync.Mutex
	root       *os.Root
	modules    map[string]*generation
	principals map[int64]int
	next       uint64
	closed     bool
	active     sync.WaitGroup
	loads      chan struct{}
	calls      chan struct{}
	closeOnce  sync.Once
	closeErr   error
}

type generation struct {
	data      []byte
	runtime   wazy.Runtime
	cache     *component.CompileCache
	functions map[string]*function
	info      Info
	refs      int // Protected by Manager.mu.
	retired   bool
}

// Info is detached metadata, safe to retain across replacement and unloading.
type Info struct {
	Name       string
	SHA256     string
	Generation uint64
	Exports    []Export
}
type Export struct {
	Name       string
	Parameters []Parameter
	Result     string
}
type Parameter struct{ Name, Type string }

func Open(directory string) (*Manager, error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	if err := initializeParser(); err != nil {
		_ = root.Close()
		return nil, fmt.Errorf("compile WIT parser: %w", err)
	}
	return &Manager{root: root, modules: make(map[string]*generation), loads: make(chan struct{}, 1), calls: make(chan struct{}, 8)}, nil
}

// LoadAll loads the operator directory at startup. The caller closes the manager
// if any file fails, so a server never starts with a partially admitted directory.
func (m *Manager) LoadAll() error {
	f, err := m.root.Open(".")
	if err != nil {
		return err
	}
	defer f.Close()
	entries, err := f.ReadDir(-1)
	if err != nil {
		return err
	}
	slices.SortFunc(entries, func(a, b os.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".wasm") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".wasm")
		if _, err := m.Load(context.Background(), name); err != nil {
			return fmt.Errorf("load component %q: %w", name, err)
		}
	}
	return nil
}

func (m *Manager) Load(ctx context.Context, name string) (info Info, err error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return info, err
	}
	if !moduleName.MatchString(name) {
		return info, errors.New("invalid module name")
	}
	select {
	case m.loads <- struct{}{}:
		defer func() { <-m.loads }()
	default:
		return info, errors.New("a component load is already in progress")
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return info, errors.New("component session is closed")
	}
	if len(m.modules) >= MaxModules && m.modules[name] == nil {
		m.mu.Unlock()
		return info, errors.New("component module limit reached")
	}
	m.active.Add(1)
	m.mu.Unlock()
	defer m.active.Done()
	stat, err := m.root.Lstat(name + ".wasm")
	if err != nil {
		return info, err
	}
	if !stat.Mode().IsRegular() || stat.Size() > MaxComponentBytes {
		return info, errors.New("component must be a regular file of at most 16 MiB")
	}
	f, err := m.root.Open(name + ".wasm")
	if err != nil {
		return info, err
	}
	defer f.Close()
	stat, err = f.Stat()
	if err != nil {
		return info, err
	}
	if !stat.Mode().IsRegular() || stat.Size() > MaxComponentBytes {
		return info, errors.New("component must be a regular file of at most 16 MiB")
	}
	data, err := io.ReadAll(io.LimitReader(f, MaxComponentBytes+1))
	if err != nil {
		return info, err
	}
	if len(data) > MaxComponentBytes {
		return info, errors.New("component exceeds 16 MiB")
	}
	g, err := prepare(ctx, data)
	if err != nil {
		return info, err
	}
	installed := false
	defer func() {
		if !installed {
			g.close()
		}
	}()
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return info, errors.New("component session is closed")
	}
	m.next++
	g.info.Name, g.info.Generation = name, m.next
	old := m.modules[name]
	m.modules[name] = g
	installed = true
	var dispose *generation
	if old != nil {
		old.retired = true
		if old.refs == 0 {
			dispose = old
		}
	}
	info = cloneInfo(g.info)
	m.mu.Unlock()
	if dispose != nil {
		dispose.close()
	}
	return info, nil
}

// singleCore fences aggregate guest memory and excludes nested components. This
// is an admission profile for standard components, not a private binary format.
func singleCore(data []byte) error {
	if len(data) < 8 || !bytes.Equal(data[:8], []byte{0, 97, 115, 109, 13, 0, 1, 0}) {
		return errors.New("expected a WebAssembly component")
	}
	cores, instances := 0, uint64(0)
	canons := uint64(0)
	for pos := 8; pos < len(data); {
		id := data[pos]
		pos++
		n, size := binary.Uvarint(data[pos:])
		if size <= 0 || size > 5 || n > uint64(len(data)-pos-size) {
			return errors.New("invalid component section")
		}
		pos += size
		if id == 1 {
			cores++
			if err := coreAllocations(data[pos : pos+int(n)]); err != nil {
				return err
			}
		}
		if id == 2 {
			count, used := binary.Uvarint(data[pos : pos+int(n)])
			if used <= 0 || used > 5 || count > 8 || instances+count > 8 {
				return errors.New("component exceeds eight core instances")
			}
			instances += count
		}
		if id == 4 {
			return errors.New("nested components are not supported")
		}
		if id == 8 {
			count, err := canonicalLifts(data[pos : pos+int(n)])
			if err != nil {
				return err
			}
			canons += count
			if canons > 64 {
				return errors.New("component exceeds 64 canonical lifts")
			}
		}
		if id == 9 {
			return errors.New("component-level start functions are not supported")
		}
		pos += int(n)
	}
	if cores != 1 {
		return errors.New("component must contain exactly one core module")
	}
	return nil
}

func prepare(ctx context.Context, data []byte) (g *generation, err error) {
	// Malformed ABI data must fail admission rather than escape as a Go panic.
	defer func() {
		if p := recover(); p != nil {
			logPanic("load", p)
			if g != nil {
				g.close()
			}
			g = nil
			err = errors.New("invalid component encoding")
		}
	}()
	if err = singleCore(data); err != nil {
		return nil, err
	}
	resolve, err := decodeWIT(ctx, data)
	if err != nil {
		return nil, fmt.Errorf("decode component WIT: %w", err)
	}
	if len(resolve.Worlds) != 1 {
		return nil, errors.New("component must expose one WIT world")
	}
	w := resolve.Worlds[0]
	for _, item := range w.Imports.All() {
		// wasm-tools places world-local type definitions in Imports even when
		// the binary has no imports. Definitions grant no host capability.
		t, ok := item.(*wit.TypeDef)
		if !ok {
			return nil, errors.New("builtin components cannot import host interfaces or WASI")
		}
		if _, err := plan(t, 0); err != nil {
			return nil, err
		}
	}
	g = &generation{data: data, functions: make(map[string]*function)}
	for name, item := range w.Exports.All() {
		f, ok := item.(*wit.Function)
		if !ok {
			if _, ok := item.(*wit.TypeDef); ok {
				continue
			}
			return nil, errors.New("export WIT functions directly from the world")
		}
		if !f.IsFreestanding() || len(f.Params) > 16 || len(f.Results) > 1 || len(g.functions) >= 64 {
			return nil, errors.New("unsupported component function signature")
		}
		fn := &function{name: name}
		ex := Export{Name: name, Result: "unit"}
		for _, p := range f.Params {
			s, e := plan(p.Type, 0)
			if e != nil {
				return nil, fmt.Errorf("%s.%s: %w", name, p.Name, e)
			}
			fn.params = append(fn.params, field{p.Name, s})
			ex.Parameters = append(ex.Parameters, Parameter{p.Name, s.String()})
		}
		if len(f.Results) == 1 {
			s, e := plan(f.Results[0].Type, 0)
			if e != nil {
				return nil, fmt.Errorf("%s result: %w", name, e)
			}
			fn.result = s
			ex.Result = s.String()
		}
		g.functions[name] = fn
		g.info.Exports = append(g.info.Exports, ex)
	}
	if len(g.functions) == 0 {
		return nil, errors.New("component has no callable exports")
	}
	slices.SortFunc(g.info.Exports, func(a, b Export) int { return strings.Compare(a.Name, b.Name) })
	g.info.SHA256 = fmt.Sprintf("%x", sha256.Sum256(data))
	g.runtime = wazy.NewRuntimeWithConfig(ctx, wazy.NewRuntimeConfig().WithMemoryLimitPages(MemoryPages).WithCloseOnContextDone(true))
	g.data = withGraphInstantiation(data)
	g.cache = component.NewCompileCache()
	inst, err := component.Instantiate(ctx, g.runtime, g.data, component.WithCompileCache(g.cache))
	if err != nil {
		g.close()
		return nil, fmt.Errorf("validate component: %w", err)
	}
	if err = inst.Close(context.Background()); err != nil {
		g.close()
		return nil, err
	}
	return g, nil
}

func (g *generation) close() {
	if g.cache != nil {
		_ = g.cache.Close(context.Background())
	}
	if g.runtime != nil {
		_ = g.runtime.Close(context.Background())
	}
}

func cloneInfo(in Info) Info {
	in.Exports = slices.Clone(in.Exports)
	for i := range in.Exports {
		in.Exports[i].Parameters = slices.Clone(in.Exports[i].Parameters)
	}
	return in
}
func (m *Manager) List() []Info {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]Info, 0, len(m.modules))
	for _, g := range m.modules {
		result = append(result, cloneInfo(g.info))
	}
	slices.SortFunc(result, func(a, b Info) int { return strings.Compare(a.Name, b.Name) })
	return result
}
func (m *Manager) Unload(name string) bool {
	m.mu.Lock()
	g := m.modules[name]
	if g != nil {
		delete(m.modules, name)
		g.retired = true
	}
	dispose := g != nil && g.refs == 0
	m.mu.Unlock()
	if dispose {
		g.close()
	}
	return g != nil
}

func (m *Manager) Call(ctx context.Context, principal int64, target string, args []types.Value) (result types.Result) {
	select {
	case m.calls <- struct{}{}:
		defer func() { <-m.calls }()
	default:
		return types.Err(types.E_QUOTA)
	}
	name, export, ok := strings.Cut(target, ".")
	if !ok {
		return types.Err(types.E_INVARG)
	}
	m.mu.Lock()
	g := m.modules[name]
	if m.closed || g == nil || g.functions[export] == nil {
		m.mu.Unlock()
		return types.Err(types.E_INVARG)
	}
	fn := g.functions[export]
	if m.principals == nil {
		m.principals = make(map[int64]int)
	}
	if m.principals[principal] >= 2 {
		m.mu.Unlock()
		return types.Err(types.E_QUOTA)
	}
	m.principals[principal]++
	g.refs++
	m.active.Add(1)
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		g.refs--
		m.principals[principal]--
		if m.principals[principal] == 0 {
			delete(m.principals, principal)
		}
		dispose := g.retired && g.refs == 0
		m.mu.Unlock()
		if dispose {
			g.close()
		}
		m.active.Done()
	}()
	defer func() {
		if p := recover(); p != nil {
			logPanic("call", p)
			result = types.Err(types.E_INVARG)
		}
	}()
	if len(args) != len(fn.params) {
		return types.Err(types.E_ARGS)
	}
	b := &budget{}
	values := make([]any, len(args))
	for i, p := range fn.params {
		v, e := p.shape.lower(args[i], b)
		if e != types.E_NONE {
			return types.Err(e)
		}
		values[i] = v
	}
	ctx, cancel := context.WithTimeout(ctx, CallTimeout)
	defer cancel()
	inst, err := component.Instantiate(ctx, g.runtime, g.data, component.WithCompileCache(g.cache))
	if err != nil {
		return guestError(ctx)
	}
	defer func() {
		if err := inst.Close(context.Background()); err != nil {
			slog.Error("close WASM guest", slog.Any("err", err))
		}
	}()
	out, err := inst.Call(ctx, export, values...)
	if err != nil {
		return guestError(ctx)
	}
	if fn.result == nil {
		if len(out) != 0 {
			return types.Err(types.E_INVARG)
		}
		return types.Ok(types.NewInt(0))
	}
	if len(out) != 1 {
		return types.Err(types.E_INVARG)
	}
	v, e := fn.result.lift(out[0], &budget{})
	if e != types.E_NONE {
		return types.Err(e)
	}
	return types.Ok(v)
}
func guestError(ctx context.Context) types.Result {
	if ctx.Err() != nil {
		return types.Err(types.E_QUOTA)
	}
	return types.Err(types.E_INVARG)
}

func logPanic(operation string, p any) {
	metrics.WASMFaultsRecovered.Add(1)
	slog.Error("panic in WASM builtin", slog.String("operation", operation), slog.String("panic", fmt.Sprint(p)), slog.String("go_stack", string(debug.Stack())))
}
func (m *Manager) Close() error {
	m.closeOnce.Do(func() {
		m.mu.Lock()
		m.closed = true
		var dispose []*generation
		for name, g := range m.modules {
			delete(m.modules, name)
			g.retired = true
			if g.refs == 0 {
				dispose = append(dispose, g)
			}
		}
		m.mu.Unlock()
		for _, g := range dispose {
			g.close()
		}
		m.active.Wait()
		m.closeErr = m.root.Close()
	})
	return m.closeErr
}
