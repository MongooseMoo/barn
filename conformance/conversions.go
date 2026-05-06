package conformance

import (
	"barn/types"
	"fmt"
	"strings"
)

// errorNameToCode converts error name to ErrorCode
func errorNameToCode(name string) (types.ErrorCode, bool) {
	switch strings.ToUpper(name) {
	case "E_NONE":
		return types.E_NONE, true
	case "E_TYPE":
		return types.E_TYPE, true
	case "E_DIV":
		return types.E_DIV, true
	case "E_PERM":
		return types.E_PERM, true
	case "E_PROPNF":
		return types.E_PROPNF, true
	case "E_VERBNF":
		return types.E_VERBNF, true
	case "E_VARNF":
		return types.E_VARNF, true
	case "E_INVIND":
		return types.E_INVIND, true
	case "E_RECMOVE":
		return types.E_RECMOVE, true
	case "E_MAXREC":
		return types.E_MAXREC, true
	case "E_RANGE":
		return types.E_RANGE, true
	case "E_ARGS":
		return types.E_ARGS, true
	case "E_NACC":
		return types.E_NACC, true
	case "E_INVARG":
		return types.E_INVARG, true
	case "E_QUOTA":
		return types.E_QUOTA, true
	case "E_FLOAT":
		return types.E_FLOAT, true
	case "E_FILE":
		return types.E_FILE, true
	case "E_EXEC":
		return types.E_EXEC, true
	default:
		return 0, false
	}
}

// errorCodeToName converts ErrorCode to name
func errorCodeToName(code types.ErrorCode) string {
	switch code {
	case types.E_NONE:
		return "E_NONE"
	case types.E_TYPE:
		return "E_TYPE"
	case types.E_DIV:
		return "E_DIV"
	case types.E_PERM:
		return "E_PERM"
	case types.E_PROPNF:
		return "E_PROPNF"
	case types.E_VERBNF:
		return "E_VERBNF"
	case types.E_VARNF:
		return "E_VARNF"
	case types.E_INVIND:
		return "E_INVIND"
	case types.E_RECMOVE:
		return "E_RECMOVE"
	case types.E_MAXREC:
		return "E_MAXREC"
	case types.E_RANGE:
		return "E_RANGE"
	case types.E_ARGS:
		return "E_ARGS"
	case types.E_NACC:
		return "E_NACC"
	case types.E_INVARG:
		return "E_INVARG"
	case types.E_QUOTA:
		return "E_QUOTA"
	case types.E_FLOAT:
		return "E_FLOAT"
	case types.E_FILE:
		return "E_FILE"
	case types.E_EXEC:
		return "E_EXEC"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", code)
	}
}

// typeNameToCode converts type name to TypeCode
func typeNameToCode(name string) (types.TypeCode, bool) {
	switch strings.ToLower(name) {
	case "int":
		return types.TYPE_INT, true
	case "obj":
		return types.TYPE_OBJ, true
	case "str":
		return types.TYPE_STR, true
	case "err":
		return types.TYPE_ERR, true
	case "list":
		return types.TYPE_LIST, true
	case "float":
		return types.TYPE_FLOAT, true
	case "map":
		return types.TYPE_MAP, true
	case "anon":
		return types.TYPE_ANON, true
	case "waif":
		return types.TYPE_WAIF, true
	case "bool":
		return types.TYPE_BOOL, true
	default:
		return 0, false
	}
}

// typeCodeToName converts TypeCode to name
func typeCodeToName(code types.TypeCode) string {
	switch code {
	case types.TYPE_INT:
		return "int"
	case types.TYPE_OBJ:
		return "obj"
	case types.TYPE_STR:
		return "str"
	case types.TYPE_ERR:
		return "err"
	case types.TYPE_LIST:
		return "list"
	case types.TYPE_FLOAT:
		return "float"
	case types.TYPE_MAP:
		return "map"
	case types.TYPE_ANON:
		return "anon"
	case types.TYPE_WAIF:
		return "waif"
	case types.TYPE_BOOL:
		return "bool"
	default:
		return fmt.Sprintf("unknown(%d)", code)
	}
}
