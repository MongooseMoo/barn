package builtins

// Session owns the mutable process state and host capabilities used while
// executing builtins. Multiple sessions may share one dispatch Registry
// without sharing handles, cached options, protected flags, or connection
// state.
type Session struct {
	registry *Registry
	host     Host
	runtime  *sessionRuntime
}

// NewSession binds a shared dispatch registry to one mutable runtime
// session. Callers with no host capabilities must say so explicitly with
// NoHost.
func NewSession(registry *Registry, host Host) *Session {
	if registry == nil {
		panic("builtins: nil registry")
	}
	return &Session{
		registry: registry,
		host:     host,
		runtime:  newSessionRuntime(),
	}
}

// Registry returns the dispatch table shared by this session.
func (s *Session) Registry() *Registry {
	if s == nil {
		return nil
	}
	return s.registry
}

// Host returns a copy of the session's current host configuration.
func (s *Session) Host() Host {
	if s == nil {
		return NoHost()
	}
	return s.host
}

// ConfigureHost replaces the host capability bundle. Runtime owners call this
// during startup, before builtin execution begins.
func (s *Session) ConfigureHost(host Host) {
	s.host = host
}
