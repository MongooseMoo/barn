package builtins

import (
	"errors"
	"strconv"
	"strings"
	"unicode/utf8"
)

// errPCREVariableLookbehind is the compile failure Toast's PCRE2 reports for a
// lookbehind branch that does not match a fixed number of characters.
var errPCREVariableLookbehind = errors.New("lookbehind assertion is not fixed length")

// pcreMaxLookbehind is PCRE2 10.42's LOOKBEHIND_MAX: a longer fixed-length
// branch fails with "lookbehind is too complicated".
const pcreMaxLookbehind = 65535

// checkPCRELookbehinds applies PCRE2 10.42's lookbehind rule, which Toast
// links (Debian libpcre2-8 10.42): every top-level branch of a lookbehind must
// match a fixed number of characters, though branches may differ from each
// other (pcre2_compile.c get_branchlength). regexp2 follows .NET and accepts
// any lookbehind, re-running an unbounded one backwards from every subject
// position, so Barn accepted patterns Toast rejects with E_INVARG and could
// spend seconds per call on them. Constructs this scanner does not recognize
// are left for regexp2 to judge.
func checkPCRELookbehinds(pattern string) error {
	// The first pass only records capture group widths, so a back reference
	// inside a lookbehind can use the width of a group defined after it.
	sizer := &pcreWidthScanner{p: pattern, names: map[string]int{}}
	sizer.scan()
	checker := &pcreWidthScanner{p: pattern, names: sizer.names, groupLens: sizer.groupLens, check: true}
	checker.scan()
	return checker.err
}

// pcreWidthScanner computes the character width of pattern pieces: a
// non-negative count, or -1 when the piece can match strings of different
// lengths.
type pcreWidthScanner struct {
	p         string
	i         int
	extended  bool
	check     bool
	err       error
	ncap      int
	groupLens map[int]int
	names     map[string]int
}

func (s *pcreWidthScanner) scan() {
	if s.groupLens == nil {
		s.groupLens = map[int]int{}
	}
	for s.i < len(s.p) && s.err == nil {
		s.alternation()
		if s.i < len(s.p) && s.p[s.i] == ')' {
			s.i++ // unbalanced; regexp2 reports it
		}
	}
}

// alternation parses branches up to an unconsumed ')' or the end and returns
// each branch's width.
func (s *pcreWidthScanner) alternation() []int {
	widths := []int{s.sequence()}
	for s.i < len(s.p) && s.p[s.i] == '|' && s.err == nil {
		s.i++
		widths = append(widths, s.sequence())
	}
	return widths
}

// groupWidth is the width of a group whose branches must all agree.
func groupWidth(widths []int) int {
	for _, w := range widths[1:] {
		if w != widths[0] {
			return -1
		}
	}
	return widths[0]
}

func addWidth(total, w int) int {
	if total < 0 || w < 0 {
		return -1
	}
	return capWidth(total + w)
}

// capWidth keeps nested exact repeats from overflowing: any width past
// PCRE2's limit is rejected the same way.
func capWidth(w int) int {
	return min(w, pcreMaxLookbehind+1)
}

func (s *pcreWidthScanner) sequence() int {
	total := 0
	for s.i < len(s.p) && s.err == nil {
		c := s.p[s.i]
		if c == '|' || c == ')' {
			break
		}
		if s.extended && (c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v') {
			s.i++
			continue
		}
		if s.extended && c == '#' {
			if nl := strings.IndexByte(s.p[s.i:], '\n'); nl >= 0 {
				s.i += nl + 1
			} else {
				s.i = len(s.p)
			}
			continue
		}
		prefix, w, atom := s.atom()
		s.i = min(s.i, len(s.p)) // an unterminated construct ends the pattern
		total = addWidth(total, prefix)
		if atom {
			w = s.quantify(w)
		}
		total = addWidth(total, w)
	}
	return total
}

// atom consumes one item. prefix is width that a following quantifier does
// not repeat (all but the last character of \Q...\E); atom reports whether a
// quantifier may follow.
func (s *pcreWidthScanner) atom() (prefix, width int, atom bool) {
	c := s.p[s.i]
	switch c {
	case '\\':
		return s.escape()
	case '[':
		s.i = skipPCREClass(s.p, s.i) + 1
		return 0, 1, true
	case '(':
		return s.group()
	case '^', '$':
		s.i++
		return 0, 0, true
	}
	_, size := utf8.DecodeRuneInString(s.p[s.i:])
	s.i += size
	return 0, 1, true
}

func (s *pcreWidthScanner) escape() (prefix, width int, atom bool) {
	s.i++ // backslash
	if s.i >= len(s.p) {
		return 0, 1, true
	}
	c := s.p[s.i]
	s.i++
	switch c {
	case 'b', 'B', 'A', 'Z', 'z', 'G', 'K':
		return 0, 0, true
	case 'X', 'R', 'C':
		return 0, -1, true
	case 'Q':
		end := strings.Index(s.p[s.i:], `\E`)
		quoted := s.p[s.i:]
		if end >= 0 {
			quoted = s.p[s.i : s.i+end]
			s.i += end + 2
		} else {
			s.i = len(s.p)
		}
		n := utf8.RuneCountInString(quoted)
		if n == 0 {
			return 0, 0, false
		}
		return n - 1, 1, true
	case 'E':
		return 0, 0, false
	case 'x', 'o', 'p', 'P', 'N':
		if s.i < len(s.p) && s.p[s.i] == '{' {
			if end := strings.IndexByte(s.p[s.i:], '}'); end >= 0 {
				s.i += end + 1
			}
		} else if c == 'x' {
			for n := 0; n < 2 && s.i < len(s.p) && isHexDigit(s.p[s.i]); n++ {
				s.i++
			}
		} else if c == 'p' || c == 'P' {
			s.i++
		}
		return 0, 1, true
	case 'c':
		s.i++
		return 0, 1, true
	case 'g':
		return 0, s.backrefG(), true
	case 'k':
		if s.i < len(s.p) {
			if close := closingDelim(s.p[s.i]); close != 0 {
				if end := strings.IndexByte(s.p[s.i+1:], close); end >= 0 {
					name := s.p[s.i+1 : s.i+1+end]
					s.i += end + 2
					return 0, s.namedRef(name), true
				}
			}
		}
		return 0, 1, true
	}
	if c >= '1' && c <= '9' {
		start := s.i - 1
		for s.i < len(s.p) && s.p[s.i] >= '0' && s.p[s.i] <= '9' {
			s.i++
		}
		n, _ := strconv.Atoi(s.p[start:s.i])
		if n < 10 || c >= '8' || n <= len(s.groupLens) {
			return 0, s.numberedRef(n), true
		}
		// An octal escape: at most three octal digits are one character.
		s.i = start
		for d := 0; d < 3 && s.i < len(s.p) && s.p[s.i] >= '0' && s.p[s.i] <= '7'; d++ {
			s.i++
		}
		return 0, 1, true
	}
	if c >= utf8.RuneSelf {
		s.i--
		_, size := utf8.DecodeRuneInString(s.p[s.i:])
		s.i += size
	}
	return 0, 1, true
}

func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func closingDelim(open byte) byte {
	switch open {
	case '<':
		return '>'
	case '\'':
		return '\''
	case '{':
		return '}'
	}
	return 0
}

// backrefG handles \gN, \g-N, \g{N}, \g{-N} and \g{name}; \g<...> and \g'...'
// are subroutine calls.
func (s *pcreWidthScanner) backrefG() int {
	if s.i >= len(s.p) {
		return 1
	}
	ref := ""
	switch s.p[s.i] {
	case '{':
		end := strings.IndexByte(s.p[s.i:], '}')
		if end < 0 {
			return 1
		}
		ref = s.p[s.i+1 : s.i+end]
		s.i += end + 1
	case '<', '\'':
		return -1
	default:
		start := s.i
		if s.p[s.i] == '-' || s.p[s.i] == '+' {
			s.i++
		}
		for s.i < len(s.p) && s.p[s.i] >= '0' && s.p[s.i] <= '9' {
			s.i++
		}
		ref = s.p[start:s.i]
	}
	if n, err := strconv.Atoi(ref); err == nil {
		if strings.HasPrefix(ref, "-") || strings.HasPrefix(ref, "+") {
			n += s.ncap
			if n <= s.ncap {
				n++ // \g-1 is the most recently opened group
			}
		}
		return s.numberedRef(n)
	}
	return s.namedRef(ref)
}

func (s *pcreWidthScanner) numberedRef(n int) int {
	if w, ok := s.groupLens[n]; ok {
		return w
	}
	return -1
}

func (s *pcreWidthScanner) namedRef(name string) int {
	if n, ok := s.names[name]; ok {
		return s.numberedRef(n)
	}
	return -1
}

// quantify applies a following quantifier to an item of width w.
func (s *pcreWidthScanner) quantify(w int) int {
	for s.extended && s.i < len(s.p) && strings.IndexByte(" \t\n\r\f\v", s.p[s.i]) >= 0 {
		s.i++
	}
	if s.i >= len(s.p) {
		return w
	}
	var lo, hi int
	switch s.p[s.i] {
	case '*':
		lo, hi = 0, -1
		s.i++
	case '+':
		lo, hi = 1, -1
		s.i++
	case '?':
		lo, hi = 0, 1
		s.i++
	case '{':
		var ok bool
		lo, hi, ok = s.braceQuantifier()
		if !ok {
			return w // a literal '{', handled as the next atom
		}
	default:
		return w
	}
	if s.i < len(s.p) && (s.p[s.i] == '+' || s.p[s.i] == '?') {
		s.i++
	}
	if w == 0 {
		return 0
	}
	if w < 0 || lo < 0 || lo != hi {
		return -1
	}
	return capWidth(w * min(lo, pcreMaxLookbehind+1))
}

// braceQuantifier parses {n}, {n,} or {n,m}. PCRE2 10.42 reads anything
// else, including {,m}, as literal text.
func (s *pcreWidthScanner) braceQuantifier() (lo, hi int, ok bool) {
	end := strings.IndexByte(s.p[s.i:], '}')
	if end < 0 {
		return 0, 0, false
	}
	body := s.p[s.i+1 : s.i+end]
	loText, hiText, comma := strings.Cut(body, ",")
	if !allDigits(loText) || (comma && hiText != "" && !allDigits(hiText)) {
		return 0, 0, false
	}
	lo, err := strconv.Atoi(loText)
	if err != nil {
		lo = -2 // too large to be one fixed count
	}
	hi = lo
	if comma {
		hi = -1
		if hiText != "" {
			if hi, err = strconv.Atoi(hiText); err != nil {
				hi = -1
			}
		}
	}
	s.i += end + 1
	return lo, hi, true
}

func allDigits(text string) bool {
	if text == "" {
		return false
	}
	for i := 0; i < len(text); i++ {
		if text[i] < '0' || text[i] > '9' {
			return false
		}
	}
	return true
}

func (s *pcreWidthScanner) group() (prefix, width int, atom bool) {
	rest := s.p[s.i+1:]
	switch {
	case strings.HasPrefix(rest, "?#"):
		if end := strings.IndexByte(rest, ')'); end >= 0 {
			s.i += end + 2
		} else {
			s.i = len(s.p)
		}
		return 0, 0, false
	case strings.HasPrefix(rest, "*"):
		if end := strings.IndexByte(rest, ')'); end >= 0 {
			s.i += end + 2
		} else {
			s.i = len(s.p)
		}
		return 0, 0, false
	case strings.HasPrefix(rest, "?=") || strings.HasPrefix(rest, "?!"):
		s.i += 3
		s.body()
		return 0, 0, true
	case strings.HasPrefix(rest, "?<=") || strings.HasPrefix(rest, "?<!"):
		s.i += 4
		for _, w := range s.body() {
			if s.check && s.err == nil && (w < 0 || w > pcreMaxLookbehind) {
				s.err = errPCREVariableLookbehind
			}
		}
		return 0, 0, true
	case strings.HasPrefix(rest, "?:") || strings.HasPrefix(rest, "?>") || strings.HasPrefix(rest, "?|"):
		s.i += 3
		return 0, groupWidth(s.body()), true
	case strings.HasPrefix(rest, "?P<"):
		return s.capture(4, '>')
	case strings.HasPrefix(rest, "?<"):
		return s.capture(3, '>')
	case strings.HasPrefix(rest, "?'"):
		return s.capture(3, '\'')
	case strings.HasPrefix(rest, "?P="):
		end := strings.IndexByte(rest, ')')
		if end < 0 {
			s.i = len(s.p)
			return 0, 1, true
		}
		name := rest[3:end]
		s.i += end + 2
		return 0, s.namedRef(name), true
	case strings.HasPrefix(rest, "?("):
		return s.conditional()
	case strings.HasPrefix(rest, "?"):
		return s.flagGroup()
	}
	s.i++
	s.ncap++
	n := s.ncap
	w := groupWidth(s.body())
	s.groupLens[n] = w
	return 0, w, true
}

// body parses a group's alternation and its closing parenthesis.
func (s *pcreWidthScanner) body() []int {
	saved := s.extended
	widths := s.alternation()
	s.extended = saved
	if s.i < len(s.p) && s.p[s.i] == ')' {
		s.i++
	}
	return widths
}

func (s *pcreWidthScanner) capture(skip int, close byte) (prefix, width int, atom bool) {
	s.i += skip // past "(" and the group's prefix
	end := strings.IndexByte(s.p[s.i:], close)
	if end < 0 {
		s.i = len(s.p)
		return 0, 1, true
	}
	name := s.p[s.i : s.i+end]
	s.i += end + 1
	s.ncap++
	n := s.ncap
	if _, ok := s.names[name]; !ok {
		s.names[name] = n
	}
	w := groupWidth(s.body())
	s.groupLens[n] = w
	return 0, w, true
}

// conditional handles (?(condition)yes|no). A missing no-branch matches the
// empty string, so a one-branch conditional is fixed only at width 0.
func (s *pcreWidthScanner) conditional() (prefix, width int, atom bool) {
	s.i += 2 // "(?"
	if strings.HasPrefix(s.p[s.i:], "(?") {
		s.group() // an assertion condition
	} else if end := strings.IndexByte(s.p[s.i:], ')'); end >= 0 {
		s.i += end + 1
	} else {
		s.i = len(s.p)
		return 0, 1, true
	}
	widths := s.body()
	if len(widths) == 1 {
		widths = append(widths, 0)
	}
	return 0, groupWidth(widths), true
}

// flagGroup handles (?flags) and (?flags:...). Subroutine calls such as (?R),
// (?1) and (?&name) have no fixed width here.
func (s *pcreWidthScanner) flagGroup() (prefix, width int, atom bool) {
	j := s.i + 2
	for j < len(s.p) && (s.p[j] >= 'a' && s.p[j] <= 'z' || s.p[j] >= 'A' && s.p[j] <= 'Z' || s.p[j] == '-' || s.p[j] == '^') {
		j++
	}
	if j >= len(s.p) || (s.p[j] != ')' && s.p[j] != ':') {
		if end := strings.IndexByte(s.p[s.i:], ')'); end >= 0 {
			s.i += end + 1
		} else {
			s.i = len(s.p)
		}
		return 0, -1, true
	}
	extended := s.extended
	flags := s.p[s.i+2 : j]
	if strings.HasPrefix(flags, "^") {
		extended = false
	}
	on := true
	for k := 0; k < len(flags); k++ {
		switch flags[k] {
		case '-':
			on = false
		case 'x':
			extended = on
		}
	}
	s.i = j + 1
	if s.p[j] == ')' {
		s.extended = extended // lasts to the end of the enclosing group
		return 0, 0, false
	}
	saved := s.extended
	s.extended = extended
	w := groupWidth(s.body())
	s.extended = saved
	return 0, w, true
}
