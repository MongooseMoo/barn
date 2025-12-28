package types

// ObjID represents a MOO object reference
// -1 = nothing, -2 = ambiguous, -3 = failed_match, 0+ = valid object
type ObjID int64

const (
	ObjNothing      ObjID = -1
	ObjAmbiguous    ObjID = -2
	ObjFailedMatch  ObjID = -3
)

// ErrorCode represents a MOO error type (E_TYPE, E_DIV, etc.)
type ErrorCode int

// Error codes - values from spec/errors.md
const (
	E_NONE    ErrorCode = 0
	E_TYPE    ErrorCode = 1
	E_DIV     ErrorCode = 2
	E_PERM    ErrorCode = 3
	E_PROPNF  ErrorCode = 4
	E_VERBNF  ErrorCode = 5
	E_VARNF   ErrorCode = 6
	E_INVIND  ErrorCode = 7
	E_RECMOVE ErrorCode = 8
	E_MAXREC  ErrorCode = 9
	E_RANGE   ErrorCode = 10
	E_ARGS    ErrorCode = 11
	E_NACC    ErrorCode = 12
	E_INVARG  ErrorCode = 13
	E_QUOTA   ErrorCode = 14
	E_FLOAT   ErrorCode = 15
	E_FILE    ErrorCode = 16
	E_EXEC    ErrorCode = 17
)

// ErrorName returns the string name for an error code
func (e ErrorCode) String() string {
	switch e {
	case E_NONE:
		return "E_NONE"
	case E_TYPE:
		return "E_TYPE"
	case E_DIV:
		return "E_DIV"
	case E_PERM:
		return "E_PERM"
	case E_PROPNF:
		return "E_PROPNF"
	case E_VERBNF:
		return "E_VERBNF"
	case E_VARNF:
		return "E_VARNF"
	case E_INVIND:
		return "E_INVIND"
	case E_RECMOVE:
		return "E_RECMOVE"
	case E_MAXREC:
		return "E_MAXREC"
	case E_RANGE:
		return "E_RANGE"
	case E_ARGS:
		return "E_ARGS"
	case E_NACC:
		return "E_NACC"
	case E_INVARG:
		return "E_INVARG"
	case E_QUOTA:
		return "E_QUOTA"
	case E_FLOAT:
		return "E_FLOAT"
	case E_FILE:
		return "E_FILE"
	case E_EXEC:
		return "E_EXEC"
	default:
		return "E_UNKNOWN"
	}
}

// Message returns a human-readable message for an error code
// These match LambdaMOO/ToastStunt error messages
func (e ErrorCode) Message() string {
	switch e {
	case E_NONE:
		return "No error"
	case E_TYPE:
		return "Type mismatch"
	case E_DIV:
		return "Division by zero"
	case E_PERM:
		return "Permission denied"
	case E_PROPNF:
		return "Property not found"
	case E_VERBNF:
		return "Verb not found"
	case E_VARNF:
		return "Variable not found"
	case E_INVIND:
		return "Invalid indirection"
	case E_RECMOVE:
		return "Recursive move"
	case E_MAXREC:
		return "Too many verb calls"
	case E_RANGE:
		return "Range error"
	case E_ARGS:
		return "Incorrect number of arguments"
	case E_NACC:
		return "Move refused by destination"
	case E_INVARG:
		return "Invalid argument"
	case E_QUOTA:
		return "Resource limit exceeded"
	case E_FLOAT:
		return "Floating-point arithmetic error"
	case E_FILE:
		return "File system error"
	case E_EXEC:
		return "Exec error"
	default:
		return "Unknown error"
	}
}

// ErrorFromString converts a string like "E_PERM" to an ErrorCode
func ErrorFromString(s string) (ErrorCode, bool) {
	switch s {
	case "E_NONE":
		return E_NONE, true
	case "E_TYPE":
		return E_TYPE, true
	case "E_DIV":
		return E_DIV, true
	case "E_PERM":
		return E_PERM, true
	case "E_PROPNF":
		return E_PROPNF, true
	case "E_VERBNF":
		return E_VERBNF, true
	case "E_VARNF":
		return E_VARNF, true
	case "E_INVIND":
		return E_INVIND, true
	case "E_RECMOVE":
		return E_RECMOVE, true
	case "E_MAXREC":
		return E_MAXREC, true
	case "E_RANGE":
		return E_RANGE, true
	case "E_ARGS":
		return E_ARGS, true
	case "E_NACC":
		return E_NACC, true
	case "E_INVARG":
		return E_INVARG, true
	case "E_QUOTA":
		return E_QUOTA, true
	case "E_FLOAT":
		return E_FLOAT, true
	case "E_FILE":
		return E_FILE, true
	case "E_EXEC":
		return E_EXEC, true
	default:
		return E_NONE, false
	}
}

// Value is the interface all MOO values implement
type Value interface {
	Type() TypeCode
	String() string   // MOO literal representation
	Equal(Value) bool // Deep equality
	Truthy() bool     // MOO truthiness rules
}
