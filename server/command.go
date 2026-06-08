package server

import (
	"barn/types"
	"strings"
	"unicode"
)

// PrepSpec represents a preposition specification
type PrepSpec int

const (
	PrepWith      PrepSpec = 0  // with/using
	PrepAt        PrepSpec = 1  // at/to
	PrepInFrontOf PrepSpec = 2  // in front of
	PrepIn        PrepSpec = 3  // in/inside/into
	PrepOn        PrepSpec = 4  // on top of/on/onto/upon
	PrepFrom      PrepSpec = 5  // out of/from inside/from
	PrepOver      PrepSpec = 6  // over
	PrepThrough   PrepSpec = 7  // through
	PrepUnder     PrepSpec = 8  // under/underneath/beneath
	PrepBehind    PrepSpec = 9  // behind
	PrepBeside    PrepSpec = 10 // beside
	PrepFor       PrepSpec = 11 // for/about
	PrepIs        PrepSpec = 12 // is
	PrepAs        PrepSpec = 13 // as
	PrepOff       PrepSpec = 14 // off/off of

	PrepNone PrepSpec = -1 // No preposition found
	PrepAny  PrepSpec = -2 // Matches any preposition (for verb definitions)
)

// Preposition aliases - index matches PrepSpec values
var prepositions = [][]string{
	{"with", "using"},                   // 0 - PrepWith
	{"at", "to"},                        // 1 - PrepAt
	{"in front of"},                     // 2 - PrepInFrontOf
	{"in", "inside", "into"},            // 3 - PrepIn
	{"on top of", "on", "onto", "upon"}, // 4 - PrepOn
	{"out of", "from inside", "from"},   // 5 - PrepFrom
	{"over"},                            // 6 - PrepOver
	{"through"},                         // 7 - PrepThrough
	{"under", "underneath", "beneath"},  // 8 - PrepUnder
	{"behind"},                          // 9 - PrepBehind
	{"beside"},                          // 10 - PrepBeside
	{"for", "about"},                    // 11 - PrepFor
	{"is"},                              // 12 - PrepIs
	{"as"},                              // 13 - PrepAs
	{"off", "off of"},                   // 14 - PrepOff
}

// ParsedCommand is the structured representation of a parsed player command
type ParsedCommand struct {
	Verb    string
	Argstr  string
	Args    []string
	Words   []string
	Dobjstr string
	Dobj    types.ObjID
	Prepstr string
	Prep    PrepSpec
	Iobjstr string
	Iobj    types.ObjID
}

// NewParsedCommand creates an empty parsed command
func NewParsedCommand() *ParsedCommand {
	return &ParsedCommand{
		Dobj: types.ObjNothing,
		Prep: PrepNone,
		Iobj: types.ObjNothing,
	}
}

// findPreposition finds a preposition in the word list
// Returns (PrepSpec, startIndex, endIndex, prepstr) or (PrepNone, -1, -1, "")
func findPreposition(words []string) (PrepSpec, int, int, string) {
	for i := range words {
		for prepIdx, aliases := range prepositions {
			for _, alias := range aliases {
				aliasWords := strings.Fields(alias)
				if len(aliasWords) == 0 || i+len(aliasWords) > len(words) {
					continue
				}
				match := true
				for j, aliasWord := range aliasWords {
					if strings.ToLower(words[i+j]) != aliasWord {
						match = false
						break
					}
				}
				if match {
					return PrepSpec(prepIdx), i, i + len(aliasWords), alias
				}
			}
		}
	}

	return PrepNone, -1, -1, ""
}

func tokenizeCommandWords(input string) []string {
	words := make([]string, 0)
	var current strings.Builder
	inQuotes := false
	i := 0
	for i < len(input) {
		ch := input[i]
		if ch == '\\' {
			if i+1 < len(input) {
				current.WriteByte(input[i+1])
				i += 2
			} else {
				current.WriteByte(ch)
				i++
			}
			continue
		}
		if ch == '"' {
			inQuotes = !inQuotes
			i++
			continue
		}
		if unicode.IsSpace(rune(ch)) && !inQuotes {
			if current.Len() > 0 {
				words = append(words, current.String())
				current.Reset()
			}
			i++
			continue
		}
		current.WriteByte(ch)
		i++
	}
	if current.Len() > 0 {
		words = append(words, current.String())
	}
	return words
}

func commandWordList(input string) []string {
	input = strings.TrimLeftFunc(input, unicode.IsSpace)
	if input == "" {
		return nil
	}
	switch input[0] {
	case '"', ':', ';':
		words := []string{input[:1]}
		words = append(words, tokenizeCommandWords(input[1:])...)
		return words
	default:
		return tokenizeCommandWords(input)
	}
}

// ParseCommand parses player input into a structured command
func ParseCommand(input string) *ParsedCommand {
	return parseCommand(input, commandWordList(input))
}

func parseCommand(input string, originalWords []string) *ParsedCommand {
	cmd := NewParsedCommand()
	cmd.Words = originalWords

	// Handle empty input
	input = strings.TrimLeftFunc(input, unicode.IsSpace)
	if input == "" {
		return cmd
	}

	// Handle special prefixes
	if strings.HasPrefix(input, "\"") {
		return parseCommand("say "+input[1:], originalWords)
	}

	if strings.HasPrefix(input, ":") {
		return parseCommand("emote "+input[1:], originalWords)
	}

	if strings.HasPrefix(input, ";") {
		return parseCommand("eval "+input[1:], originalWords)
	}

	// Tokenize - normalize whitespace while preserving quoted multiword tokens.
	words := tokenizeCommandWords(input)
	if len(words) == 0 {
		return cmd
	}

	// First word is the verb
	cmd.Verb = words[0]

	if len(words) == 1 {
		return cmd
	}

	// Rest are arguments
	restWords := words[1:]
	cmd.Args = restWords

	verbEnd := len(words[0])
	for verbEnd < len(input) && !unicode.IsSpace(rune(input[verbEnd])) {
		verbEnd++
	}
	argStart := verbEnd
	for argStart < len(input) && unicode.IsSpace(rune(input[argStart])) {
		argStart++
	}
	cmd.Argstr = input[argStart:]

	// Find preposition in the argument words
	prep, prepStart, prepEnd, prepstr := findPreposition(restWords)

	if prep == PrepNone {
		// No preposition - everything is the direct object. Build it from the
		// tokenized words (quotes stripped, whitespace normalized) so quoted
		// multiword names like `grab "red ball"` match an object named
		// "red ball", consistent with the preposition branch below and Toast.
		cmd.Dobjstr = strings.Join(restWords, " ")
	} else {
		cmd.Prep = prep
		cmd.Prepstr = prepstr

		// Words before preposition are direct object
		if prepStart > 0 {
			cmd.Dobjstr = strings.Join(restWords[:prepStart], " ")
		}

		// Words after preposition are indirect object
		if prepEnd < len(restWords) {
			cmd.Iobjstr = strings.Join(restWords[prepEnd:], " ")
		}
	}

	return cmd
}
