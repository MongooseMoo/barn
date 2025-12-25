package types

// TypeCode represents MOO type values (from spec/types.md)
type TypeCode int

const (
	TYPE_INT   TypeCode = 0
	TYPE_OBJ   TypeCode = 1
	TYPE_STR   TypeCode = 2
	TYPE_ERR   TypeCode = 3
	TYPE_LIST  TypeCode = 4
	TYPE_FLOAT TypeCode = 9
	TYPE_MAP   TypeCode = 10
	TYPE_WAIF  TypeCode = 13
	TYPE_BOOL  TypeCode = 14
)

// String returns the string representation of the type code
func (t TypeCode) String() string {
	switch t {
	case TYPE_INT:
		return "INT"
	case TYPE_OBJ:
		return "OBJ"
	case TYPE_STR:
		return "STR"
	case TYPE_ERR:
		return "ERR"
	case TYPE_LIST:
		return "LIST"
	case TYPE_FLOAT:
		return "FLOAT"
	case TYPE_MAP:
		return "MAP"
	case TYPE_WAIF:
		return "WAIF"
	case TYPE_BOOL:
		return "BOOL"
	default:
		return "UNKNOWN"
	}
}
