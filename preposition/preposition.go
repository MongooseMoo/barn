package preposition

import "strings"

// Spec is a MOO preposition specification. Its numeric value is the
// stable value used by the database wire format.
type Spec int

const (
	Any  Spec = -2
	None Spec = -1

	With      Spec = 0
	At        Spec = 1
	InFrontOf Spec = 2
	In        Spec = 3
	On        Spec = 4
	From      Spec = 5
	Over      Spec = 6
	Through   Spec = 7
	Under     Spec = 8
	Behind    Spec = 9
	Beside    Spec = 10
	For       Spec = 11
	Is        Spec = 12
	As        Spec = 13
	Off       Spec = 14
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

// All returns the ordinary prepositions in stable wire-code order.
func All() []Spec {
	result := make([]Spec, len(prepositionDefinitions))
	for code := range result {
		result[code] = Spec(code)
	}
	return result
}

// FromCode converts a database wire code to a preposition.
func FromCode(code int) (Spec, bool) {
	prep := Spec(code)
	return prep, prep == None || prep == Any || (code >= 0 && code < len(prepositionDefinitions))
}

// Code returns the stable database wire code.
func (p Spec) Code() (int, bool) {
	_, ok := FromCode(int(p))
	return int(p), ok
}

// Canonical returns the slash-delimited database spelling (or none/any).
func (p Spec) Canonical() (string, bool) {
	switch p {
	case None:
		return "none", true
	case Any:
		return "any", true
	default:
		if p >= 0 && int(p) < len(prepositionDefinitions) {
			return prepositionDefinitions[p].canonical, true
		}
		return "", false
	}
}

// Aliases returns a copy of the ordered aliases for an ordinary preposition.
func (p Spec) Aliases() []string {
	if p < 0 || int(p) >= len(prepositionDefinitions) {
		return nil
	}
	return append([]string(nil), prepositionDefinitions[p].aliases...)
}

// ParseAlias parses none, any, or one ordered preposition alias.
// Matching is case-insensitive.
func ParseAlias(value string) (Spec, bool) {
	value = strings.ToLower(value)
	switch value {
	case "none":
		return None, true
	case "any":
		return Any, true
	}
	for code, definition := range prepositionDefinitions {
		for _, alias := range definition.aliases {
			if value == alias {
				return Spec(code), true
			}
		}
	}
	return None, false
}

// Parse parses an alias or a canonical database spelling.
func Parse(value string) (Spec, bool) {
	if prep, ok := ParseAlias(value); ok {
		return prep, true
	}
	value = strings.ToLower(value)
	for code, definition := range prepositionDefinitions {
		if value == definition.canonical {
			return Spec(code), true
		}
	}
	return None, false
}
