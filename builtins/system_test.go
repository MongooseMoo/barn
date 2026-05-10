package builtins

import (
	"barn/config"
	"barn/types"
	"testing"
)

func TestServerVersionReportsOutboundNetworkOption(t *testing.T) {
	ctx := types.NewTaskContext()
	ctx.RuntimeOptions = config.Options{OutboundNetwork: false}

	res := builtinServerVersion(ctx, []types.Value{types.NewStr("options.OUTBOUND_NETWORK")})
	if res.IsError() {
		t.Fatalf("unexpected error: %v", res.Error)
	}
	got, ok := res.Val.(types.IntValue)
	if !ok || got.Val != 0 {
		t.Fatalf("got %v (%T), want 0", res.Val, res.Val)
	}

	ctx.RuntimeOptions = config.Options{OutboundNetwork: true}
	res = builtinServerVersion(ctx, []types.Value{types.NewStr("features")})
	if res.IsError() {
		t.Fatalf("unexpected error: %v", res.Error)
	}
	features, ok := res.Val.(types.ListValue)
	if !ok {
		t.Fatalf("got %T, want list", res.Val)
	}
	for i := 1; i <= features.Len(); i++ {
		if feature, ok := features.Get(i).(types.StrValue); ok && feature.Value() == config.FeatureOutboundNetwork {
			return
		}
	}
	t.Fatalf("features %s did not include %s", features.String(), config.FeatureOutboundNetwork)
}
