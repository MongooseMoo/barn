package command

import "strings"

type IntrinsicPhase int

const (
	IntrinsicBeforeDoCommand IntrinsicPhase = iota
	IntrinsicAfterVerbMiss
)

type IntrinsicCommand int

const (
	IntrinsicNone IntrinsicCommand = iota
	IntrinsicProgram
	IntrinsicPrefix
	IntrinsicSuffix
	IntrinsicEval
)

func LookupIntrinsic(verb string, phase IntrinsicPhase) IntrinsicCommand {
	switch phase {
	case IntrinsicBeforeDoCommand:
		return lookupBeforeDoCommandIntrinsic(verb)
	case IntrinsicAfterVerbMiss:
		if strings.EqualFold(verb, "eval") {
			return IntrinsicEval
		}
	}
	return IntrinsicNone
}

func lookupBeforeDoCommandIntrinsic(verb string) IntrinsicCommand {
	switch {
	case matchesProgramIntrinsic(verb):
		return IntrinsicProgram
	case strings.EqualFold(verb, "prefix"), strings.EqualFold(verb, "outputprefix"):
		return IntrinsicPrefix
	case strings.EqualFold(verb, "suffix"), strings.EqualFold(verb, "outputsuffix"):
		return IntrinsicSuffix
	default:
		return IntrinsicNone
	}
}

func matchesProgramIntrinsic(verb string) bool {
	verb = strings.ToLower(verb)
	if !strings.HasPrefix(verb, ".pr") {
		return false
	}
	remainder := verb[len(".pr"):]
	return strings.HasPrefix("ogram", remainder)
}
