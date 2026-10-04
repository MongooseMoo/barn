package builtins

import (
	"testing"

	"github.com/MongooseMoo/barn/parser"
	"github.com/MongooseMoo/barn/types"
	"github.com/MongooseMoo/barn/verb"
)

func TestVerbCodeEditRecompilePreservesStringBytes(t *testing.T) {
	ctx, _, object := b2aTestContext(t)
	value := "tab\tcarriage\rhigh\x80quote\"slash\\"
	source := "return \"tab\tcarriage\rhigh\x80quote\\\"slash\\\\\";"
	code := types.NewList([]types.Value{types.NewStr(source)})
	for range 3 {
		result := builtinSetVerbCode(ctx, []types.Value{types.NewObj(object), types.NewStr("scratch"), code})
		if result.IsError() || result.Val.Type() != types.TYPE_LIST || result.Val.Len() != 0 {
			t.Fatalf("recompile failed: %v", result)
		}
		result = builtinVerbCode(ctx, []types.Value{types.NewObj(object), types.NewStr("scratch")})
		if result.IsError() || result.Val.Len() != 1 || result.Val.Get(1).Str() != source {
			t.Fatalf("canonical source changed: %v; want %q", result, source)
		}
		code = result.Val
		program, err := parser.NewParser(code.Get(1).Str()).ParseProgram()
		if err != nil {
			t.Fatal(err)
		}
		literal := program.Statements[0].(*verb.ReturnStmt).Value.(*verb.LiteralExpr)
		if literal.StringValue != value {
			t.Fatalf("literal changed: %q; want %q", literal.StringValue, value)
		}
	}
}
