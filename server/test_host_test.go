package server

import "github.com/MongooseMoo/barn/builtins"

func configureTestBuiltinHost(session *builtins.Session, configure func(*builtins.Host)) {
	host := session.Host()
	configure(&host)
	session.ConfigureHost(host)
}

func setTestConnectionManager(session *builtins.Session, manager builtins.ConnectionManager) {
	configureTestBuiltinHost(session, func(host *builtins.Host) { host.ConnManager = manager })
}
