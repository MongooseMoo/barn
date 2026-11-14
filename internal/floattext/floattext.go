// Package floattext spells a MOO float the way ToastStunt prints one, both as
// a value (tostr, toliteral) and as a literal in printed code (verb_code):
// printf "%.15g", with ".0" added when the result has no point or exponent.
// It is shared by the runtime value type and the formatter, which may not
// depend on each other.
package floattext

import (
	"math"
	"strconv"
	"strings"
)

// Format spells f. NaN and the infinities get the runtime's spellings; they
// have no literal form in code.
func Format(f float64) string {
	if math.IsNaN(f) {
		return "NaN"
	}
	if math.IsInf(f, 1) {
		return "Inf"
	}
	if math.IsInf(f, -1) {
		return "-Inf"
	}
	s := strconv.FormatFloat(f, 'g', 15, 64)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	return s
}
