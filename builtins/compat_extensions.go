package builtins

import (
	"barn/types"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
)

func builtinUrlEncode(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}
	s, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	spacePlus := true
	if len(args) == 2 {
		spacePlus = args[1].Truthy()
	}
	if spacePlus {
		return types.Ok(types.NewStr(url.QueryEscape(s.Value())))
	}
	return types.Ok(types.NewStr(strings.ReplaceAll(url.PathEscape(s.Value()), "+", "%20")))
}

func builtinUrlDecode(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	s, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	decoded, err := url.QueryUnescape(s.Value())
	if err != nil {
		return types.Err(types.E_INVARG)
	}
	return types.Ok(types.NewStr(decoded))
}

func builtinPcreMatch(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) < 2 || len(args) > 4 {
		return types.Err(types.E_ARGS)
	}
	subject, ok1 := args[0].(types.StrValue)
	pattern, ok2 := args[1].(types.StrValue)
	if !ok1 || !ok2 {
		return types.Err(types.E_TYPE)
	}
	if subject.Value() == "" || pattern.Value() == "" {
		return types.Err(types.E_INVARG)
	}

	caseMatters := false
	if len(args) >= 3 {
		caseMatters = args[2].Truthy()
	}
	findAll := true
	if len(args) == 4 {
		findAll = args[3].Truthy()
	}

	pat := pattern.Value()
	if !caseMatters {
		pat = "(?i)" + pat
	}
	re, err := regexp.Compile(pat)
	if err != nil {
		return types.Err(types.E_INVARG)
	}

	maxMatches := -1
	if !findAll {
		maxMatches = 1
	}
	matches := re.FindAllStringSubmatchIndex(subject.Value(), maxMatches)
	if len(matches) == 0 {
		return types.Ok(types.NewList([]types.Value{}))
	}

	names := re.SubexpNames()
	out := make([]types.Value, 0, len(matches))
	for _, loc := range matches {
		entryPairs := make([][2]types.Value, 0, len(names)+1)
		entryPairs = append(entryPairs, [2]types.Value{
			types.NewStr("0"),
			buildPcreCapture(subject.Value(), loc[0], loc[1]),
		})

		for i := 1; i < len(names); i++ {
			gStart := -1
			gEnd := -1
			if i*2+1 < len(loc) {
				gStart = loc[i*2]
				gEnd = loc[i*2+1]
			}
			key := strconv.Itoa(i)
			if names[i] != "" {
				// Named groups use their name instead of numeric key.
				key = names[i]
			}
			entryPairs = append(entryPairs, [2]types.Value{
				types.NewStr(key),
				buildPcreCapture(subject.Value(), gStart, gEnd),
			})
		}
		out = append(out, types.NewMap(entryPairs))
	}

	return types.Ok(types.NewList(out))
}

func buildPcreCapture(subject string, start, end int) types.Value {
	match := ""
	pos := []types.Value{}
	if start >= 0 && end >= start && end <= len(subject) {
		match = subject[start:end]
		// 1-based inclusive positions.
		pos = []types.Value{types.NewInt(int64(start + 1)), types.NewInt(int64(end))}
	}
	return types.NewMap([][2]types.Value{
		{types.NewStr("match"), types.NewStr(match)},
		{types.NewStr("position"), types.NewList(pos)},
	})
}

func builtinPcreReplace(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}
	subject, ok1 := args[0].(types.StrValue)
	spec, ok2 := args[1].(types.StrValue)
	if !ok1 || !ok2 {
		return types.Err(types.E_TYPE)
	}

	pattern, replacement, flags, ok := parseSedReplaceSpec(spec.Value())
	if !ok || pattern == "" {
		return types.Err(types.E_INVARG)
	}

	global := false
	caseInsensitive := false
	for _, flag := range flags {
		switch flag {
		case 'g':
			global = true
		case 'i':
			caseInsensitive = true
		default:
			return types.Err(types.E_INVARG)
		}
	}

	if caseInsensitive {
		pattern = "(?i)" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return types.Err(types.E_INVARG)
	}

	replacement = normalizePcreReplacement(replacement)

	var out string
	if global {
		out = re.ReplaceAllString(subject.Value(), replacement)
	} else {
		idx := re.FindStringIndex(subject.Value())
		if idx == nil {
			out = subject.Value()
		} else {
			replaced := re.ReplaceAllString(subject.Value()[idx[0]:idx[1]], replacement)
			out = subject.Value()[:idx[0]] + replaced + subject.Value()[idx[1]:]
		}
	}
	if errCode := CheckStringLimit(out); errCode != types.E_NONE {
		return types.Err(errCode)
	}
	return types.Ok(types.NewStr(out))
}

func normalizePcreReplacement(replacement string) string {
	// MOO PCRE replacement supports $& for whole-match; Go uses $0.
	return strings.ReplaceAll(replacement, "$&", "$0")
}

func parseSedReplaceSpec(spec string) (pattern, replacement, flags string, ok bool) {
	if len(spec) < 4 || spec[0] != 's' {
		return "", "", "", false
	}
	delim := spec[1]
	pattern, next, ok := readDelimited(spec, 2, delim)
	if !ok {
		return "", "", "", false
	}
	replacement, next, ok = readDelimited(spec, next, delim)
	if !ok {
		return "", "", "", false
	}
	return pattern, replacement, spec[next:], true
}

func readDelimited(s string, start int, delim byte) (string, int, bool) {
	var out strings.Builder
	for i := start; i < len(s); i++ {
		ch := s[i]
		if ch == delim {
			return out.String(), i + 1, true
		}
		if ch == '\\' {
			if i+1 >= len(s) {
				return "", 0, false
			}
			next := s[i+1]
			if next == delim || next == '\\' {
				out.WriteByte(next)
			} else {
				out.WriteByte('\\')
				out.WriteByte(next)
			}
			i++
			continue
		}
		out.WriteByte(ch)
	}
	return "", 0, false
}

func builtinPcreCacheStats(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	return types.Ok(types.NewList([]types.Value{types.NewInt(0), types.NewInt(0)}))
}

func builtinArgon2(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}
	password, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	var salt []byte
	if len(args) == 2 {
		s, ok := args[1].(types.StrValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		salt = []byte(s.Value())
		if len(salt) < 8 {
			return types.Err(types.E_INVARG)
		}
	} else {
		salt = make([]byte, 16)
		if _, err := rand.Read(salt); err != nil {
			return types.Err(types.E_EXEC)
		}
	}
	const t = uint32(1)
	const m = uint32(64 * 1024)
	const p = uint8(2)
	const keyLen = uint32(32)
	h := argon2.IDKey([]byte(password.Value()), salt, t, m, p, keyLen)
	encoded := fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", m, t, p,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(h),
	)
	return types.Ok(types.NewStr(encoded))
}

func parseArgon2Hash(encoded string) (uint32, uint32, uint8, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return 0, 0, 0, nil, nil, fmt.Errorf("invalid")
	}
	params := strings.Split(parts[3], ",")
	if len(params) != 3 {
		return 0, 0, 0, nil, nil, fmt.Errorf("invalid")
	}
	m64, err := strconv.ParseUint(strings.TrimPrefix(params[0], "m="), 10, 32)
	if err != nil {
		return 0, 0, 0, nil, nil, err
	}
	t64, err := strconv.ParseUint(strings.TrimPrefix(params[1], "t="), 10, 32)
	if err != nil {
		return 0, 0, 0, nil, nil, err
	}
	p64, err := strconv.ParseUint(strings.TrimPrefix(params[2], "p="), 10, 8)
	if err != nil {
		return 0, 0, 0, nil, nil, err
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return 0, 0, 0, nil, nil, err
	}
	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return 0, 0, 0, nil, nil, err
	}
	return uint32(m64), uint32(t64), uint8(p64), salt, hash, nil
}

func builtinArgon2Verify(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}
	a, ok1 := args[0].(types.StrValue)
	b, ok2 := args[1].(types.StrValue)
	if !ok1 || !ok2 {
		return types.Err(types.E_TYPE)
	}
	hashStr := a.Value()
	password := b.Value()
	if !strings.HasPrefix(hashStr, "$argon2") && strings.HasPrefix(password, "$argon2") {
		hashStr, password = password, hashStr
	}
	m, t, p, salt, expected, err := parseArgon2Hash(hashStr)
	if err != nil {
		return types.Err(types.E_INVARG)
	}
	actual := argon2.IDKey([]byte(password), salt, t, m, p, uint32(len(expected)))
	if subtle.ConstantTimeCompare(actual, expected) == 1 {
		return types.Ok(types.NewInt(1))
	}
	return types.Ok(types.NewInt(0))
}

func builtinCurl(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}
	urlVal, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	method := "GET"
	body := ""
	if len(args) >= 2 {
		m, ok := args[1].(types.StrValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		method = strings.ToUpper(strings.TrimSpace(m.Value()))
		if method == "" {
			method = "GET"
		}
	}
	if len(args) == 3 {
		b, ok := args[2].(types.StrValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		body = b.Value()
	}
	req, err := http.NewRequest(method, urlVal.Value(), strings.NewReader(body))
	if err != nil {
		return types.Err(types.E_INVARG)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return types.Err(types.E_INVARG)
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return types.Err(types.E_EXEC)
	}
	result := types.NewMap([][2]types.Value{
		{types.NewStr("status"), types.NewInt(int64(resp.StatusCode))},
		{types.NewStr("body"), types.NewStr(string(payload))},
	})
	return types.Ok(result)
}
