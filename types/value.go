package types

// Value is the interface all MOO values implement
type Value interface {
	Type() TypeCode
	String() string   // MOO literal representation
	Equal(Value) bool // Deep equality
	Truthy() bool     // MOO truthiness rules
}
