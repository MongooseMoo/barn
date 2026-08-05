package builtins

import (
	"strings"
	"testing"

	"barn/kernel"
	"barn/types"

	cryptbcrypt "github.com/go-crypt/x/bcrypt"
)

const bcrypt2xTestSalt = "/OK.fbVrR/bpIqNJ5ianF."

// These known-answer vectors are from Openwall crypt_blowfish wrapper.c at
// tag CRYPT_BLOWFISH_1_3, commit 3354bb81eea489e972b0a7c63231514ab34f73a0.
func TestCryptBcrypt2xOpenwallKnownAnswers(t *testing.T) {
	tests := []struct {
		name     string
		password string
		setting  string
		want     string
	}{
		{
			name:     "a3",
			password: string([]byte{0xa3}),
			setting:  "$2x$05$" + bcrypt2xTestSalt,
			want:     "$2x$05$/OK.fbVrR/bpIqNJ5ianF.CE5elHaaO4EbggVDjb8P19RukzXSM3e",
		},
		{
			name:     "ff-ff-a3-collision",
			password: string([]byte{0xff, 0xff, 0xa3}),
			setting:  "$2x$05$" + bcrypt2xTestSalt,
			want:     "$2x$05$/OK.fbVrR/bpIqNJ5ianF.CE5elHaaO4EbggVDjb8P19RukzXSM3e",
		},
		{
			name:     "d0-c1-d2-cf-cc-d8",
			password: string([]byte{0xd0, 0xc1, 0xd2, 0xcf, 0xcc, 0xd8}),
			setting:  "$2x$05$6bNw2HLQYeqHYyBfLMsv/O",
			want:     "$2x$05$6bNw2HLQYeqHYyBfLMsv/O9LIGgn8OMzuDoHfof8AQimSGfcSWxnS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, errCode := cryptPasswordWithPerm(tt.password, tt.setting, true)
			if errCode != 0 {
				t.Fatalf("cryptPasswordWithPerm(%x, %q, true) error = %v, want success", []byte(tt.password), tt.setting, errCode)
			}
			if got != tt.want {
				t.Errorf("cryptPasswordWithPerm(%x, %q, true) = %q, want %q", []byte(tt.password), tt.setting, got, tt.want)
			}
		})
	}
}

func TestCryptBcrypt2xVariantSemantics(t *testing.T) {
	t.Run("ascii exact and same body as corrected bcrypt", func(t *testing.T) {
		const password = "U*U"
		const setting2x = "$2x$05$CCCCCCCCCCCCCCCCCCCCC."
		const want2x = "$2x$05$CCCCCCCCCCCCCCCCCCCCC.E5YPO9kmyuRGyh0XouQYb4YMJKvyOeW"

		got2x, errCode := cryptPasswordWithPerm(password, setting2x, true)
		if errCode != 0 {
			t.Fatalf("cryptPasswordWithPerm(%q, %q, true) error = %v, want success", password, setting2x, errCode)
		}
		if got2x != want2x {
			t.Fatalf("cryptPasswordWithPerm(%q, %q, true) = %q, want %q", password, setting2x, got2x, want2x)
		}

		got2y, errCode := cryptPasswordWithPerm(password, "$2y$"+setting2x[4:], true)
		if errCode != 0 {
			t.Fatalf("corrected bcrypt comparison error = %v, want success", errCode)
		}
		if got2x[29:] != got2y[29:] {
			t.Errorf("ASCII bcrypt bodies differ: 2x = %q, 2y = %q", got2x[29:], got2y[29:])
		}
	})

	t.Run("high byte body differs from corrected bcrypt", func(t *testing.T) {
		password := string([]byte{0xa3})
		setting2x := "$2x$05$" + bcrypt2xTestSalt
		got2x, errCode := cryptPasswordWithPerm(password, setting2x, true)
		if errCode != 0 {
			t.Fatalf("2x high-byte hash error = %v, want success", errCode)
		}
		if len(got2x) != 60 {
			t.Fatalf("2x high-byte hash length = %d, want 60: %q", len(got2x), got2x)
		}
		got2y, errCode := cryptPasswordWithPerm(password, "$2y$"+setting2x[4:], true)
		if errCode != 0 {
			t.Fatalf("2y high-byte hash error = %v, want success", errCode)
		}
		if got2x[29:] == got2y[29:] {
			t.Errorf("high-byte bcrypt bodies are both %q, want genuine 2x sign-extension behavior", got2x[29:])
		}
	})
}

func TestCryptBcrypt2xPasswordBoundaries(t *testing.T) {
	t.Run("C string NUL truncation", func(t *testing.T) {
		setting := "$2x$05$CCCCCCCCCCCCCCCCCCCCC."
		beforeNUL, errCode := cryptPasswordWithPerm("U*U", setting, true)
		if errCode != 0 {
			t.Fatalf("baseline hash error = %v, want success", errCode)
		}
		withNUL, errCode := cryptPasswordWithPerm("U*U\x00ignored", setting, true)
		if errCode != 0 {
			t.Fatalf("NUL-containing hash error = %v, want success", errCode)
		}
		if withNUL != beforeNUL {
			t.Errorf("hash after embedded NUL = %q, want %q", withNUL, beforeNUL)
		}
	})

	t.Run("72 byte schedule limit", func(t *testing.T) {
		const first72 = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
		const setting = "$2x$05$abcdefghijklmnopqrstuu"
		const want = "$2x$05$abcdefghijklmnopqrstuu5s2v8.iXieOjg/.AySBTTZIIVFJeBui"
		if len(first72) != 72 {
			t.Fatalf("test password length = %d, want 72", len(first72))
		}

		got, errCode := cryptPasswordWithPerm(first72+"chars after 72 are ignored", setting, true)
		if errCode != 0 {
			t.Fatalf("long password hash error = %v, want success", errCode)
		}
		if got != want {
			t.Errorf("long password hash = %q, want %q", got, want)
		}
	})
}

func TestCryptBcrypt2xParserAndPermissionOrdering(t *testing.T) {
	tests := []struct {
		name     string
		setting  string
		isWizard bool
		wantErr  types.ErrorCode
	}{
		{name: "exact 04 wizard", setting: "$2x$04$" + bcrypt2xTestSalt, isWizard: true},
		{name: "exact 04 programmer", setting: "$2x$04$" + bcrypt2xTestSalt, wantErr: types.E_PERM},
		{name: "one digit wizard", setting: "$2x$4$" + bcrypt2xTestSalt, isWizard: true, wantErr: types.E_INVARG},
		{name: "one digit programmer", setting: "$2x$4$" + bcrypt2xTestSalt, wantErr: types.E_PERM},
		{name: "three digits wizard", setting: "$2x$004$" + bcrypt2xTestSalt, isWizard: true, wantErr: types.E_INVARG},
		{name: "three digits programmer", setting: "$2x$004$" + bcrypt2xTestSalt, wantErr: types.E_PERM},
		{name: "exact 05 programmer", setting: "$2x$05$" + bcrypt2xTestSalt},
		{name: "out of range before permission", setting: "$2x$03$" + bcrypt2xTestSalt, wantErr: types.E_INVARG},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, errCode := cryptPasswordWithPerm("password", tt.setting, tt.isWizard)
			if errCode != tt.wantErr {
				t.Fatalf("cryptPasswordWithPerm(%q, %q, %t) error = %v, want %v", "password", tt.setting, tt.isWizard, errCode, tt.wantErr)
			}
			if tt.wantErr == 0 {
				if len(got) != 60 {
					t.Errorf("successful 2x hash length = %d, want 60: %q", len(got), got)
				}
				if !strings.HasPrefix(got, tt.setting[:7]) {
					t.Errorf("successful 2x hash = %q, want prefix %q", got, tt.setting[:7])
				}
			}
		})
	}
}

func TestCryptBcrypt2xSettingValidation(t *testing.T) {
	tests := []struct {
		name    string
		setting string
	}{
		{name: "short salt", setting: "$2x$05$short"},
		{name: "invalid alphabet", setting: "$2x$05$!!!!!!!!!!!!!!!!!!!!!!"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, errCode := cryptPasswordWithPerm("password", tt.setting, true); errCode != types.E_INVARG {
				t.Errorf("cryptPasswordWithPerm(%q, %q, true) = (%q, %v), want E_INVARG", "password", tt.setting, got, errCode)
			}
		})
	}

	t.Run("full hash setting", func(t *testing.T) {
		const password = "U*U"
		const fullHash = "$2x$05$CCCCCCCCCCCCCCCCCCCCC.E5YPO9kmyuRGyh0XouQYb4YMJKvyOeW"
		got, errCode := cryptPasswordWithPerm(password, fullHash, true)
		if errCode != 0 {
			t.Fatalf("full hash setting error = %v, want success", errCode)
		}
		if got != fullHash {
			t.Errorf("cryptPasswordWithPerm(%q, fullHash, true) = %q, want %q", password, got, fullHash)
		}
	})

	t.Run("raw 16 byte salt", func(t *testing.T) {
		rawSalt, err := cryptbcrypt.Base64Decode([]byte(bcrypt2xTestSalt))
		if err != nil {
			t.Fatalf("decoding test salt: %v", err)
		}
		got, errCode := cryptPasswordWithPerm(string([]byte{0xa3}), "$2x$05$"+string(rawSalt), true)
		if errCode != 0 {
			t.Fatalf("raw salt setting error = %v, want success", errCode)
		}
		const want = "$2x$05$/OK.fbVrR/bpIqNJ5ianF.CE5elHaaO4EbggVDjb8P19RukzXSM3e"
		if got != want {
			t.Errorf("raw salt hash = %q, want %q", got, want)
		}
	})
}

func TestSaltRejectsBcrypt2x(t *testing.T) {
	ctx := kernel.NewTaskContext()
	result := builtinSalt(ctx, []types.Value{
		types.NewStr("$2x$05$"),
		types.NewStr(strings.Repeat("A", 16)),
	})
	if result.Flow != types.FlowException || result.Error != types.E_INVARG {
		t.Errorf("salt($2x$) = flow %v error %v value %v, want E_INVARG", result.Flow, result.Error, result.Val)
	}
}
