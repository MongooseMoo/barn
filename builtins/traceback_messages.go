package builtins

import (
	"barn/types"
	"fmt"
	"strings"
)

type builtinArgKind int

const (
	builtinArgAny builtinArgKind = iota
	builtinArgNumeric
	builtinArgInt
	builtinArgObj
	builtinArgStr
	builtinArgErr
	builtinArgList
	builtinArgFloat
	builtinArgMap
	builtinArgAnon
	builtinArgWaif
	builtinArgBool
)

type builtinSignature struct {
	MinArgs  int
	MaxArgs  int
	ArgTypes []builtinArgKind
}

func hasDetailedExceptionValue(v types.Value) bool {
	switch t := v.(type) {
	case types.StrValue:
		return strings.TrimSpace(t.Value()) != ""
	case types.ListValue:
		// Structured raise payload is typically {code, message, value, traceback}.
		if t.Len() >= 2 {
			if msg, ok := t.Get(2).(types.StrValue); ok {
				return strings.TrimSpace(msg.Value()) != ""
			}
		}
		return t.Len() > 0
	default:
		return v != nil
	}
}

func enrichBuiltinException(name string, args []types.Value, result types.Result) types.Result {
	if result.Flow != types.FlowException {
		return result
	}
	if hasDetailedExceptionValue(result.Val) {
		return result
	}

	sig, hasSig := lookupBuiltinSignature(name)

	switch result.Error {
	case types.E_ARGS:
		result.Val = types.NewStr(formatBuiltinArgsMessage(sig, hasSig, len(args)))
	case types.E_TYPE:
		if hasSig {
			if msg, ok := formatBuiltinTypeMessage(name, sig, args); ok {
				result.Val = types.NewStr(msg)
			}
		}
	}
	if hasDetailedExceptionValue(result.Val) {
		return result
	}

	// For all remaining bare E_* builtin failures, attach contextual detail.
	result.Val = types.NewStr(formatBuiltinGenericMessage(name, args, result.Error))

	return result
}

func formatBuiltinArgsMessage(sig builtinSignature, hasSig bool, got int) string {
	base := types.E_ARGS.Message()
	if !hasSig {
		return fmt.Sprintf("%s (got %d)", base, got)
	}
	if sig.MaxArgs < 0 {
		return fmt.Sprintf("%s (expected at least %d; got %d)", base, sig.MinArgs, got)
	}
	if sig.MinArgs == sig.MaxArgs {
		return fmt.Sprintf("%s (expected %d; got %d)", base, sig.MinArgs, got)
	}
	return fmt.Sprintf("%s (expected %d-%d; got %d)", base, sig.MinArgs, sig.MaxArgs, got)
}

func formatBuiltinTypeMessage(name string, sig builtinSignature, args []types.Value) (string, bool) {
	if len(args) == 0 || len(sig.ArgTypes) == 0 {
		return "", false
	}

	// Toast checks only the fixed prefix for variadics; otherwise all provided args.
	checkCount := len(args)
	if sig.MaxArgs < 0 {
		checkCount = sig.MinArgs
	}
	if checkCount > len(args) {
		checkCount = len(args)
	}
	if checkCount > len(sig.ArgTypes) {
		checkCount = len(sig.ArgTypes)
	}

	for i := 0; i < checkCount; i++ {
		expected := sig.ArgTypes[i]
		if matchesBuiltinArgKind(expected, args[i]) {
			continue
		}
		return fmt.Sprintf(
			"%s (args[%d] of %s() expected %s; got %s)",
			types.E_TYPE.Message(),
			i+1,
			name,
			builtinArgKindName(expected),
			builtinValueTypeName(args[i]),
		), true
	}

	return "", false
}

func formatBuiltinGenericMessage(name string, args []types.Value, code types.ErrorCode) string {
	base := code.Message()
	if strings.TrimSpace(name) == "" {
		return base
	}
	if len(args) == 0 {
		return fmt.Sprintf("%s (in %s())", base, name)
	}
	return fmt.Sprintf("%s (in %s(); %s)", base, name, summarizeBuiltinArgs(args))
}

func summarizeBuiltinArgs(args []types.Value) string {
	const maxArgs = 3
	limit := len(args)
	if limit > maxArgs {
		limit = maxArgs
	}

	parts := make([]string, 0, limit+1)
	for i := 0; i < limit; i++ {
		parts = append(parts, fmt.Sprintf("args[%d]=%s", i+1, summarizeBuiltinValue(args[i])))
	}
	if len(args) > limit {
		parts = append(parts, fmt.Sprintf("+%d more", len(args)-limit))
	}
	return strings.Join(parts, ", ")
}

func summarizeBuiltinValue(v types.Value) string {
	const maxLen = 80
	raw := strings.ReplaceAll(v.String(), "\r", " ")
	raw = strings.ReplaceAll(raw, "\n", " ")
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "<empty>"
	}
	if len(raw) <= maxLen {
		return raw
	}
	return raw[:maxLen-3] + "..."
}

func matchesBuiltinArgKind(kind builtinArgKind, v types.Value) bool {
	switch kind {
	case builtinArgAny:
		return true
	case builtinArgNumeric:
		t := v.Type()
		return t == types.TYPE_INT || t == types.TYPE_FLOAT
	case builtinArgInt:
		return v.Type() == types.TYPE_INT
	case builtinArgObj:
		return v.Type() == types.TYPE_OBJ
	case builtinArgStr:
		return v.Type() == types.TYPE_STR
	case builtinArgErr:
		return v.Type() == types.TYPE_ERR
	case builtinArgList:
		return v.Type() == types.TYPE_LIST
	case builtinArgFloat:
		return v.Type() == types.TYPE_FLOAT
	case builtinArgMap:
		return v.Type() == types.TYPE_MAP
	case builtinArgAnon:
		return v.Type() == types.TYPE_ANON
	case builtinArgWaif:
		return v.Type() == types.TYPE_WAIF
	case builtinArgBool:
		return v.Type() == types.TYPE_BOOL
	default:
		return false
	}
}

func builtinArgKindName(kind builtinArgKind) string {
	switch kind {
	case builtinArgAny:
		return "any type"
	case builtinArgNumeric:
		return "number"
	case builtinArgInt:
		return "integer"
	case builtinArgObj:
		return "object"
	case builtinArgStr:
		return "string"
	case builtinArgErr:
		return "error"
	case builtinArgList:
		return "list"
	case builtinArgFloat:
		return "float"
	case builtinArgMap:
		return "map"
	case builtinArgAnon:
		return "anonymous object"
	case builtinArgWaif:
		return "waif"
	case builtinArgBool:
		return "bool"
	default:
		return "unknown type"
	}
}

func builtinValueTypeName(v types.Value) string {
	switch v.Type() {
	case types.TYPE_INT:
		return "integer"
	case types.TYPE_OBJ:
		return "object"
	case types.TYPE_STR:
		return "string"
	case types.TYPE_ERR:
		return "error"
	case types.TYPE_LIST:
		return "list"
	case types.TYPE_FLOAT:
		return "float"
	case types.TYPE_MAP:
		return "map"
	case types.TYPE_ANON:
		return "anonymous object"
	case types.TYPE_WAIF:
		return "waif"
	case types.TYPE_BOOL:
		return "bool"
	default:
		return "unknown type"
	}
}

func lookupBuiltinSignature(name string) (builtinSignature, bool) {
	if sig, ok := toastBuiltinSignatures[name]; ok {
		return sig, true
	}
	if sig, ok := barnBuiltinSignatures[name]; ok {
		return sig, true
	}
	return builtinSignature{}, false
}
