package config

import "fmt"

const (
	FeatureOutboundNetwork       = "option.OUTBOUND_NETWORK"
	FeatureOpenNetworkConnection = "builtin.open_network_connection"
	FeaturePromoteNumbers        = "option.PROMOTE_NUMBERS"
)

// Options holds Barn runtime options that affect Toast-compatible semantics.
type Options struct {
	OutboundNetwork bool
	PromoteNumbers  bool
	// Zero selects the runtime default; these affect scheduling, not MOO semantics.
	AdmissionLimit, AdmissionPrincipalLimit         int
	AdmissionInputWeight, AdmissionBackgroundWeight int
	AdmissionAnonymousWeight, AdmissionSystemWeight int
	// Nil uses Barn's default build capabilities; a pointer to zero disables all.
	BuiltinCapabilities *Capabilities
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
	for _, value := range []int{o.AdmissionLimit, o.AdmissionPrincipalLimit, o.AdmissionInputWeight, o.AdmissionBackgroundWeight, o.AdmissionAnonymousWeight, o.AdmissionSystemWeight} {
		if value < 0 || value > 1000000 {
			return fmt.Errorf("admission settings must be between 0 and 1000000")
		}
	}
	if o.Capabilities() & ^DefaultCapabilities() != 0 {
		return fmt.Errorf("unknown builtin capabilities")
	}
	return nil
}

func (o Options) Capabilities() Capabilities {
	if o.BuiltinCapabilities == nil {
		return DefaultCapabilities()
	}
	return *o.BuiltinCapabilities
}

// FeatureMap returns the machine-readable feature keys used by profile
// manifests and conformance metadata gates.
func (o Options) FeatureMap() map[string]any {
	return map[string]any{
		FeatureOutboundNetwork: o.OutboundNetwork,
		FeaturePromoteNumbers:  o.PromoteNumbers,
	}
}

// FeatureNames returns the MOO-visible feature names exposed by
// server_version("features").
func (o Options) FeatureNames() []string {
	features := []string{
		"64bit",
	}
	if o.OutboundNetwork {
		features = append(features, FeatureOutboundNetwork)
	}
	if o.PromoteNumbers {
		features = append(features, FeaturePromoteNumbers)
	}
	return features
}
