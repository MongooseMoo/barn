package command

import (
	"strings"
	"unicode"

	"github.com/MongooseMoo/barn/types"
	"github.com/MongooseMoo/barn/verb"
)

// PrepSpec represents a preposition specification
type PrepSpec = verb.Preposition

const (
	PrepWith      = verb.PrepositionWith
	PrepAt        = verb.PrepositionAt
	PrepInFrontOf = verb.PrepositionInFrontOf
	PrepIn        = verb.PrepositionIn
	PrepOn        = verb.PrepositionOn
	PrepFrom      = verb.PrepositionFrom
	PrepOver      = verb.PrepositionOver
	PrepThrough   = verb.PrepositionThrough
	PrepUnder     = verb.PrepositionUnder
	PrepBehind    = verb.PrepositionBehind
	PrepBeside    = verb.PrepositionBeside
	PrepFor       = verb.PrepositionFor
	PrepIs        = verb.PrepositionIs
	PrepAs        = verb.PrepositionAs
	PrepOff       = verb.PrepositionOff

	PrepNone = verb.PrepositionNone
	PrepAny  = verb.PrepositionAny
)

func PrepSpecForAlias(alias string) (PrepSpec, bool) {
	prep, ok := verb.ParsePrepositionAlias(alias)
	return prep, ok && prep >= 0
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
		for _, prep := range verb.Prepositions() {
			aliases := prep.Aliases()
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
					return prep, i, i + len(aliasWords), alias
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

func CommandWordList(input string) []string {
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
	return parseCommand(input, CommandWordList(input))
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
