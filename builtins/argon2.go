package builtins

import (
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"math"
	"strconv"
	"strings"

	"barn/kernel"
	"barn/types"

	"golang.org/x/crypto/argon2"
)

func builtinArgon2(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if len(args) < 2 || len(args) > 5 {
		return types.Err(types.E_ARGS)
	}
	password, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	s, ok := args[1].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	salt := []byte(s.Value())
	if len(salt) < 8 {
		return types.Err(types.E_INVARG)
	}

	t := uint32(1)
	m := uint32(64 * 1024)
	p := uint8(2)
	if len(args) >= 3 {
		iterVal, ok := args[2].(types.IntValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		if iterVal.Val <= 0 {
			return types.Err(types.E_INVARG)
		}
		t = uint32(iterVal.Val)
	}
	if len(args) >= 4 {
		memVal, ok := args[3].(types.IntValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		if memVal.Val <= 0 {
			return types.Err(types.E_INVARG)
		}
		m = uint32(memVal.Val)
	}
	if len(args) == 5 {
		parallelVal, ok := args[4].(types.IntValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		if parallelVal.Val <= 0 || parallelVal.Val > math.MaxUint8 {
			return types.Err(types.E_INVARG)
		}
		p = uint8(parallelVal.Val)
	}

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

func builtinArgon2Verify(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
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
