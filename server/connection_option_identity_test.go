package server

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/command"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/engine"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func connectionOptionIdentityFixture(t *testing.T) (*engine.Runtime, *ConnectionManager, *InputProcessor) {
	t.Helper()
	store := dbstore.NewStore()
	addTestObject(t, store, 0, dbstore.FlagWizard)
	addTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagProgrammer)
	addTestObject(t, store, 3, dbstore.FlagUser|dbstore.FlagProgrammer)
	rt := engine.NewRuntime(store)
	cm := NewConnectionManager(7777)
	processor := NewInputProcessor(store, rt)
	processor.SetConnectionManager(cm)
	setTestConnectionManager(rt.Session(), cm)
	t.Cleanup(func() {
		processor.Stop()
		rt.Stop()
		if err := rt.Session().Close(); err != nil {
			t.Error(err)
		}
	})
	return rt, cm, processor
}

func requireConnectionOptionEval(t *testing.T, rt *engine.Runtime, source, want string) {
	t.Helper()
	outcome := rt.Eval(0, []string{source})
	if len(outcome.Diagnostics) != 0 || outcome.Panic != nil || outcome.Result.Flow != types.FlowReturn || outcome.Result.Val.String() != want {
		t.Fatalf("eval = %+v, want %s", outcome, want)
	}
}

type pausedOptionConnManager struct {
	*ConnectionManager
	captured chan builtins.Connection
	release  <-chan struct{}
	once     sync.Once
}

func (m *pausedOptionConnManager) GetConnection(player types.ObjID) builtins.Connection {
	conn := m.ConnectionManager.GetConnection(player)
	m.once.Do(func() {
		m.captured <- conn
		<-m.release
	})
	return conn
}

func TestAdmittedOptionSetterCannotRecreateDisconnectedBag(t *testing.T) {
	rt, cm, processor := connectionOptionIdentityFixture(t)
	conn := cm.NewConnectionFromTransport(stubTransport{})
	t.Cleanup(func() { _ = conn.Close() })
	if err := cm.SwitchPlayer(types.ObjID(-conn.ID), 2); err != nil {
		t.Fatal(err)
	}
	// Disconnect cleanup must not invoke a runtime hook while the paused setter
	// owns the irreversible builtin gate.
	conn.setDisconnectHook("")
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseSetter := func() { releaseOnce.Do(func() { close(release) }) }
	manager := &pausedOptionConnManager{ConnectionManager: cm, captured: make(chan builtins.Connection, 1), release: release}
	setTestConnectionManager(rt.Session(), manager)
	finished := make(chan struct{})
	var outcome engine.EvalOutcome
	// Install release/join cleanup before launching the setter, including all
	// early failure paths while waiting for actual manager admission.
	t.Cleanup(func() {
		releaseSetter()
		select {
		case <-finished:
		case <-time.After(5 * time.Second):
			t.Error("admitted setter did not join after release")
		}
	})
	go func() {
		defer close(finished)
		outcome = rt.Eval(0, []string{`return set_connection_option(#2, "binary", 1);`})
	}()
	select {
	case captured := <-manager.captured:
		if captured != conn {
			t.Fatalf("admission captured %v, want actual connection %v", captured, conn)
		}
	case <-finished:
		t.Fatalf("setter finished before capture: %+v", outcome)
	case <-time.After(5 * time.Second):
		t.Fatal("setter did not reach real connection lookup")
	}
	processor.processDisconnect(command.InputEvent{ConnID: conn.ID, IsDisconnect: true})
	releaseSetter()
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("setter did not finish after disconnect and release")
	}
	if len(outcome.Diagnostics) != 0 || outcome.Panic != nil || outcome.Result.Flow != types.FlowReturn || outcome.Result.Val.Type() != types.TYPE_INT || outcome.Result.Val.Int() != 0 {
		t.Fatalf("already-admitted setter = %+v, want normal zero result", outcome)
	}
	if rt.Session().ConnectionOptionTruthy(conn.ID, "binary") {
		t.Fatal("admitted setter recreated the disconnected physical connection's binary option bag")
	}
}

func TestSwitchPlayerPreservesCommittedConnectionOptions(t *testing.T) {
	rt, cm, processor := connectionOptionIdentityFixture(t)
	conn := cm.NewConnectionFromTransport(stubTransport{})
	defer conn.Close()
	if err := cm.SwitchPlayer(types.ObjID(-conn.ID), 2); err != nil {
		t.Fatal(err)
	}
	requireConnectionOptionEval(t, rt, `set_connection_option(#2, "binary", 1); switch_player(#2, #3); return 1;`, "1")
	requireConnectionOptionEval(t, rt, `value = connection_options(#3, "binary"); try old = connection_options(#2, "binary"); except (E_INVARG) old = E_INVARG; endtry return {value, old};`, "{1, E_INVARG}")
	processor.processDisconnect(command.InputEvent{ConnID: conn.ID, IsDisconnect: true})
	if rt.Session().ConnectionOptionTruthy(conn.ID, "binary") {
		t.Fatal("disconnect retained physical option entry")
	}
}

func TestConnectionOptionsFollowPreloginIdentityAndResetOnReconnect(t *testing.T) {
	rt, cm, processor := connectionOptionIdentityFixture(t)
	conn := cm.NewConnectionFromTransport(stubTransport{})
	defer conn.Close()
	requireConnectionOptionEval(t, rt, fmt.Sprintf(`set_connection_option(#%d, "binary", 1); return 1;`, -conn.ID), "1")
	if err := cm.SwitchPlayer(types.ObjID(-conn.ID), 2); err != nil {
		t.Fatal(err)
	}
	requireConnectionOptionEval(t, rt, `return connection_options(#2, "binary");`, "1")
	processor.processDisconnect(command.InputEvent{ConnID: conn.ID, IsDisconnect: true})
	if rt.Session().ConnectionOptionTruthy(conn.ID, "binary") {
		t.Fatal("closed prelogin connection retained options")
	}
	replacement := cm.NewConnectionFromTransport(stubTransport{})
	defer replacement.Close()
	if err := cm.SwitchPlayer(types.ObjID(-replacement.ID), 2); err != nil {
		t.Fatal(err)
	}
	requireConnectionOptionEval(t, rt, `return {connection_options(#2, "binary"), connection_options(#2, "client-echo")};`, "{0, 1}")
}

func TestStaleDisconnectForgetsOnlyItsPhysicalOptions(t *testing.T) {
	rt, cm, processor := connectionOptionIdentityFixture(t)
	old := cm.NewConnectionFromTransport(stubTransport{})
	defer old.Close()
	if err := cm.SwitchPlayer(types.ObjID(-old.ID), 2); err != nil {
		t.Fatal(err)
	}
	requireConnectionOptionEval(t, rt, `set_connection_option(#2, "binary", 1); return 1;`, "1")
	replacement := cm.NewConnectionFromTransport(stubTransport{})
	defer replacement.Close()
	if err := cm.SwitchPlayer(types.ObjID(-replacement.ID), 2); err != nil {
		t.Fatal(err)
	}
	requireConnectionOptionEval(t, rt, `return connection_options(#2, "binary");`, "0")
	requireConnectionOptionEval(t, rt, `set_connection_option(#2, "binary", 1); return 1;`, "1")
	processor.processDisconnect(command.InputEvent{ConnID: old.ID, IsDisconnect: true})
	if rt.Session().ConnectionOptionTruthy(old.ID, "binary") {
		t.Fatal("stale physical option entry survived disconnect")
	}
	requireConnectionOptionEval(t, rt, `return connection_options(#2, "binary");`, "1")
}

func TestConnectionOptionReadDoesNotWaitForOutputMutex(t *testing.T) {
	rt, cm, _ := connectionOptionIdentityFixture(t)
	peer := newShutdownPeer()
	peer.stalled = true
	conn := cm.NewConnectionFromTransport(shutdownTransport("TCP", peer))
	var workers sync.WaitGroup
	t.Cleanup(func() {
		_ = conn.Close()
		workers.Wait()
	})
	if err := cm.SwitchPlayer(types.ObjID(-conn.ID), 2); err != nil {
		t.Fatal(err)
	}
	requireConnectionOptionEval(t, rt, `set_connection_option(#2, "binary", 1); return 1;`, "1")
	writeDone := make(chan error, 1)
	workers.Add(1)
	go func() {
		defer workers.Done()
		writeDone <- conn.SendNotification(task.PendingNotification{Message: "blocked"})
	}()
	awaitShutdown(t, peer.written, "option fixture notification write entry")
	finished := make(chan engine.EvalOutcome, 1)
	workers.Add(1)
	go func() {
		defer workers.Done()
		finished <- rt.Eval(0, []string{`return connection_options(#2, "binary");`})
	}()
	outcome := awaitShutdown(t, finished, "option read during blocked notification")
	if len(outcome.Diagnostics) != 0 || outcome.Panic != nil || outcome.Result.Val.Int() != 1 {
		t.Fatalf("read during blocked output = %+v", outcome)
	}
	select {
	case err := <-writeDone:
		t.Fatalf("notification stopped before transport close: %v", err)
	default:
	}
	_ = conn.Close()
	if err := awaitShutdown(t, writeDone, "option fixture notification writer"); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("notification error = %v, want closed", err)
	}
}
