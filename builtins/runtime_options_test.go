package builtins

import (
	"testing"

	"barn/config"
	"barn/kernel"
	"barn/types"
)

func runtimeOptionCtx(options config.Options) *kernel.TaskContext {
	ctx := kernel.NewTaskContext()
	ctx.IsWizard = true
	ctx.RuntimeOptions = options
	return ctx
}

func TestServerVersionReportsOutboundNetworkOption(t *testing.T) {
	ctx := runtimeOptionCtx(config.Options{OutboundNetwork: false})

	result := builtinServerVersion(ctx, []types.Value{types.NewStr("options.OUTBOUND_NETWORK")})
	if !result.IsNormal() {
		t.Fatalf("server_version returned error: %s", result.Error)
	}
	if got := result.Val.(types.IntValue).Val; got != 0 {
		t.Fatalf("OUTBOUND_NETWORK = %d, want 0", got)
	}
}

func TestServerVersionFeaturesHideDisabledOutboundNetwork(t *testing.T) {
	ctx := runtimeOptionCtx(config.Options{OutboundNetwork: false})

	result := builtinServerVersion(ctx, []types.Value{types.NewStr("features")})
	if !result.IsNormal() {
		t.Fatalf("server_version returned error: %s", result.Error)
	}
	features := result.Val.(types.ListValue)
	for _, feature := range features.Elements() {
		if s, ok := feature.(types.StrValue); ok && s.Value() == config.FeatureOutboundNetwork {
			t.Fatalf("disabled features unexpectedly include %s", config.FeatureOutboundNetwork)
		}
	}
}

func TestOpenNetworkConnectionDisabledReturnsPerm(t *testing.T) {
	ctx := runtimeOptionCtx(config.Options{OutboundNetwork: false})

	result := builtinOpenNetworkConnection(ctx, []types.Value{types.NewStr("127.0.0.1"), types.NewInt(1)})
	if result.Flow != types.FlowException || result.Error != types.E_PERM {
		t.Fatalf("open_network_connection result = flow %v error %s, want E_PERM", result.Flow, result.Error)
	}
}

func TestCurlDisabledReturnsPerm(t *testing.T) {
	ctx := runtimeOptionCtx(config.Options{OutboundNetwork: false})

	result := builtinCurl(ctx, []types.Value{types.NewStr("http://127.0.0.1/")})
	if result.Flow != types.FlowException || result.Error != types.E_PERM {
		t.Fatalf("curl result = flow %v error %s, want E_PERM", result.Flow, result.Error)
	}
}
