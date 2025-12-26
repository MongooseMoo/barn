package types

import (
	"math"
	"strconv"
	"strings"
)

// FloatValue represents a MOO floating point number
type FloatValue struct {
	Val float64
}

// Type returns the type code for floats
func (f FloatValue) Type() TypeCode {
	return TYPE_FLOAT
}

// String returns the MOO literal representation
func (f FloatValue) String() string {
	// Handle special cases
	if math.IsNaN(f.Val) {
		return "NaN"
	}
	if math.IsInf(f.Val, 1) {
		return "Inf"
	}
	if math.IsInf(f.Val, -1) {
		return "-Inf"
	}
	// MOO expects whole numbers to still show decimal (3.0 not 3)
	s := strconv.FormatFloat(f.Val, 'g', -1, 64)
	// Add .0 if no decimal point and not in scientific notation
	if !strings.Contains(s, ".") && !strings.Contains(s, "e") && !strings.Contains(s, "E") {
		s += ".0"
	}
	return s
}

// Equal checks deep equality
func (f FloatValue) Equal(other Value) bool {
	if other == nil {
		return false
	}
	if other.Type() != TYPE_FLOAT {
		return false
	}
	otherFloat, ok := other.(FloatValue)
	if !ok {
		return false
	}
	// NaN != NaN in MOO (IEEE 754 semantics)
	if math.IsNaN(f.Val) || math.IsNaN(otherFloat.Val) {
		return false
	}
	return f.Val == otherFloat.Val
}

// Truthy returns the MOO truthiness
// In MOO, floats are never truthy (only non-zero ints and non-empty strings)
func (f FloatValue) Truthy() bool {
	return false
}

// NewFloat creates a new FloatValue
func NewFloat(val float64) FloatValue {
	return FloatValue{Val: val}
}

// IsNaN returns true if the float is NaN
func (f FloatValue) IsNaN() bool {
	return math.IsNaN(f.Val)
}

// IsInf returns true if the float is infinite
func (f FloatValue) IsInf() bool {
	return math.IsInf(f.Val, 0)
}
