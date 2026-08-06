package builtins

import (
	"strings"
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
	rawSalts := []struct {
		name string
		raw  string
	}{
		{name: "dollar_first", raw: "$123456789012345"},
		{name: "dollar_middle", raw: "1234567$89012345"},
		{name: "dollar_last", raw: "123456789012345$"},
		{name: "multiple_dollars", raw: "$123$56789$12345"},
		{name: "nul_and_high_bytes", raw: string([]byte{0x00, '$', 0x80, 0xff, 0x01, 0x7f, 'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I', 'J'})},
	}
	for _, tc := range rawSalts {
		t.Run(tc.name, func(t *testing.T) {
			if got := len(tc.raw); got != 16 {
				t.Fatalf("raw salt length = %d, want 16", got)
			}
			for _, prefix := range []string{"$2a$", "$2y$"} {
				t.Run(prefix[1:3], func(t *testing.T) {
					assertRawBcryptMatchesEncoded(t, "foobar", prefix, "04", tc.raw, true)
				})
			}
		})
	}
}

func TestCryptBcryptRawSaltDollarPreservesPermissionOrdering(t *testing.T) {
	const rawSalt = "$123$56789$12345"
	tests := []struct {
		name     string
		prefix   string
		cost     string
		isWizard bool
		wantErr  types.ErrorCode
	}{
		{name: "2a_default_programmer", prefix: "$2a$", cost: "05"},
		{name: "2y_default_programmer", prefix: "$2y$", cost: "05"},
		{name: "2a_non_default_programmer", prefix: "$2a$", cost: "04", wantErr: types.E_PERM},
		{name: "2y_non_default_programmer", prefix: "$2y$", cost: "04", wantErr: types.E_PERM},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.wantErr == types.E_NONE {
				assertRawBcryptMatchesEncoded(t, "foobar", tc.prefix, tc.cost, rawSalt, tc.isWizard)
				return
			}
			input := tc.prefix + tc.cost + "$" + rawSalt
			_, errCode := cryptPasswordWithPerm("foobar", input, tc.isWizard)
			if errCode != tc.wantErr {
				t.Fatalf("cryptPasswordWithPerm(%q) error = %s, want %s", input, errCode, tc.wantErr)
			}
		})
	}
}

func assertRawBcryptMatchesEncoded(t *testing.T, password, prefix, cost, rawSalt string, isWizard bool) {
	t.Helper()
	input := prefix + cost + "$" + rawSalt
	got, errCode := cryptPasswordWithPerm(password, input, isWizard)
	if errCode != types.E_NONE {
		t.Fatalf("cryptPasswordWithPerm(%q) error = %s, want E_NONE", input, errCode)
	}
	control := prefix + cost + "$" + bcryptBase64Encode([]byte(rawSalt))
	want, controlErr := cryptPasswordWithPerm(password, control, isWizard)
	if controlErr != types.E_NONE {
		t.Fatalf("encoded control %q error = %s, want E_NONE", control, controlErr)
	}
	if got != want {
		t.Errorf("cryptPasswordWithPerm(%q) = %q, want encoded-salt result %q", input, got, want)
	}
}

func TestParseBcryptPrefixCostTokens(t *testing.T) {
	leadingZeros := strings.Repeat("0", 64)
	tests := []struct {
		name      string
		costToken string
		want      int
		wantErr   bool
	}{
		{name: "leading_zeros_min", costToken: leadingZeros + "4", want: 4},
		{name: "leading_zeros_default", costToken: leadingZeros + "5", want: 5},
		{name: "leading_zeros_max", costToken: leadingZeros + "31", want: 31},
		{name: "overflow", costToken: "18446744073709551621", wantErr: true},
		{name: "leading_plus", costToken: "+10", wantErr: true},
		{name: "leading_minus", costToken: "-10", wantErr: true},
		{name: "alphabetic", costToken: "0x04", wantErr: true},
		{name: "empty", costToken: "", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			salt := "$2y$" + tc.costToken + "$KRGxLBS0Lxe3KBCwKxOzLe"
			got, err := parseBcryptPrefixCost(salt)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseBcryptPrefixCost(%q) = %d, want error", salt, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseBcryptPrefixCost(%q) error = %v, want nil", salt, err)
			}
			if got != tc.want {
				t.Errorf("parseBcryptPrefixCost(%q) = %d, want %d", salt, got, tc.want)
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
