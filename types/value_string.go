package types

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// String returns the MOO literal representation of the value (the form used by
// toliteral / error tracebacks). Strings are quoted and binary-encoded; other
// scalars match their former per-type String() methods.
func (v Value) String() string {
	switch v.kind {
	case KindNone:
		return "<unbound>"
	case KindInt:
		return strconv.FormatInt(int64(v.num), 10)
	case KindFloat:
		return formatFloat(v.Float())
	case KindStr:
		return quoteMooString(v.str)
	case KindObj:
		return fmt.Sprintf("#%d", int64(v.num))
	case KindAnon:
		return fmt.Sprintf("*#%d", int64(v.num))
	case KindErr:
		return ErrorCode(int64(v.num)).String()
	case KindBool:
		if v.num != 0 {
			return "true"
		}
		return "false"
	case KindList:
		return v.List().String()
	case KindMap:
		return v.Map().String()
	case KindWaif:
		return v.Waif().String()
	default:
		return "<unknown>"
	}
}

// formatFloat renders a MOO float literal (whole numbers keep a trailing .0).
func formatFloat(f float64) string {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, 1):
		return "Inf"
	case math.IsInf(f, -1):
		return "-Inf"
	}
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if !strings.Contains(s, ".") && !strings.Contains(s, "e") && !strings.Contains(s, "E") {
		s += ".0"
	}
	return s
}

// quoteMooString renders the quoted, binary-encoded string literal: non-printable
// bytes become ~XX, with tabs preserved for protocol compatibility.
func quoteMooString(val string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(val); i++ {
		c := val[i]
		switch {
		case c == '"':
			b.WriteString("\\\"")
		case c == '\\':
			b.WriteString("\\\\")
		case c == '\t':
			b.WriteByte(c)
		case c >= 32 && c <= 126:
			b.WriteByte(c)
		default:
			b.WriteString(fmt.Sprintf("~%02X", c))
		}
	}
	b.WriteByte('"')
	return b.String()
}
