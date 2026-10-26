package builtins

func wireTestTaskHost(ctx *Execution) {
	manager := taskManagerStub{}
	session := ctx.Session
	if session == nil {
		registry := ctx.Registry
		if registry == nil {
			registry = NewRegistry()
			ctx.Registry = registry
		}
		session = NewSession(registry, NoHost())
		ctx.Session = session
	}
	configureTestHost(session, func(host *Host) { host.TaskManager = manager })
}
