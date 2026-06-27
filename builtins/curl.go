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
	if args[0].Type() != types.TYPE_STR {
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
		if args[1].Type() != types.TYPE_STR {
			return types.Err(types.E_TYPE)
		}
		method = strings.ToUpper(strings.TrimSpace(args[1].Str()))
		if method == "" {
			method = "GET"
		}
	}
	if len(args) == 3 {
		if args[2].Type() != types.TYPE_STR {
			return types.Err(types.E_TYPE)
		}
		body = args[2].Str()
	}
	req, err := http.NewRequest(method, args[0].Str(), strings.NewReader(body))
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
