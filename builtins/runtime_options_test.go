package builtins

import (
	"testing"

	"github.com/MongooseMoo/barn/config"
	"github.com/MongooseMoo/barn/types"
)

func runtimeOptionCtx(options config.Options) *Execution {
	ctx := newTestExecution()
	ctx.IsWizard = true
	ctx.RuntimeOptions = options
	return ctx
}

func TestServerVersionReportsBooleanOptions(t *testing.T) {
	tests := []struct {
		name    string
		options config.Options
		key     string
		want    string
	}{
		{name: "outbound dot disabled", key: "options.OUTBOUND_NETWORK", want: "OFF"},
		{name: "outbound slash disabled", key: "options/OUTBOUND_NETWORK", want: "OFF"},
		{name: "outbound dot enabled", options: config.Options{OutboundNetwork: true}, key: "options.OUTBOUND_NETWORK", want: "ON"},
		{name: "outbound slash enabled", options: config.Options{OutboundNetwork: true}, key: "options/OUTBOUND_NETWORK", want: "ON"},
		{name: "promote dot disabled", key: "options.PROMOTE_NUMBERS", want: "OFF"},
		{name: "promote slash disabled", key: "options/PROMOTE_NUMBERS", want: "OFF"},
		{name: "promote dot enabled", options: config.Options{PromoteNumbers: true}, key: "options.PROMOTE_NUMBERS", want: "ON"},
		{name: "promote slash enabled", options: config.Options{PromoteNumbers: true}, key: "options/PROMOTE_NUMBERS", want: "ON"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := builtinServerVersion(runtimeOptionCtx(test.options), []types.Value{types.NewStr(test.key)})
			if !result.IsNormal() {
				t.Fatalf("server_version(%q) returned error: %s", test.key, result.Error)
			}
			if got := result.Val.Type(); got != types.TYPE_STR {
				t.Fatalf("server_version(%q) type = %s, want STR", test.key, got)
			}
			if got := result.Val.Str(); got != test.want {
				t.Errorf("server_version(%q) = %q, want %q", test.key, got, test.want)
			}
		})
	}
}

// ToastStunt lists ONLY_32_BITS in every build's options group; a 64-bit build
// leaves it undefined, which version_options.h reports as #-1.
func TestServerVersionReportsOnly32BitsUndefined(t *testing.T) {
	for _, key := range []string{"options.ONLY_32_BITS", "options/ONLY_32_BITS"} {
		t.Run(key, func(t *testing.T) {
			result := builtinServerVersion(runtimeOptionCtx(config.Options{}), []types.Value{types.NewStr(key)})
			if !result.IsNormal() {
				t.Fatalf("server_version(%q) returned error: %s", key, result.Error)
			}
			if got := result.Val.Type(); got != types.TYPE_OBJ {
				t.Fatalf("server_version(%q) type = %s, want OBJ", key, got)
			}
			if got := result.Val.String(); got != "#-1" {
				t.Errorf("server_version(%q) = %s, want #-1", key, got)
			}
		})
	}
}

func TestServerVersionRejectsUnknownOption(t *testing.T) {
	for _, key := range []string{"options.UNKNOWN", "options/UNKNOWN"} {
		t.Run(key, func(t *testing.T) {
			result := builtinServerVersion(runtimeOptionCtx(config.Options{}), []types.Value{types.NewStr(key)})
			if result.Flow != types.FlowException || result.Error != types.E_INVARG {
				t.Errorf("server_version(%q) = flow %v error %s, want E_INVARG", key, result.Flow, result.Error)
			}
		})
	}
}

func TestServerVersionFeaturesHideDisabledOutboundNetwork(t *testing.T) {
	ctx := runtimeOptionCtx(config.Options{OutboundNetwork: false})

	result := builtinServerVersion(ctx, []types.Value{types.NewStr("features")})
	if !result.IsNormal() {
		t.Fatalf("server_version returned error: %s", result.Error)
	}
	features := result.Val
	for _, feature := range features.Elements() {
		if feature.Type() == types.TYPE_STR && feature.Str() == config.FeatureOutboundNetwork {
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
