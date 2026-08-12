package verb

import "strings"

// Preposition is a MOO preposition specification. Its numeric value is the
// stable value used by the database wire format.
type Preposition int

const (
	PrepositionAny  Preposition = -2
	PrepositionNone Preposition = -1

	PrepositionWith      Preposition = 0
	PrepositionAt        Preposition = 1
	PrepositionInFrontOf Preposition = 2
	PrepositionIn        Preposition = 3
	PrepositionOn        Preposition = 4
	PrepositionFrom      Preposition = 5
	PrepositionOver      Preposition = 6
	PrepositionThrough   Preposition = 7
	PrepositionUnder     Preposition = 8
	PrepositionBehind    Preposition = 9
	PrepositionBeside    Preposition = 10
	PrepositionFor       Preposition = 11
	PrepositionIs        Preposition = 12
	PrepositionAs        Preposition = 13
	PrepositionOff       Preposition = 14
)

type prepositionDefinition struct {
	canonical string
	aliases   []string
}

// This is the single ordered definition of the MOO preposition table. The
// index of each entry is its database wire code and alias order is significant
// to command parsing.
var prepositionDefinitions = [...]prepositionDefinition{
	{"with/using", []string{"with", "using"}},
	{"at/to", []string{"at", "to"}},
	{"in front of", []string{"in front of"}},
	{"in/inside/into", []string{"in", "inside", "into"}},
	{"on top of/on/onto/upon", []string{"on top of", "on", "onto", "upon"}},
	{"out of/from inside/from", []string{"out of", "from inside", "from"}},
	{"over", []string{"over"}},
	{"through", []string{"through"}},
	{"under/underneath/beneath", []string{"under", "underneath", "beneath"}},
	{"behind", []string{"behind"}},
	{"beside", []string{"beside"}},
	{"for/about", []string{"for", "about"}},
	{"is", []string{"is"}},
	{"as", []string{"as"}},
	{"off/off of", []string{"off", "off of"}},
}

// Prepositions returns the ordinary prepositions in stable wire-code order.
func Prepositions() []Preposition {
	result := make([]Preposition, len(prepositionDefinitions))
	for code := range result {
		result[code] = Preposition(code)
	}
	return result
}

// PrepositionFromCode converts a database wire code to a preposition.
func PrepositionFromCode(code int) (Preposition, bool) {
	prep := Preposition(code)
	return prep, prep == PrepositionNone || prep == PrepositionAny || (code >= 0 && code < len(prepositionDefinitions))
}

// Code returns the stable database wire code.
func (p Preposition) Code() (int, bool) {
	_, ok := PrepositionFromCode(int(p))
	return int(p), ok
}

// Canonical returns the slash-delimited database spelling (or none/any).
func (p Preposition) Canonical() (string, bool) {
	switch p {
	case PrepositionNone:
		return "none", true
	case PrepositionAny:
		return "any", true
	default:
		if p >= 0 && int(p) < len(prepositionDefinitions) {
			return prepositionDefinitions[p].canonical, true
		}
		return "", false
	}
}

// Aliases returns a copy of the ordered aliases for an ordinary preposition.
func (p Preposition) Aliases() []string {
	if p < 0 || int(p) >= len(prepositionDefinitions) {
		return nil
	}
	return append([]string(nil), prepositionDefinitions[p].aliases...)
}

// ParsePrepositionAlias parses none, any, or one ordered preposition alias.
// Matching is case-insensitive.
func ParsePrepositionAlias(value string) (Preposition, bool) {
	value = strings.ToLower(value)
	switch value {
	case "none":
		return PrepositionNone, true
	case "any":
		return PrepositionAny, true
	}
	for code, definition := range prepositionDefinitions {
		for _, alias := range definition.aliases {
			if value == alias {
				return Preposition(code), true
			}
		}
	}
	return PrepositionNone, false
}

// ParsePreposition parses an alias or a canonical database spelling.
func ParsePreposition(value string) (Preposition, bool) {
	if prep, ok := ParsePrepositionAlias(value); ok {
		return prep, true
	}
	value = strings.ToLower(value)
	for code, definition := range prepositionDefinitions {
		if value == definition.canonical {
			return Preposition(code), true
		}
	}
	return PrepositionNone, false
}
