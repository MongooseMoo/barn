package server

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/config"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/engine"
	"github.com/MongooseMoo/barn/types"
	"github.com/MongooseMoo/barn/vm"
)

type preloginReadProbe struct {
	*TCPTransport
	reads  int
	parked chan struct{}
}

func (p *preloginReadProbe) ReadInput() (string, bool, error) {
	p.reads++
	if p.reads == 2 {
		close(p.parked)
	}
	return p.TCPTransport.ReadInput()
}

func TestPreloginBinaryOutputDoesNotExpireLogin(t *testing.T) {
	store := dbstore.NewStore()
	addTestObject(t, store, 0, dbstore.FlagWizard|dbstore.FlagProgrammer)
	addTestObject(t, store, 2, dbstore.FlagWizard|dbstore.FlagProgrammer|dbstore.FlagUser)
	parked := make(chan struct{})
	registry, err := builtins.NewRegistryFromDescriptors(config.DefaultCapabilities(), append(vm.Descriptors(), builtins.Descriptor{
		Name: "wait_for_reader", Signature: &builtins.Signature{MinArgs: 0, MaxArgs: 0}, Visibility: builtins.Hidden,
		Effect: builtins.Transactional, Capability: config.Core, Implementation: func(_ *builtins.Execution, _ []types.Value) types.Result {
			select {
			case <-parked:
				return types.Ok(types.NewInt(1))
			case <-time.After(2 * time.Second):
				return types.Err(types.E_QUOTA)
			}
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	rt := engine.NewRuntimeWithRegistry(store, config.DefaultOptions(), registry)
	defer rt.Stop()
	p := NewInputProcessor(store, rt)
	cm := NewConnectionManager(7777)
	p.SetConnectionManager(cm)
	setTestConnectionManager(rt.Session(), cm)
	// This is Mongoose's $telnet:send_raw sequence, before the account read().
	addTestVerb(store, 0, "do_login_command", `
if (!argstr) return 0; endif
wait_for_reader();
try
  set_connection_option(player, "binary", 1);
  notify(player, "~FF~FB~46");
finally
  set_connection_option(player, "binary", 0);
endtry
notify(player, "login-prompt");
line = read();
notify(player, "login-read: " + line);
return 0;`)
	server, client := net.Pipe()
	transport := &preloginReadProbe{TCPTransport: NewTCPTransport(server), parked: parked}
	conn := cm.NewConnectionFromTransport(transport)
	p.Start()
	defer p.Stop()
	done := make(chan struct{})
	go func() { p.HandleConnection(conn); close(done) }()
	defer func() {
		_ = client.Close()
		_ = conn.Close()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("reader did not stop")
		}
	}()
	_ = client.SetDeadline(time.Now().Add(3 * time.Second))
	reader := bufio.NewReader(client)
	if _, err := fmt.Fprintln(client, "hello"); err != nil {
		t.Fatal(err)
	}
	if line, err := reader.ReadString('\n'); err != nil || !strings.Contains(line, "login-prompt") {
		t.Fatalf("login prompt: %q, %v", line, err)
	}
	if _, err := fmt.Fprintln(client, "answer"); err != nil {
		t.Fatalf("binary output expired the prelogin reader: %v", err)
	}
	if line, err := reader.ReadString('\n'); err != nil || !strings.Contains(line, "login-read: answer") {
		t.Fatalf("binary output expired the prelogin reader: %q, %v", line, err)
	}
}
