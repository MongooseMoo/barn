package server

import (
	"barn/types"
	"strings"
)

// PrepSpec represents a preposition specification
type PrepSpec int

const (
	PrepWith       PrepSpec = 0  // with/using
	PrepAt         PrepSpec = 1  // at/to
	PrepInFrontOf  PrepSpec = 2  // in front of
	PrepIn         PrepSpec = 3  // in/inside/into
	PrepOn         PrepSpec = 4  // on top of/on/onto/upon
	PrepFrom       PrepSpec = 5  // out of/from inside/from
	PrepOver       PrepSpec = 6  // over
	PrepThrough    PrepSpec = 7  // through
	PrepUnder      PrepSpec = 8  // under/underneath/beneath
	PrepBehind     PrepSpec = 9  // behind
	PrepBeside     PrepSpec = 10 // beside
	PrepFor        PrepSpec = 11 // for/about
	PrepIs         PrepSpec = 12 // is
	PrepAs         PrepSpec = 13 // as
	PrepOff        PrepSpec = 14 // off/off of

	PrepNone PrepSpec = -1 // No preposition found
	PrepAny  PrepSpec = -2 // Matches any preposition (for verb definitions)
)

// Preposition aliases - index matches PrepSpec values
var prepositions = [][]string{
	{"with", "using"},                          // 0 - PrepWith
	{"at", "to"},                               // 1 - PrepAt
	{"in front of"},                            // 2 - PrepInFrontOf
	{"in", "inside", "into"},                   // 3 - PrepIn
	{"on top of", "on", "onto", "upon"},        // 4 - PrepOn
	{"out of", "from inside", "from"},          // 5 - PrepFrom
	{"over"},                                   // 6 - PrepOver
	{"through"},                                // 7 - PrepThrough
	{"under", "underneath", "beneath"},         // 8 - PrepUnder
	{"behind"},                                 // 9 - PrepBehind
	{"beside"},                                 // 10 - PrepBeside
	{"for", "about"},                           // 11 - PrepFor
	{"is"},                                     // 12 - PrepIs
	{"as"},                                     // 13 - PrepAs
	{"off", "off of"},                          // 14 - PrepOff
}

// ParsedCommand is the structured representation of a parsed player command
type ParsedCommand struct {
	Verb    string
	Argstr  string
	Args    []string
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
	// Check for multi-word prepositions first (longest to shortest)
	for prepIdx, aliases := range prepositions {
		for _, alias := range aliases {
			aliasWords := strings.Fields(alias)
			aliasLen := len(aliasWords)
			if aliasLen > 1 {
				// Multi-word preposition - scan through words
				for i := 0; i <= len(words)-aliasLen; i++ {
					match := true
					for j := 0; j < aliasLen; j++ {
						if strings.ToLower(words[i+j]) != aliasWords[j] {
							match = false
							break
						}
					}
					if match {
						return PrepSpec(prepIdx), i, i + aliasLen, alias
					}
				}
			}
		}
	}

	// Check for single-word prepositions
	for i, word := range words {
		wordLower := strings.ToLower(word)
		for prepIdx, aliases := range prepositions {
			for _, alias := range aliases {
				if wordLower == alias {
					return PrepSpec(prepIdx), i, i + 1, wordLower
				}
			}
		}
	}

	return PrepNone, -1, -1, ""
}

// ParseCommand parses player input into a structured command
func ParseCommand(input string) *ParsedCommand {
	cmd := NewParsedCommand()

	// Handle empty input
	input = strings.TrimSpace(input)
	if input == "" {
		return cmd
	}

	// Handle special prefixes
	if strings.HasPrefix(input, "\"") {
		cmd.Verb = "say"
		cmd.Argstr = input[1:]
		if cmd.Argstr != "" {
			cmd.Args = strings.Fields(cmd.Argstr)
		}
		return cmd
	}

	if strings.HasPrefix(input, ":") {
		cmd.Verb = "emote"
		cmd.Argstr = input[1:]
		if cmd.Argstr != "" {
			cmd.Args = strings.Fields(cmd.Argstr)
		}
		return cmd
	}

	if strings.HasPrefix(input, ";") {
		cmd.Verb = "eval"
		cmd.Argstr = input[1:]
		if cmd.Argstr != "" {
			cmd.Args = strings.Fields(cmd.Argstr)
		}
		return cmd
	}

	// Tokenize - normalize whitespace
	words := strings.Fields(input)
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
	cmd.Argstr = strings.Join(restWords, " ")

	// Find preposition in the argument words
	prep, prepStart, prepEnd, prepstr := findPreposition(restWords)

	if prep == PrepNone {
		// No preposition - everything is direct object
		cmd.Dobjstr = cmd.Argstr
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
