package dbtool

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/config"
)

func TestEvalWASMComponentLifecycle(t *testing.T) {
	dir := t.TempDir()
	data, err := os.ReadFile("../wasmbuiltins/testdata/example.wasm")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "example.wasm"), data, 0600); err != nil {
		t.Fatal(err)
	}
	options := config.DefaultOptions()
	options.WASMDirectory = dir
	var out, errOut bytes.Buffer
	input := `answer = wasm_call("example.add", 20, 22); wasm_unload("example"); wasm_load("example"); return {answer, wasm_call("example.greet", "MOO")};`
	if err = EvalExpression(&out, &errOut, evalStore(t), input, options); err != nil {
		t.Fatalf("eval: %v\n%s", err, errOut.String())
	}
	if got := out.String(); got != "=> {42, \"Hello, MOO!\"}\n" {
		t.Fatalf("result: %q", got)
	}
	if err = os.WriteFile(filepath.Join(dir, "example.wasm"), []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = EvalExpression(&out, &errOut, evalStore(t), "1", options); err == nil || !strings.Contains(errOut.String(), "configure WASM builtins") {
		t.Fatalf("startup admitted corrupt directory: %v", err)
	}
}
