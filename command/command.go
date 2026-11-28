package command

import (
	"slices"
	"strings"
	"unicode"

	"github.com/MongooseMoo/barn/metrics"
	"github.com/MongooseMoo/barn/preposition"
	"github.com/MongooseMoo/barn/types"
)

// ParsedCommand is the structured representation of a parsed player command
type ParsedCommand struct {
	Diagnostic *metrics.Command
	Verb       string
	Argstr     string
	Args       []string
	Words      []string
	Dobjstr    string
	Dobj       types.ObjID
	Prepstr    string
	Prep       preposition.Spec
	Iobjstr    string
	Iobj       types.ObjID
}

// NewParsedCommand creates an empty parsed command
func NewParsedCommand() *ParsedCommand {
	return &ParsedCommand{
		Dobj: types.ObjNothing,
		Prep: preposition.None,
		Iobj: types.ObjNothing,
	}
}

type prepositionAlias struct {
	prep      preposition.Spec
	alias     string
	remaining []string
}

// Index by first word, retaining table order for aliases with the same start.
var prepositionAliases = func() map[string][]prepositionAlias {
	index := make(map[string][]prepositionAlias)
	for _, prep := range preposition.All() {
		for _, alias := range prep.Aliases() {
			words := strings.Fields(alias)
			if len(words) != 0 {
				index[words[0]] = append(index[words[0]], prepositionAlias{prep, alias, words[1:]})
			}
		}
	}
	return index
}()

// findPreposition finds a preposition in the word list
// Returns the preposition, start and end indexes, and matched alias; none uses indexes -1.
func findPreposition(words []string) (preposition.Spec, int, int, string) {
	for i, word := range words {
		for _, candidate := range prepositionAliases[strings.ToLower(word)] {
			end := i + 1 + len(candidate.remaining)
			if end > len(words) {
				continue
			}
			match := true
			for j, aliasWord := range candidate.remaining {
				if strings.ToLower(words[i+1+j]) != aliasWord {
					match = false
					break
				}
			}
			if match {
				return candidate.prep, i, end, candidate.alias
			}
		}
	}

	return preposition.None, -1, -1, ""
}

func tokenizeCommandWords(input string) []string {
	var shortCommand [9]string
	words := shortCommand[:0]
	var current strings.Builder
	start := 0
	transformed := false
	inQuotes := false
	i := 0
	for i < len(input) {
		ch := input[i]
		if !transformed && (ch == '\\' || ch == '"') {
			current.WriteString(input[start:i])
			transformed = true
		}
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
			if transformed {
				if current.Len() > 0 {
					words = append(words, current.String())
				}
				current.Reset()
				transformed = false
			} else if start < i {
				words = append(words, input[start:i])
			}
			i++
			start = i
			continue
		}
		if transformed {
			current.WriteByte(ch)
		}
		i++
	}
	if transformed {
		if current.Len() > 0 {
			words = append(words, current.String())
		}
	} else if start < i {
		words = append(words, input[start:i])
	}
	return append(make([]string, 0, len(words)), words...)
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
	originalWords := CommandWordList(input)
	cmd := NewParsedCommand()
	cmd.Words = originalWords

	if len(originalWords) == 0 {
		return cmd
	}
	input = strings.TrimLeftFunc(input, unicode.IsSpace)

	cmd.Verb = originalWords[0]
	verbEnd := 1
	switch input[0] {
	case '"':
		cmd.Verb = "say"
	case ':':
		cmd.Verb = "emote"
	case ';':
		cmd.Verb = "eval"
	default:
		verbEnd = len(cmd.Verb)
		for verbEnd < len(input) && !unicode.IsSpace(rune(input[verbEnd])) {
			verbEnd++
		}
	}

	if len(originalWords) == 1 {
		return cmd
	}

	// Rest are arguments
	restWords := slices.Clone(originalWords[1:])
	cmd.Args = restWords

	argStart := verbEnd
	for argStart < len(input) && unicode.IsSpace(rune(input[argStart])) {
		argStart++
	}
	cmd.Argstr = input[argStart:]

	// Find preposition in the argument words
	prep, prepStart, prepEnd, prepstr := findPreposition(restWords)

	if prep == preposition.None {
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
