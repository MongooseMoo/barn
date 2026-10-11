package wasmbuiltins

import (
	"bytes"
	"compress/gzip"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"go.bytecodealliance.org/wit"
)

// This is the unchanged parser artifact distributed by Bytecode Alliance's Go
// modules v0.7.0. Its license and provenance accompany it. Owning the runner lets
// us bound time, memory and output and close every instance (DecodeWIT cannot).
//
//go:embed wasm-tools.wasm.gz
var compressedParser []byte

var parserBytes = sync.OnceValues(func() ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(compressedParser))
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	return io.ReadAll(io.LimitReader(r, 64<<20))
})
var parserCache = sync.OnceValue(wazero.NewCompilationCache)

// Compile the trusted parser once during session setup, before MOO tasks exist.
// Native compilation itself is not interruptible; only parser execution belongs
// inside a caller's load deadline. Every later runner hits this memory cache.
var initializeParser = sync.OnceValue(func() error {
	ctx := context.Background()
	r := wazero.NewRuntimeWithConfig(ctx, parserConfig())
	defer func() { _ = r.Close(ctx) }()
	program, err := parserBytes()
	if err != nil {
		return err
	}
	_, err = r.CompileModule(ctx, program)
	return err
})

func parserConfig() wazero.RuntimeConfig {
	return wazero.NewRuntimeConfig().WithCompilationCache(parserCache()).WithMemoryLimitPages(2048).WithCloseOnContextDone(true)
}

type boundedBuffer struct {
	buffer bytes.Buffer
	limit  int
}

func (b *boundedBuffer) Bytes() []byte  { return b.buffer.Bytes() }
func (b *boundedBuffer) String() string { return b.buffer.String() }

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if len(p) > b.limit-b.buffer.Len() {
		return 0, errors.New("WIT parser output limit exceeded")
	}
	return b.buffer.Write(p)
}

func decodeWIT(ctx context.Context, data []byte) (*wit.Resolve, error) {
	r := wazero.NewRuntimeWithConfig(ctx, parserConfig())
	defer func() { _ = r.Close(context.Background()) }()
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, r); err != nil {
		return nil, err
	}
	program, err := parserBytes()
	if err != nil {
		return nil, err
	}
	compiled, err := r.CompileModule(ctx, program)
	if err != nil {
		return nil, err
	}
	stdout, stderr := &boundedBuffer{limit: 4 << 20}, &boundedBuffer{limit: 4096}
	cfg := wazero.NewModuleConfig().WithArgs("wasm-tools", "component", "wit", "-j", "--all-features").WithStdin(bytes.NewReader(data)).WithStdout(stdout).WithStderr(stderr)
	_, err = r.InstantiateModule(ctx, compiled, cfg)
	if err != nil {
		return nil, fmt.Errorf("WIT parser: %w: %s", err, stderr.String())
	}
	return wit.DecodeJSON(bytes.NewReader(stdout.Bytes()))
}
