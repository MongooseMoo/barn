package builtins

import (
	"testing"

	"barn/kernel"
	"barn/types"
)

func TestCryptBcryptPrefixesMatchToast(t *testing.T) {
	const salt = "KRGxLBS0Lxe3KBCwKxOzLe"
	const checksum = "Me5OhXhsCBVMLq7IYo9z2kiiCZMSmz6"

	for _, prefix := range []string{"$2a$", "$2y$"} {
		t.Run(prefix, func(t *testing.T) {
			got, errCode := cryptPasswordWithPerm("foobar", prefix+"05$"+salt, true)
			if errCode != types.E_NONE {
				t.Fatalf("crypt prefix %q returned %s", prefix, errCode)
			}
			want := prefix + "05$" + salt + checksum
			if got != want {
				t.Fatalf("crypt prefix %q = %q, want %q", prefix, got, want)
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

func TestCryptBcryptRawSaltDollarUsesCostDelimiter(t *testing.T) {
	const (
		password = "foobar"
		rawSalt  = "1234$67890123456"
	)
	encodedSalt := bcryptBase64Encode([]byte(rawSalt))
	tests := []struct {
		name     string
		prefix   string
		cost     string
		isWizard bool
		wantErr  types.ErrorCode
	}{
		{name: "2a_default_programmer", prefix: "$2a$", cost: "05"},
		{name: "2y_default_programmer", prefix: "$2y$", cost: "05"},
		{name: "2a_non_default_wizard", prefix: "$2a$", cost: "04", isWizard: true},
		{name: "2y_non_default_wizard", prefix: "$2y$", cost: "04", isWizard: true},
		{name: "2a_non_default_programmer", prefix: "$2a$", cost: "04", wantErr: types.E_PERM},
		{name: "2y_non_default_programmer", prefix: "$2y$", cost: "04", wantErr: types.E_PERM},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := tc.prefix + tc.cost + "$" + rawSalt
			got, errCode := cryptPasswordWithPerm(password, input, tc.isWizard)
			if errCode != tc.wantErr {
				t.Fatalf("cryptPasswordWithPerm(%q) error = %s, want %s", input, errCode, tc.wantErr)
			}
			if tc.wantErr != types.E_NONE {
				return
			}

			control := tc.prefix + tc.cost + "$" + encodedSalt
			want, controlErr := cryptPasswordWithPerm(password, control, tc.isWizard)
			if controlErr != types.E_NONE {
				t.Fatalf("encoded control %q error = %s, want E_NONE", control, controlErr)
			}
			if got != want {
				t.Errorf("cryptPasswordWithPerm(%q) = %q, want encoded-salt result %q", input, got, want)
			}
		})
	}
}

func TestParseBcryptPrefixCostRejectsInvalidTokens(t *testing.T) {
	for _, costToken := range []string{"18446744073709551621", "+10", "-10"} {
		t.Run(costToken, func(t *testing.T) {
			salt := "$2y$" + costToken + "$KRGxLBS0Lxe3KBCwKxOzLe"
			if cost, err := parseBcryptPrefixCost(salt); err == nil {
				t.Fatalf("parseBcryptPrefixCost(%q) = %d, want invalid-token error", salt, cost)
			}
		})
	}
}

func TestCryptBcryptRequiresExactlyTwoCostDigits(t *testing.T) {
	for name, salt := range map[string]string{
		"2a_one_digit":    "$2a$5$KRGxLBS0Lxe3KBCwKxOzLe",
		"2a_three_digits": "$2a$005$KRGxLBS0Lxe3KBCwKxOzLe",
		"2y_one_digit":    "$2y$5$KRGxLBS0Lxe3KBCwKxOzLe",
		"2y_three_digits": "$2y$005$KRGxLBS0Lxe3KBCwKxOzLe",
	} {
		t.Run(name, func(t *testing.T) {
			if got, errCode := cryptPasswordWithPerm("foobar", salt, true); errCode != types.E_INVARG {
				t.Fatalf("crypt malformed cost %q = %q, %s; want E_INVARG", salt, got, errCode)
			}
		})
	}
}

func TestCryptBcryptCostValidationPrecedesPermission(t *testing.T) {
	const salt = "KRGxLBS0Lxe3KBCwKxOzLe"
	tests := []struct {
		name string
		cost string
		want types.ErrorCode
	}{
		{name: "below_range", cost: "03", want: types.E_INVARG},
		{name: "above_range", cost: "32", want: types.E_INVARG},
		{name: "valid_non_default", cost: "10", want: types.E_PERM},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, errCode := cryptPasswordWithPerm("foobar", "$2y$"+tc.cost+"$"+salt, false)
			if errCode != tc.want {
				t.Fatalf("crypt cost %q = %q, %s; want %s", tc.cost, got, errCode, tc.want)
			}
		})
	}
}

func TestCryptBcryptPermissionPrecedesExactCostWidth(t *testing.T) {
	const malformed = "$2y$010$KRGxLBS0Lxe3KBCwKxOzLe"
	if got, errCode := cryptPasswordWithPerm("foobar", malformed, false); errCode != types.E_PERM {
		t.Fatalf("non-wizard malformed non-default cost = %q, %s; want E_PERM", got, errCode)
	}
	if got, errCode := cryptPasswordWithPerm("foobar", malformed, true); errCode != types.E_INVARG {
		t.Fatalf("wizard malformed non-default cost = %q, %s; want E_INVARG", got, errCode)
	}
}

func TestSaltBcryptPrefixesMatchToast(t *testing.T) {
	ctx := kernel.NewTaskContext()
	for _, prefix := range []string{"$2a$", "$2y$"} {
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

	for _, prefix := range []string{"$2b$", "$2x$"} {
		t.Run(prefix, func(t *testing.T) {
			result := builtinSalt(ctx, []types.Value{
				types.NewStr(prefix + "05$"),
				types.NewStr("1234567890123456"),
			})
			if !result.IsError() || result.Error != types.E_INVARG {
				t.Fatalf("unsupported %s salt = %+v, want E_INVARG", prefix, result)
			}
		})
	}
}

func TestSaltBcryptCostWidthMatchesToast(t *testing.T) {
	ctx := kernel.NewTaskContext()
	const randomData = "1234567890123456"
	const encodedSalt = "KRGxLBS0Lxe3KBCwKxOzLe"

	for _, prefix := range []string{"$2a$", "$2y$"} {
		t.Run(prefix+"leading_zero", func(t *testing.T) {
			result := builtinSalt(ctx, []types.Value{
				types.NewStr(prefix + "004$"),
				types.NewStr(randomData),
			})
			if !result.IsNormal() {
				t.Fatalf("salt prefix %q returned %+v", prefix+"004$", result)
			}
			want := prefix + "04$" + encodedSalt
			if got := result.Val.Str(); got != want {
				t.Fatalf("salt prefix %q = %q, want %q", prefix+"004$", got, want)
			}
		})

		t.Run(prefix+"one_digit", func(t *testing.T) {
			result := builtinSalt(ctx, []types.Value{
				types.NewStr(prefix + "4$"),
				types.NewStr(randomData),
			})
			if !result.IsError() || result.Error != types.E_INVARG {
				t.Fatalf("salt prefix %q = %+v, want E_INVARG", prefix+"4$", result)
			}
		})
	}
}
