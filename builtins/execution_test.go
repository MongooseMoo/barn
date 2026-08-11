package builtins

import "github.com/MongooseMoo/barn/kernel"

func newTestExecution() *Execution {
	registry := NewRegistry()
	return registry.NewExecution(kernel.NewTaskContext(), nil)
}
