package builtins

import (
	"testing"

	"barn/kernel"
	"barn/types"
)

func TestCryptDES(t *testing.T) {
	// Both Unix (via libcrypt) and Windows (via github.com/digitive/crypt)
	// should produce the same traditional DES crypt(3) hash for a given
	// password and salt. Test against a known good value from ToastStunt.
	result, err := cryptDES("foobar", "SA")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	expected := "SAEmC5UwrAl2A"
	if result != expected {
		t.Errorf("crypt('foobar', 'SA') = %q, expected %q", result, expected)
	}
}

// TestCryptGlibcKnownAnswers proves that Barn's md5crypt ($1$), sha256crypt
// ($5$) and sha512crypt ($6$) reproduce the EXACT output of glibc crypt(3) /
// ToastStunt (which delegates to glibc, toaststunt/src/crypto.cc:373).
//
// Vectors are the published known-answer tests from Ulrich Drepper's SHA-crypt
// specification (http://www.akkadia.org/drepper/SHA-crypt.txt) and Poul-Henning
// Kamp's md5crypt, as carried verbatim in the reference pure-Go implementation
// github.com/GehirnInc/crypt ({sha256,sha512,md5}_crypt/*_test.go).  Exact
// reproduction means Barn-generated hashes verify against any real password
// database produced by glibc/Toast.
func TestCryptGlibcKnownAnswers(t *testing.T) {
	type kat struct {
		name     string
		fn       func(password, salt string) (string, error)
		password string
		salt     string
		want     string
	}
	cases := []kat{
		// sha256crypt — Drepper SHA-crypt.txt vectors.
		{"sha256-default", cryptSHA256, "Hello world!", "$5$saltstring",
			"$5$saltstring$5B8vYYiY.CVt1RlTTf8KbXBH3hsxY/GNooZaBBGWEc5"},
		{"sha256-rounds10000", cryptSHA256, "Hello world!", "$5$rounds=10000$saltstringsaltstring",
			"$5$rounds=10000$saltstringsaltst$3xv.VbSHBb41AL9AvLeujZkZRBAwqFMz2.opqey6IcA"},
		{"sha256-rounds5000-longsalt", cryptSHA256, "This is just a test", "$5$rounds=5000$toolongsaltstring",
			"$5$rounds=5000$toolongsaltstrin$Un/5jzAHMgOGZ5.mWJpuVolil07guHPvOW8mGRcvxa5"},
		// sha512crypt — Drepper SHA-crypt.txt vectors.
		{"sha512-default", cryptSHA512, "Hello world!", "$6$saltstring",
			"$6$saltstring$svn8UoSVapNtMuq1ukKS4tPQd8iKwSMHWjl/O817G3uBnIFNjnQJuesI68u4OTLiBFdcbYEdFCoEOfaS35inz1"},
		{"sha512-rounds10000", cryptSHA512, "Hello world!", "$6$rounds=10000$saltstringsaltstring",
			"$6$rounds=10000$saltstringsaltst$OW1/O6BYHV6BcXZu8QVeXbDWra3Oeqh0sbHbbMCVNSnCM/UrjmM0Dp8vOuZeHBy/YTBmSK6H9qs/y3RnOaw5v."},
		// md5crypt — Poul-Henning Kamp / glibc "$1$" vectors.
		{"md5-emptysalt", cryptMD5, "abcdefghijk", "$1$$",
			"$1$$pL/BYSxMXs.jVuSV1lynn1"},
		{"md5-deadbeef", cryptMD5, "password", "$1$deadbeef$",
			"$1$deadbeef$Q7g0UO4hRC0mgQUQ/qkjZ0"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := c.fn(c.password, c.salt)
			if err != nil {
				t.Fatalf("%s: unexpected error: %v", c.name, err)
			}
			if got != c.want {
				t.Errorf("crypt(%q, %q) =\n  got  %q\n  want %q\n(glibc/Toast parity broken)",
					c.password, c.salt, got, c.want)
			}
		})
	}
}

// TestCryptRoundsHonored proves rounds= actually changes the computation (not a
// silent 1000 cap) and that the emitted prefix advertises the real round count.
func TestCryptRoundsHonored(t *testing.T) {
	a, err := cryptSHA256("testpassword", "$5$rounds=5000$testsalt")
	if err != nil {
		t.Fatal(err)
	}
	b, err := cryptSHA256("testpassword", "$5$rounds=2000$testsalt")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatalf("rounds=5000 and rounds=2000 produced identical output %q", a)
	}
	if got := "$5$rounds=5000$testsalt$"; len(a) < len(got) || a[:len(got)] != got {
		t.Errorf("rounds=5000 output prefix = %q, want it to begin %q", a, got)
	}
}

func TestValueHashHidesAnonymousObjectIdentity(t *testing.T) {
	ctx := kernel.NewTaskContext()
	first := builtinValueHash(ctx, []types.Value{types.NewAnon(12)})
	second := builtinValueHash(ctx, []types.Value{types.NewAnon(13)})
	if first.IsError() || second.IsError() {
		t.Fatalf("value_hash failed: first=%+v second=%+v", first, second)
	}
	if first.Val.Str() != second.Val.Str() {
		t.Fatalf("distinct anonymous hashes differ: %q != %q", first.Val.Str(), second.Val.Str())
	}
}

func TestDecodeBinaryFullyNumericUsesPendingListValueByteLimit(t *testing.T) {
	ctx := kernel.NewTaskContext()
	decoded := types.NewList([]types.Value{types.NewInt(120), types.NewInt(120)})
	ctx.PendingEffects = []kernel.PendingEffect{{
		Kind: kernel.PendingEffectServerOptions,
		ServerOptions: kernel.PendingServerOptions{
			MaxListValueBytes: ValueBytes(decoded),
		},
	}}

	result := builtinDecodeBinary(ctx, []types.Value{types.NewStr("xx"), types.NewInt(1)})
	if result.Flow != types.FlowException || result.Error != types.E_QUOTA {
		t.Fatalf("fully numeric decode at pending byte limit = flow %v error %v value %v, want E_QUOTA", result.Flow, result.Error, result.Val)
	}
}
