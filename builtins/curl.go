package builtins

import (
	"io"
	"net/http"
	neturl "net/url"
	"time"

	"github.com/MongooseMoo/barn/types"
)

func builtinCurl(ctx *Execution, args []types.Value) types.Result {
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
	includeHeaders := false
	if len(args) >= 2 {
		includeHeaders = args[1].Truthy()
	}
	timeout := 5 * time.Second
	if len(args) == 3 {
		if args[2].Type() != types.TYPE_INT {
			return types.Err(types.E_TYPE)
		}
		if seconds := args[2].Int(); seconds > 0 {
			timeout = time.Duration(seconds) * time.Second
		}
	}
	rawURL := args[0].Str()
	request := func() types.Result {
		return curlRequest(rawURL, includeHeaders, timeout)
	}
	// curl() is a threaded builtin (see threaded.go). Threaded, the request is
	// issued after this slice commits and releases the commit gate; inline, it
	// is an irreversible effect inside the slice.
	if threadedCall(ctx) {
		return runInBackground(ctx, request)
	}
	if ctx.TaskContext != nil {
		if !beginIrreversible(ctx) {
			return abortedAttempt()
		}
		ctx.IrreversibleSideEffect = true
	}
	return request()
}

func curlRequest(rawURL string, includeHeaders bool, timeout time.Duration) types.Result {
	parsed, err := neturl.Parse(rawURL)
	if err != nil {
		return curlErrorMap(err.Error())
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return curlErrorMap("unsupported protocol")
	}
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return curlErrorMap(err.Error())
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return curlErrorMap(err.Error())
	}
	defer func() { _ = resp.Body.Close() }()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return curlErrorMap(err.Error())
	}
	entries := [][2]types.Value{
		{types.NewStr("status"), types.NewInt(int64(resp.StatusCode))},
		{types.NewStr("body"), types.NewStr(string(payload))},
	}
	if includeHeaders {
		entries = append(entries, [2]types.Value{types.NewStr("headers"), types.NewStr(resp.Header.Get("Content-Type"))})
	}
	result := types.NewMap(entries)
	return types.Ok(result)
}

func curlErrorMap(message string) types.Result {
	return types.Ok(types.NewMap([][2]types.Value{
		{types.NewStr("error"), types.NewStr(message)},
	}))
}
