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

// Format spells f. NaN and the infinities have no literal form in code; as
// values Toast prints them through the same printf (list.cc:461-466), so
// glibc's "inf", "-inf", "nan" and "-nan" get ".0" appended. SQLite results
// are where a MOO value can hold one.
func Format(f float64) string {
	if math.IsNaN(f) {
		if math.Signbit(f) {
			return "-nan.0"
		}
		return "nan.0"
	}
	if math.IsInf(f, 1) {
		return "inf.0"
	}
	if math.IsInf(f, -1) {
		return "-inf.0"
	}
	s := strconv.FormatFloat(f, 'g', 15, 64)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	return s
}
