package server

import (
	"errors"
	"net"
	"path/filepath"
	"strconv"
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/engine"
	"github.com/MongooseMoo/barn/internal/listener"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func TestStartListenerFailureClosesBuiltinHandles(t *testing.T) {
	t.Chdir(t.TempDir())
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	port := occupied.Addr().(*net.TCPAddr).Port
	s, err := NewServer(filepath.Join(t.TempDir(), "unused.db"), []listener.Spec{{
		Protocol: listener.ProtocolTCP, Interface: "127.0.0.1", Port: int64(port),
	}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	s.store = dbstore.NewStore()
	addTestObject(t, s.store, 0, dbstore.FlagWizard|dbstore.FlagProgrammer)
	s.runtime = engine.NewRuntimeWithRegistry(s.store, s.options, s.registry)
	s.input = NewInputProcessor(s.store, s.runtime)
	s.input.SetConnectionManager(s.connManager)
	builtinSession := s.runtime.Session()
	t.Cleanup(func() {
		s.cancel()
		s.connManager.CloseListeners()
		s.input.Stop()
		s.runtime.Stop()
		if err := builtinSession.Close(); err != nil {
			t.Errorf("cleanup builtin session: %v", err)
		}
	})
	outcome := s.runtime.Eval(0, []string{`return {file_open("startup-owned", "w+tn"), sqlite_open(":memory:")};`})
	if outcome.Result.Flow != types.FlowReturn || outcome.Result.Val.Type() != types.TYPE_LIST || outcome.Result.Val.Len() != 2 {
		t.Fatalf("open real handles through runtime: %+v", outcome)
	}
	handles := outcome.Result.Val.Elements()
	ctx := task.NewContext()
	ctx.IsWizard = true
	for index, name := range []string{"file_name", "sqlite_info"} {
		result, found := builtinSession.CallByNameWithExecution(name, builtinSession.NewExecution(ctx, nil), []types.Value{handles[index]})
		if !found || result.IsError() {
			t.Fatalf("%s before Start: found=%v result=%+v", name, found, result)
		}
	}
	err = s.Start()
	var listenError *net.OpError
	if !errors.As(err, &listenError) || listenError.Op != "listen" {
		t.Fatalf("Start = %v, want native listen error", err)
	}
	for index, name := range []string{"file_name", "sqlite_info"} {
		result, found := builtinSession.CallByNameWithExecution(name, builtinSession.NewExecution(ctx, nil), []types.Value{handles[index]})
		if !found || result.Error != types.E_INVARG {
			t.Errorf("%s after failed Start: found=%v result=%+v, want E_INVARG", name, found, result)
		}
	}
	if s.running || len(s.connManager.ListenerInfos()) != 0 {
		t.Fatalf("failed Start retained server/listeners: running=%v listeners=%v", s.running, s.connManager.ListenerInfos())
	}
}

func TestStartRetainsReservedListener(t *testing.T) {
	s, err := NewServer(filepath.Join(t.TempDir(), "unused.db"), []listener.Spec{{
		Protocol: listener.ProtocolTCP, Interface: "127.0.0.1", Port: 0,
	}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	release, err := s.ReserveListeners()
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	port := s.connManager.GetListenPort()
	if port == 0 {
		t.Fatal("reservation did not bind a real port")
	}
	// Use the concrete allocated port so an attempted rebind in Start fails.
	s.listenerSpecs[0].Port = int64(port)
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	if ln, err := net.Listen("tcp", addr); err == nil {
		ln.Close()
		t.Fatal("game port is not held during startup")
	}
	// Exercise actual Start and shutdown against a disposable in-memory world.
	s.store = dbstore.NewStore()
	addTestObject(t, s.store, 0, dbstore.FlagWizard)
	s.runtime = engine.NewRuntime(s.store)
	s.input = NewInputProcessor(s.store, s.runtime)
	s.input.SetConnectionManager(s.connManager)
	s.cancel()
	if err := s.Start(); err != nil {
		t.Fatalf("Start did not retain reserved socket: %v", err)
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("shutdown did not release reservation: %v", err)
	}
	ln.Close()
}
