package builtins

import "github.com/MongooseMoo/barn/kernel"

func newTestExecution() *Execution {
	registry := NewRegistry()
	return newTestExecutionForSession(NewSession(registry, NoHost()))
}

func newTestExecutionForSession(session *Session) *Execution {
	return session.NewExecution(kernel.NewTaskContext(), nil)
}

func configureTestHost(session *Session, configure func(*Host)) {
	host := session.Host()
	configure(&host)
	session.ConfigureHost(host)
}
