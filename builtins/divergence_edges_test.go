package builtins

import (
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func TestDivergenceNumericAndUtilityEdges(t *testing.T) {
	ctx := newTestExecution()

	tests := []struct {
		name    string
		builtin BuiltinFunc
		args    []types.Value
		want    types.Value
		wantErr types.ErrorCode
	}{
		{name: "floatstr precision twenty", builtin: builtinFloatstr, args: []types.Value{types.NewFloat(1.5), types.NewInt(20)}, want: types.NewStr("1.50000000000000000000")},
		{name: "exp underflow", builtin: builtinExp, args: []types.Value{types.NewFloat(-1000)}, wantErr: types.E_FLOAT},
		{name: "empty index", builtin: builtinIndex, args: []types.Value{types.NewStr(""), types.NewStr("")}, want: types.NewInt(1)},
		{name: "empty distance", builtin: builtinDistance, args: []types.Value{types.NewEmptyList(), types.NewEmptyList()}, want: types.NewFloat(0)},
		{name: "shorter distance", builtin: builtinDistance, args: []types.Value{types.NewList([]types.Value{types.NewInt(1), types.NewInt(2)}), types.NewList([]types.Value{types.NewInt(1), types.NewInt(2), types.NewInt(3)})}, want: types.NewFloat(0)},
		{name: "relative heading dimensions", builtin: builtinRelativeHeading, args: []types.Value{types.NewList([]types.Value{types.NewInt(0), types.NewInt(0)}), types.NewList([]types.Value{types.NewInt(1), types.NewInt(1)})}, wantErr: types.E_TYPE},
		{name: "decode tab", builtin: builtinDecodeBinary, args: []types.Value{types.NewStr("~09")}, want: types.NewList([]types.Value{types.NewStr("\t")})},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := test.builtin(ctx, test.args)
			if test.wantErr != types.E_NONE {
				if result.Flow != types.FlowException || result.Error != test.wantErr {
					t.Fatalf("result = flow %v error %v value %v, want %v", result.Flow, result.Error, result.Val, test.wantErr)
				}
				return
			}
			if !result.IsNormal() || !result.Val.Equal(test.want) {
				t.Fatalf("result = flow %v error %v value %v, want %v", result.Flow, result.Error, result.Val, test.want)
			}
		})
	}
}

func TestDivergenceJSONFirstKeyAndInt64(t *testing.T) {
	ctx := newTestExecution()
	duplicate := builtinParseJson(ctx, []types.Value{types.NewStr(`{"a":1,"a":2}`)})
	if !duplicate.IsNormal() || duplicate.Val.Len() != 1 || duplicate.Val.Pairs()[0][1].Int() != 1 {
		t.Fatalf("duplicate-key parse = flow %v error %v value %v, want first value 1", duplicate.Flow, duplicate.Error, duplicate.Val)
	}
	maxInt := builtinParseJson(ctx, []types.Value{types.NewStr("9223372036854775807")})
	if !maxInt.IsNormal() || maxInt.Val.Type() != types.TYPE_INT || maxInt.Val.Int() != int64(9223372036854775807) {
		t.Fatalf("max-int parse = flow %v error %v value %v", maxInt.Flow, maxInt.Error, maxInt.Val)
	}
}

func TestDivergenceCryptoParameters(t *testing.T) {
	ctx := newTestExecution()
	ctx.IsWizard = true

	for _, test := range []struct {
		prefix string
		want   string
	}{
		{prefix: "$1$", want: "$1$V7qMYJaN"},
		{prefix: "$5$", want: "$5$V7qMYJaNbVKOeh4P"},
		{prefix: "$6$", want: "$6$V7qMYJaNbVKOeh4P"},
		{prefix: "bogus", want: "VW"},
	} {
		result := builtinSalt(ctx, []types.Value{types.NewStr(test.prefix), types.NewStr("abcdefghijklmnop")})
		if !result.IsNormal() || result.Val.Str() != test.want {
			t.Fatalf("salt(%q) = flow %v error %v value %v, want %q", test.prefix, result.Flow, result.Error, result.Val, test.want)
		}
	}

	hash := builtinArgon2(ctx, []types.Value{types.NewStr("pw"), types.NewStr("somesalt12345678")})
	const wantHash = "$argon2id$v=19$m=4096,t=3,p=1$c29tZXNhbHQxMjM0NTY3OA$aMxacoOi/qjIgJYiWQiFwZWIushzpAiw6eVJ9Fc/7jE"
	if !hash.IsNormal() || hash.Val.Str() != wantHash {
		t.Fatalf("argon2 defaults = flow %v error %v value %v, want %q", hash.Flow, hash.Error, hash.Val, wantHash)
	}
	garbage := builtinArgon2Verify(ctx, []types.Value{types.NewStr("bogus"), types.NewStr("pw")})
	if !garbage.IsNormal() || garbage.Val.Int() != 0 {
		t.Fatalf("argon2_verify garbage = flow %v error %v value %v, want 0", garbage.Flow, garbage.Error, garbage.Val)
	}
	md5 := builtinStringHmac(ctx, []types.Value{types.NewStr("abc"), types.NewStr("key"), types.NewStr("md5")})
	if md5.Flow != types.FlowException || md5.Error != types.E_INVARG {
		t.Fatalf("string_hmac md5 = flow %v error %v value %v, want E_INVARG", md5.Flow, md5.Error, md5.Val)
	}
}

func TestDivergenceRuntimeIntrospection(t *testing.T) {
	ctx := newTestExecution()
	for _, hidden := range []string{"capitalize", "connection_option", "downcase", "implode", "ltrim", "mapmerge", "read_stdin", "rtrim", "trim", "unique", "upcase"} {
		result := builtinFunctionInfo(ctx, []types.Value{types.NewStr(hidden)})
		if result.Flow != types.FlowException || result.Error != types.E_INVARG {
			t.Errorf("function_info(%q) = flow %v error %v value %v, want E_INVARG", hidden, result.Flow, result.Error, result.Val)
		}
	}
	if result := builtinBufferedOutputLength(ctx, nil); !result.IsNormal() || result.Val.Int() != 65536 {
		t.Fatalf("buffered_output_length() = flow %v error %v value %v, want 65536", result.Flow, result.Error, result.Val)
	}
	if result := builtinCtime(ctx, []types.Value{types.NewInt(9223372036854775807)}); !result.IsNormal() || !strings.Contains(result.Val.Str(), "2146059811") {
		t.Fatalf("ctime(maxint) = flow %v error %v value %v, want clamped Toast year", result.Flow, result.Error, result.Val)
	}
	if result := builtinCtime(ctx, []types.Value{types.NewInt(-9223372036854775807)}); result.Flow != types.FlowException || result.Error != types.E_INVARG {
		t.Fatalf("ctime(-maxint) = flow %v error %v value %v, want E_INVARG", result.Flow, result.Error, result.Val)
	}
	if result := builtinMemoryUsage(ctx, nil); !result.IsNormal() {
		t.Fatalf("memory_usage() = flow %v error %v", result.Flow, result.Error)
	} else {
		for _, value := range result.Val.Elements() {
			if value.Type() != types.TYPE_FLOAT {
				t.Fatalf("memory_usage() element %v has type %v, want float", value, value.Type())
			}
		}
	}
}
