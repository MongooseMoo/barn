package builtins

import (
	"io"
	"net/http"
	"strings"
	"time"

	"barn/kernel"
	"barn/types"
)

func builtinCurl(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}
	urlVal, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if !ctx.RuntimeOptions.OutboundNetwork {
		return types.Err(types.E_PERM)
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
