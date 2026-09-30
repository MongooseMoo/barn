package builtins

import (
	"bytes"
	"fmt"
	"testing"
)

func BenchmarkFileConversion(b *testing.B) {
	for _, n := range []int{64, 65536} {
		for _, shape := range []string{"text", "filtered", "mixed", "binary", "escaped"} {
			b.Run(fmt.Sprintf("%s%d", shape, n), func(b *testing.B) {
				pattern := []byte("abcdefgh")
				switch shape {
				case "filtered":
					pattern = []byte{0, 1, 2, 3}
				case "mixed":
					pattern = []byte{'a', 0, ' ', 255}
				case "escaped":
					pattern = []byte{0, 255, '~', 1}
				}
				data := bytes.Repeat(pattern, n/len(pattern))
				convert := filterTextMode
				if shape == "binary" || shape == "escaped" {
					convert = encodeBinaryBytes
				}
				want := convert(data)
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					if got := convert(data); got != want {
						b.Fatal("conversion changed")
					}
				}
			})
		}
	}
}

func TestFileConversionAllBytes(t *testing.T) {
	data := make([]byte, 256)
	for i := range data {
		data[i] = byte(i)
	}
	var text, binary string
	for _, ch := range data {
		if ch >= 32 && ch <= 126 {
			text += string(ch)
		}
		if ch < 32 || ch > 126 || ch == '~' {
			binary += fmt.Sprintf("~%02X", ch)
		} else {
			binary += string(ch)
		}
	}
	if got := filterTextMode(data); got != text {
		t.Fatal("text mismatch")
	}
	if got := encodeBinaryBytes(data); got != binary {
		t.Fatal("binary mismatch")
	}
	if filterTextMode(nil) != "" || encodeBinaryBytes(nil) != "" {
		t.Fatal("empty mismatch")
	}
}

// The promotion verifier alone runs this mixed-size actual builtin holdout.
func BenchmarkFileConversionHoldout(b *testing.B) { benchPoolRead(b, 32768, true) }
