package config

const (
	FeatureOutboundNetwork       = "option.OUTBOUND_NETWORK"
	FeatureOpenNetworkConnection = "builtin.open_network_connection"
	FeaturePromoteNumbers        = "option.PROMOTE_NUMBERS"
)

// Options holds Barn runtime options that affect Toast-compatible semantics.
type Options struct {
	OutboundNetwork bool
	PromoteNumbers  bool
}

// DefaultOptions returns Barn's default runtime options for normal operation.
func DefaultOptions() Options {
	return Options{
		OutboundNetwork: true,
		PromoteNumbers:  false,
	}
}

// Validate checks whether the option set is internally consistent.
func (o Options) Validate() error {
	return nil
}

// FeatureMap returns the machine-readable feature keys used by profile
// manifests and conformance metadata gates.
func (o Options) FeatureMap() map[string]any {
	return map[string]any{
		FeatureOutboundNetwork:       o.OutboundNetwork,
		FeatureOpenNetworkConnection: "present",
		FeaturePromoteNumbers:        o.PromoteNumbers,
	}
}

// FeatureNames returns the MOO-visible feature names exposed by
// server_version("features").
func (o Options) FeatureNames() []string {
	features := []string{
		"64bit",
		FeatureOpenNetworkConnection,
	}
	if o.OutboundNetwork {
		features = append(features, FeatureOutboundNetwork)
	}
	if o.PromoteNumbers {
		features = append(features, FeaturePromoteNumbers)
	}
	return features
}
