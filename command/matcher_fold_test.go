package command

import (
	"strings"
	"testing"
)

func TestLowerCompareMatchesToLower(t *testing.T) {
	words := []string{"", "a", "A", "lamp", "Lamp", "LAMP", "lampshade", "La", "brass lamp",
		"Émile", "émile", "ÉMILE", "straße", "STRASSE", "İstanbul", "istanbul", "K", "K", "k", "x\x80"}
	for _, s := range words {
		for _, w := range words {
			lower := strings.ToLower(w)
			if got, want := lowerEquals(s, lower), strings.ToLower(s) == lower; got != want {
				t.Errorf("lowerEquals(%q, %q) = %v, want %v", s, lower, got, want)
			}
			if got, want := lowerHasPrefix(s, lower), strings.HasPrefix(strings.ToLower(s), lower); got != want {
				t.Errorf("lowerHasPrefix(%q, %q) = %v, want %v", s, lower, got, want)
			}
		}
	}
}
