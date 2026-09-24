package builtins

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/MongooseMoo/barn/types"
	"github.com/rivo/uniseg"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/htmlindex"
	"golang.org/x/text/encoding/ianaindex"
	"golang.org/x/text/encoding/unicode/utf32"
	"golang.org/x/text/unicode/runenames"
)

// Character builtins for Barn's code-point strings. ord, tochar, charname,
// encode_chars and decode_chars follow LambdaMOO 1.9's Unicode server
// (wrog's list.c); string_width and graphemes measure what a terminal shows.

// isMOOPrintable is LambdaMOO 1.9's my_is_printable: the code points a MOO
// string constant may hold. Tab is allowed; other C0/C1 controls, DEL,
// surrogates and noncharacters are not.
func isMOOPrintable(r rune) bool {
	if r == '\t' {
		return true
	}
	if r < 0 || r > utf8.MaxRune {
		return false
	}
	if r <= 0xff && (r&0x60 == 0 || r == 0x7f) {
		return false
	}
	if r >= 0xd800 && r <= 0xdfff {
		return false
	}
	// Noncharacters: U+FDD0..U+FDEF and the last two code points of every plane.
	if (r >= 0xfdd0 && r <= 0xfdef) || r&0xfffe == 0xfffe {
		return false
	}
	return true
}

// singleRune returns the code point of a one-character string. A lone invalid
// byte is a character in Barn but has no code point.
func singleRune(s string) (rune, bool) {
	r, size := utf8.DecodeRuneInString(s)
	if size == 0 || size != len(s) || (r == utf8.RuneError && size == 1) {
		return 0, false
	}
	return r, true
}

func builtinOrd(ctx *Execution, args []types.Value) types.Result {
	r, ok := singleRune(args[0].Str())
	if !ok {
		return types.Err(types.E_INVARG)
	}
	return types.Ok(types.NewInt(int64(r)))
}

// builtinTochar maps a code point, or a Unicode character name, to a
// one-character string.
func builtinTochar(ctx *Execution, args []types.Value) types.Result {
	var r rune
	switch args[0].Type() {
	case types.TYPE_INT:
		n := args[0].Int()
		if n < 0 || n > utf8.MaxRune {
			return types.Err(types.E_INVARG)
		}
		r = rune(n)
	case types.TYPE_STR:
		var ok bool
		if r, ok = lookupCharName(args[0].Str()); !ok {
			return types.Err(types.E_INVARG)
		}
	default:
		return types.Err(types.E_TYPE)
	}
	if !isMOOPrintable(r) {
		return types.Err(types.E_INVARG)
	}
	return types.Ok(types.NewStr(string(r)))
}

func builtinCharname(ctx *Execution, args []types.Value) types.Result {
	r, ok := singleRune(args[0].Str())
	if !ok {
		return types.Err(types.E_INVARG)
	}
	name := charName(r)
	if name == "" {
		return types.Err(types.E_INVARG)
	}
	return types.Ok(types.NewStr(name))
}

const (
	hangulBase  = 0xac00
	hangulCount = 11172
	hangulV     = 21
	hangulT     = 28
)

var (
	hangulL = []string{"G", "GG", "N", "D", "DD", "R", "M", "B", "BB", "S", "SS", "", "J", "JJ", "C", "K", "T", "P", "H"}
	hangulM = []string{"A", "AE", "YA", "YAE", "EO", "E", "YEO", "YE", "O", "WA", "WAE", "OE", "YO", "U", "WEO", "WE", "WI", "YU", "EU", "YI", "I"}
	hangulF = []string{"", "G", "GG", "GS", "N", "NJ", "NH", "D", "L", "LG", "LM", "LB", "LS", "LT", "LP", "LH", "M", "B", "BS", "S", "SS", "NG", "J", "C", "K", "T", "P", "H"}
)

// charName is the Unicode Name property. runenames abbreviates the
// algorithmically named ranges, so those names are derived here (Unicode
// chapter 4.8); code points with no Name property (controls, private use,
// unassigned) return "".
func charName(r rune) string {
	name := runenames.Name(r)
	if !strings.HasPrefix(name, "<") {
		return name
	}
	switch name {
	case "<CJK Ideograph>", "<CJK Ideograph Extension A>", "<CJK Ideograph Extension B>",
		"<CJK Ideograph Extension C>", "<CJK Ideograph Extension D>", "<CJK Ideograph Extension E>",
		"<CJK Ideograph Extension F>", "<CJK Ideograph Extension G>", "<CJK Ideograph Extension H>":
		return fmt.Sprintf("CJK UNIFIED IDEOGRAPH-%04X", r)
	case "<Tangut Ideograph>", "<Tangut Ideograph Supplement>":
		return fmt.Sprintf("TANGUT IDEOGRAPH-%04X", r)
	case "<Hangul Syllable>":
		s := int(r) - hangulBase
		return "HANGUL SYLLABLE " + hangulL[s/(hangulV*hangulT)] + hangulM[s%(hangulV*hangulT)/hangulT] + hangulF[s%hangulT]
	}
	return ""
}

var (
	charNamesOnce sync.Once
	charNames     map[string]rune
)

// lookupCharName inverts charName, ignoring case as ICU's u_charFromName does.
func lookupCharName(name string) (rune, bool) {
	name = strings.ToUpper(name)
	for _, prefix := range []string{"CJK UNIFIED IDEOGRAPH-", "TANGUT IDEOGRAPH-"} {
		if hex, ok := strings.CutPrefix(name, prefix); ok {
			n, err := strconv.ParseUint(hex, 16, 32)
			if err != nil || charName(rune(n)) != name {
				return 0, false
			}
			return rune(n), true
		}
	}
	charNamesOnce.Do(func() {
		charNames = make(map[string]rune, 50000)
		for r := rune(0); r <= utf8.MaxRune; r++ {
			if n := runenames.Name(r); n != "" && !strings.HasPrefix(n, "<") {
				charNames[n] = r
			}
		}
		for s := 0; s < hangulCount; s++ {
			charNames[charName(rune(hangulBase+s))] = rune(hangulBase + s)
		}
	})
	r, ok := charNames[name]
	return r, ok
}

// lookupEncoding resolves a charset name. IANA names come first so "latin1"
// is ISO-8859-1; WHATWG labels such as "utf8" and "cp1252" fill in after.
func lookupEncoding(name string) (encoding.Encoding, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "utf-32", "utf32":
		return utf32.UTF32(utf32.BigEndian, utf32.UseBOM), true
	case "utf-32be", "utf32be":
		return utf32.UTF32(utf32.BigEndian, utf32.IgnoreBOM), true
	case "utf-32le", "utf32le":
		return utf32.UTF32(utf32.LittleEndian, utf32.IgnoreBOM), true
	}
	if enc, err := ianaindex.IANA.Encoding(name); err == nil && enc != nil {
		return enc, true
	}
	if enc, err := htmlindex.Get(name); err == nil && enc != nil {
		return enc, true
	}
	return nil, false
}

func isUTF8Encoding(enc encoding.Encoding) bool {
	name, _ := ianaindex.IANA.Name(enc)
	return name == "UTF-8"
}

// appendCharsForEncoding flattens encode_chars' argument: strings, code
// point integers, and lists of either.
func appendCharsForEncoding(out *strings.Builder, v types.Value, rawBytesOK bool) types.ErrorCode {
	switch v.Type() {
	case types.TYPE_STR:
		s := v.Str()
		if !rawBytesOK && !utf8.ValidString(s) {
			return types.E_INVARG
		}
		out.WriteString(s)
	case types.TYPE_INT:
		n := v.Int()
		if n < 0 || n > utf8.MaxRune || (n >= 0xd800 && n <= 0xdfff) {
			return types.E_INVARG
		}
		out.WriteRune(rune(n))
	case types.TYPE_LIST:
		for i := 1; i <= v.Len(); i++ {
			if err := appendCharsForEncoding(out, v.Get(i), rawBytesOK); err != types.E_NONE {
				return err
			}
		}
	default:
		return types.E_INVARG
	}
	return types.E_NONE
}

// builtinEncodeChars encodes characters into a binary string in the named
// encoding. UTF-8 copies a string's invalid bytes through unchanged; other
// encodings cannot represent them and raise E_INVARG.
func builtinEncodeChars(ctx *Execution, args []types.Value) types.Result {
	enc, ok := lookupEncoding(args[1].Str())
	if !ok {
		return types.Err(types.E_INVARG)
	}
	utf8Target := isUTF8Encoding(enc)
	var chars strings.Builder
	if err := appendCharsForEncoding(&chars, args[0], utf8Target); err != types.E_NONE {
		return types.Err(err)
	}
	encoded := chars.String()
	if !utf8Target {
		var err error
		if encoded, err = enc.NewEncoder().String(encoded); err != nil {
			return types.Err(types.E_INVARG)
		}
	}
	result := encodeBinaryStr([]byte(encoded))
	ctx.Session.UpdateContextLimits(ctx.TaskContext)
	if err := ctx.CheckStringLimit(len(result)); err != types.E_NONE {
		return types.Err(err)
	}
	return types.Ok(types.NewStr(result))
}

// builtinDecodeChars decodes a binary string in the named encoding. Bytes
// that do not decode raise E_INVARG. By default runs of printable characters
// become strings and every other code point an integer; a true third argument
// returns every code point as an integer.
func builtinDecodeChars(ctx *Execution, args []types.Value) types.Result {
	raw, bad := decodeBinaryString(args[0].Str())
	if bad {
		return types.Err(types.E_INVARG)
	}
	enc, ok := lookupEncoding(args[1].Str())
	if !ok {
		return types.Err(types.E_INVARG)
	}
	fully := len(args) == 3 && args[2].Truthy()

	var text string
	if isUTF8Encoding(enc) {
		if !utf8.Valid(raw) {
			return types.Err(types.E_INVARG)
		}
		text = string(raw)
	} else {
		decoded, err := enc.NewDecoder().Bytes(raw)
		if err != nil {
			return types.Err(types.E_INVARG)
		}
		text = string(decoded)
	}
	text = strings.TrimPrefix(text, "\ufeff")

	ctx.Session.UpdateContextLimits(ctx.TaskContext)
	var elements []types.Value
	if fully {
		for _, r := range text {
			elements = append(elements, types.NewInt(int64(r)))
		}
	} else {
		run := 0
		for i, r := range text {
			if isMOOPrintable(r) {
				continue
			}
			if run < i {
				elements = append(elements, types.NewStr(text[run:i]))
			}
			elements = append(elements, types.NewInt(int64(r)))
			run = i + utf8.RuneLen(r)
		}
		if run < len(text) {
			elements = append(elements, types.NewStr(text[run:]))
		}
	}
	result := types.NewList(elements)
	if err := ctx.Session.CheckListLimitForTask(ctx.TaskContext, result); err != types.E_NONE {
		return types.Err(err)
	}
	return types.Ok(result)
}

// builtinStringWidth returns the number of terminal columns a string
// occupies: wide East Asian characters and emoji take two, combining marks
// and zero-width joiners none.
func builtinStringWidth(ctx *Execution, args []types.Value) types.Result {
	return types.Ok(types.NewInt(int64(uniseg.StringWidth(args[0].Str()))))
}

// builtinGraphemes splits a string into user-perceived characters (Unicode
// extended grapheme clusters). The pieces rejoin to the original bytes.
func builtinGraphemes(ctx *Execution, args []types.Value) types.Result {
	s := args[0].Str()
	var elements []types.Value
	state := -1
	for len(s) > 0 {
		var cluster string
		cluster, s, _, state = uniseg.FirstGraphemeClusterInString(s, state)
		elements = append(elements, types.NewStr(cluster))
	}
	result := types.NewList(elements)
	if err := ctx.Session.CheckListLimitForTask(ctx.TaskContext, result); err != types.E_NONE {
		return types.Err(err)
	}
	return types.Ok(result)
}
