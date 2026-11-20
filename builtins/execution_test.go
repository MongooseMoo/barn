package builtins

import "github.com/MongooseMoo/barn/task"

func newTestExecution() *Execution {
	registry := NewRegistry()
	return newTestExecutionForSession(NewSession(registry, NoHost()))
}

func newTestExecutionForSession(session *Session) *Execution {
	return session.NewExecution(task.NewContext(), nil)
}

func configureTestHost(session *Session, configure func(*Host)) {
	host := session.Host()
	configure(&host)
	session.ConfigureHost(host)
}
