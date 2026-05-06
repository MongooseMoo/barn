package builtins

import (
	"barn/types"
	"regexp"
	"strings"
)

var ansiEscapeRe = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)
var ansiTagRe = regexp.MustCompile(`\[([^\[\]]+)\]`)

var ansiTags = map[string]string{
	"black":     "\x1b[30m",
	"red":       "\x1b[31m",
	"green":     "\x1b[32m",
	"yellow":    "\x1b[33m",
	"blue":      "\x1b[34m",
	"purple":    "\x1b[35m",
	"magenta":   "\x1b[35m",
	"cyan":      "\x1b[36m",
	"white":     "\x1b[37m",
	"gray":      "\x1b[90m",
	"grey":      "\x1b[90m",
	"b:black":   "\x1b[40m",
	"b:red":     "\x1b[41m",
	"b:green":   "\x1b[42m",
	"b:yellow":  "\x1b[43m",
	"b:blue":    "\x1b[44m",
	"b:purple":  "\x1b[45m",
	"b:magenta": "\x1b[45m",
	"b:cyan":    "\x1b[46m",
	"b:white":   "\x1b[47m",
	"bold":      "\x1b[1m",
	"unbold":    "\x1b[22m",
	"bright":    "\x1b[1m",
	"unbright":  "\x1b[22m",
	"underline": "\x1b[4m",
	"inverse":   "\x1b[7m",
	"blink":     "\x1b[5m",
	"unblink":   "\x1b[25m",
	"normal":    "\x1b[0m",
	"beep":      "\a",
	"random":    "\x1b[37m",
	"null":      "",
}

func builtinParseAnsi(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	s, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	converted := ansiTagRe.ReplaceAllStringFunc(s.Value(), func(tag string) string {
		name := strings.ToLower(tag[1 : len(tag)-1])
		if code, ok := ansiTags[name]; ok {
			return code
		}
		return tag
	})
	return types.Ok(types.NewStr(converted))
}

func builtinRemoveAnsi(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	s, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	strippedTags := ansiTagRe.ReplaceAllStringFunc(s.Value(), func(tag string) string {
		name := strings.ToLower(tag[1 : len(tag)-1])
		if _, ok := ansiTags[name]; ok {
			return ""
		}
		return tag
	})
	return types.Ok(types.NewStr(ansiEscapeRe.ReplaceAllString(strippedTags, "")))
}
