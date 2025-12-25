package types

// ErrValue represents a MOO error value
type ErrValue struct {
	code ErrorCode
}

// NewErr creates a new error value
func NewErr(code ErrorCode) ErrValue {
	return ErrValue{code: code}
}

// String returns the MOO string representation
func (e ErrValue) String() string {
	return e.code.String()
}

// Type returns the MOO type
func (e ErrValue) Type() TypeCode {
	return TYPE_ERR
}

// Truthy returns whether the value is truthy
// In MOO, errors are never truthy (only non-zero ints and non-empty strings)
func (e ErrValue) Truthy() bool {
	return false
}

// Equal compares two values for equality
func (e ErrValue) Equal(other Value) bool {
	if o, ok := other.(ErrValue); ok {
		return e.code == o.code
	}
	return false
}

// Code returns the error code
func (e ErrValue) Code() ErrorCode {
	return e.code
}
