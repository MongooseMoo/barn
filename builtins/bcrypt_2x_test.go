package builtins

import (
	"strings"
	"testing"

	"barn/types"
)

const bcrypt2xTestSalt = "/OK.fbVrR/bpIqNJ5ianF."

func TestCryptBcrypt2xKnownAnswers(t *testing.T) {
	tests := []struct {
		name     string
		password string
		setting  string
		want     string
	}{
		{
			name:     "ascii",
			password: "U*U",
			setting:  "$2x$05$CCCCCCCCCCCCCCCCCCCCC.",
			want:     "$2x$05$CCCCCCCCCCCCCCCCCCCCC.E5YPO9kmyuRGyh0XouQYb4YMJKvyOeW",
		},
		{
			name:     "high_byte",
			password: string([]byte{0xa3}),
			setting:  "$2x$05$" + bcrypt2xTestSalt,
			want:     "$2x$05$/OK.fbVrR/bpIqNJ5ianF.CE5elHaaO4EbggVDjb8P19RukzXSM3e",
		},
		{
			name:     "historical_collision",
			password: string([]byte{0xff, 0xff, 0xa3}),
			setting:  "$2x$05$" + bcrypt2xTestSalt,
			want:     "$2x$05$/OK.fbVrR/bpIqNJ5ianF.CE5elHaaO4EbggVDjb8P19RukzXSM3e",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, errCode := cryptPasswordWithPerm(tc.password, tc.setting, true)
			if errCode != types.E_NONE {
				t.Fatalf("cryptPasswordWithPerm(%x, %q, true) error = %s, want E_NONE", []byte(tc.password), tc.setting, errCode)
			}
			if got != tc.want {
				t.Errorf("cryptPasswordWithPerm(%x, %q, true) = %q, want %q", []byte(tc.password), tc.setting, got, tc.want)
			}
		})
	}
}

func TestCryptBcrypt2xMatchesCorrectedOnlyForASCII(t *testing.T) {
	tests := []struct {
		name      string
		password  string
		setting   string
		wantEqual bool
	}{
		{name: "ascii", password: "U*U", setting: "$2x$05$CCCCCCCCCCCCCCCCCCCCC.", wantEqual: true},
		{name: "high_byte", password: string([]byte{0xa3}), setting: "$2x$05$" + bcrypt2xTestSalt},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got2x, errCode := cryptPasswordWithPerm(tc.password, tc.setting, true)
			if errCode != types.E_NONE {
				t.Fatalf("2x hash error = %s, want E_NONE", errCode)
			}
			if len(got2x) != 60 {
				t.Fatalf("2x hash = %q (length %d), want length 60", got2x, len(got2x))
			}
			got2y, errCode := cryptPasswordWithPerm(tc.password, "$2y$"+tc.setting[4:], true)
			if errCode != types.E_NONE {
				t.Fatalf("2y hash error = %s, want E_NONE", errCode)
			}
			if equal := got2x[4:] == got2y[4:]; equal != tc.wantEqual {
				t.Errorf("2x and 2y bodies equal = %t, want %t\n2x: %q\n2y: %q", equal, tc.wantEqual, got2x, got2y)
			}
		})
	}
}

func TestCryptBcrypt2xPermissionAndCostWidthOrdering(t *testing.T) {
	tests := []struct {
		name     string
		cost     string
		isWizard bool
		wantErr  types.ErrorCode
	}{
		{name: "exact_two_digits_wizard", cost: "04", isWizard: true},
		{name: "exact_two_digits_programmer", cost: "04", wantErr: types.E_PERM},
		{name: "default_programmer", cost: "05"},
		{name: "one_digit_wizard", cost: "4", isWizard: true, wantErr: types.E_INVARG},
		{name: "one_digit_programmer", cost: "4", wantErr: types.E_PERM},
		{name: "three_digits_wizard", cost: "004", isWizard: true, wantErr: types.E_INVARG},
		{name: "three_digits_programmer", cost: "004", wantErr: types.E_PERM},
		{name: "out_of_range_precedes_permission", cost: "03", wantErr: types.E_INVARG},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setting := "$2x$" + tc.cost + "$" + bcrypt2xTestSalt
			got, errCode := cryptPasswordWithPerm("password", setting, tc.isWizard)
			if errCode != tc.wantErr {
				t.Fatalf("cryptPasswordWithPerm(%q, wizard=%t) = %q, %s; want %s", setting, tc.isWizard, got, errCode, tc.wantErr)
			}
			if tc.wantErr == types.E_NONE && (!strings.HasPrefix(got, "$2x$"+tc.cost+"$") || len(got) != 60) {
				t.Errorf("successful 2x hash = %q, want 60-byte hash with preserved setting prefix", got)
			}
		})
	}
}

func TestCryptBcrypt2xRawSaltDollarUsesCostDelimiter(t *testing.T) {
	rawSalts := []string{
		"$123456789012345",
		"1234567$89012345",
		"123456789012345$",
		"$123$56789$12345",
		string([]byte{0x00, '$', 0x80, 0xff, 0x01, 0x7f, 'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I', 'J'}),
	}

	for _, rawSalt := range rawSalts {
		if got := len(rawSalt); got != 16 {
			t.Fatalf("raw salt length = %d, want 16", got)
		}
		assertRawBcryptMatchesEncoded(t, string([]byte{0xa3}), "$2x$", "04", rawSalt, true)
	}
}
