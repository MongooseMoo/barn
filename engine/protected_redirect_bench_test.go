package engine

import "testing"

// Both paths execute the same wrapper. Only the route into it differs.
func BenchmarkProtectedBuiltinDispatch(b *testing.B) {
	for _, tc := range []struct {
		name string
		call string
	}{
		{"Builtin", "valid(#0)"},
		{"Verb", "#0:bf_valid(#0)"},
	} {
		b.Run(tc.name, func(b *testing.B) {
			s := NewRuntime(protectedRedirectStore(b))
			defer s.Stop()
			if got := s.EvalCommandOutput(2, protectValidSetup+"return 1;"); got != "{1, 1}" {
				b.Fatal(got)
			}
			code := "n = 0; for i in [1..1000] n = n + " + tc.call + "; endfor return n;"
			if got := s.EvalCommandOutput(2, code); got != "{1, 11000}" {
				b.Fatal(got)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if got := s.EvalCommandOutput(2, code); got != "{1, 11000}" {
					b.Fatal(got)
				}
			}
		})
	}
}
