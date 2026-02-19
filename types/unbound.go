package types

// UnboundValue is an internal marker for declared-but-unbound locals.
// VM variable reads convert this marker to E_VARNF.
type UnboundValue struct{}

func (v UnboundValue) Type() TypeCode {
	// Internal-only marker; type is not externally observable.
	return TYPE_INT
}

func (v UnboundValue) String() string {
	return "<unbound>"
}

func (v UnboundValue) Equal(other Value) bool {
	_, ok := other.(UnboundValue)
	return ok
}

func (v UnboundValue) Truthy() bool {
	return false
}
