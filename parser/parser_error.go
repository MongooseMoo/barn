package parser

var errorNames = map[string]struct{}{
	"E_NONE":    {},
	"E_TYPE":    {},
	"E_DIV":     {},
	"E_PERM":    {},
	"E_PROPNF":  {},
	"E_VERBNF":  {},
	"E_VARNF":   {},
	"E_INVIND":  {},
	"E_RECMOVE": {},
	"E_MAXREC":  {},
	"E_RANGE":   {},
	"E_ARGS":    {},
	"E_NACC":    {},
	"E_INVARG":  {},
	"E_QUOTA":   {},
	"E_FLOAT":   {},
	"E_FILE":    {},
	"E_EXEC":    {},
	"E_INTRPT":  {}, // ToastStunt structures.h:73 — enum error{}, code 18 (last element)
}

func isErrorName(name string) bool {
	_, ok := errorNames[name]
	return ok
}
