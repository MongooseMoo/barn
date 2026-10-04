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
	// Page cache given to each sqlite_open() handle, in KiB. Zero selects
	// DefaultSQLiteCacheKiB. It bounds memory per handle and changes no result.
	SQLiteCacheKiB int
	// Nil uses Barn's default build capabilities; a pointer to zero disables all.
	BuiltinCapabilities *Capabilities
}

// DefaultSQLiteCacheKiB is the page cache of a sqlite_open() handle. SQLite's
// own default of 2 MiB makes every scan of a larger database reread it from the
// operating system: the Mongoose sound database is 256 MB and is scanned on
// every `say`, which cost about 125 ms of file reads per command.
const DefaultSQLiteCacheKiB = 256 * 1024

const maxSQLiteCacheKiB = 64 * 1024 * 1024

// SQLiteCacheSizeKiB returns the configured page cache, or the default.
func (o Options) SQLiteCacheSizeKiB() int {
	if o.SQLiteCacheKiB == 0 {
		return DefaultSQLiteCacheKiB
	}
	return o.SQLiteCacheKiB
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
	if o.SQLiteCacheKiB < 0 || o.SQLiteCacheKiB > maxSQLiteCacheKiB {
		return fmt.Errorf("SQLITE_CACHE_KIB must be between 0 and %d", maxSQLiteCacheKiB)
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
