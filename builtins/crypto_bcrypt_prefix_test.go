package builtins

import (
	"strings"
	"testing"

	"barn/kernel"
	"barn/types"
)

func TestCryptBcryptPrefixesMatchToast(t *testing.T) {
	for _, prefix := range []string{"$2a$", "$2x$", "$2y$"} {
		t.Run(prefix, func(t *testing.T) {
			got, errCode := cryptPasswordWithPerm(
				"password",
				prefix+"05$1234567890123456",
				true,
			)
			if errCode != types.E_NONE {
				t.Fatalf("crypt prefix %q returned %s", prefix, errCode)
			}
			if !strings.HasPrefix(got, prefix+"05$") {
				t.Fatalf("crypt prefix %q returned %q", prefix, got)
			}
		})
	}

	got, errCode := cryptPasswordWithPerm(
		"password",
		"$2b$05$1234567890123456",
		true,
	)
	if errCode != types.E_NONE || got != "*0" {
		t.Fatalf("unsupported $2b$ crypt = %q, %s; want *0, E_NONE", got, errCode)
	}
}

func TestSaltBcryptPrefixesMatchToast(t *testing.T) {
	ctx := kernel.NewTaskContext()
	for _, prefix := range []string{"$2a$", "$2x$", "$2y$"} {
		t.Run(prefix, func(t *testing.T) {
			result := builtinSalt(ctx, []types.Value{
				types.NewStr(prefix + "05$"),
				types.NewStr("1234567890123456"),
			})
			if !result.IsNormal() {
				t.Fatalf("salt prefix %q returned %+v", prefix, result)
			}
			want := prefix + "05$KRGxLBS0Lxe3KBCwKxOzLe"
			if got := result.Val.Str(); got != want {
				t.Fatalf("salt prefix %q = %q, want %q", prefix, got, want)
			}
		})
	}

	result := builtinSalt(ctx, []types.Value{
		types.NewStr("$2b$05$"),
		types.NewStr("1234567890123456"),
	})
	if !result.IsError() || result.Error != types.E_INVARG {
		t.Fatalf("unsupported $2b$ salt = %+v, want E_INVARG", result)
	}
}
