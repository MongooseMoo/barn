package engine

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/command"
	dbformat "github.com/MongooseMoo/barn/db/format"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/internal/listener"
	"github.com/MongooseMoo/barn/types"
)

type evalCommandStubConn struct {
	sent    []string
	sendErr error
}

func (c *evalCommandStubConn) Send(message string) error {
	c.sent = append(c.sent, message)
	return c.sendErr
}
func (c *evalCommandStubConn) Buffer(message string)     {}
func (c *evalCommandStubConn) Flush() error              { return nil }
func (c *evalCommandStubConn) RemoteAddr() string        { return "" }
func (c *evalCommandStubConn) GetOutputPrefix() string   { return "" }
func (c *evalCommandStubConn) GetOutputSuffix() string   { return "" }
func (c *evalCommandStubConn) BufferedOutputLength() int { return 0 }
func (c *evalCommandStubConn) ConnectedSeconds() int64   { return 0 }
func (c *evalCommandStubConn) IdleSeconds() int64        { return 0 }
func (c *evalCommandStubConn) GetResolvedName() string   { return "" }
func (c *evalCommandStubConn) ListenerPort() (int64, bool) {
	return 0, false
}

type evalCommandStubConnManager struct {
	player           types.ObjID
	conn             *evalCommandStubConn
	disconnectOnBoot bool
}

func (m *evalCommandStubConnManager) GetConnection(player types.ObjID) builtins.Connection {
	if player == m.player && m.conn != nil {
		return m.conn
	}
	return nil
}
func (m *evalCommandStubConnManager) ConnectedPlayers(showAll bool) []types.ObjID {
	return []types.ObjID{m.player}
}
func (m *evalCommandStubConnManager) BootPlayer(player types.ObjID) error {
	if m.disconnectOnBoot && player == m.player && m.conn != nil {
		_ = m.conn.Send("*** Disconnected ***")
		m.conn = nil
	}
	return nil
}
func (m *evalCommandStubConnManager) RecyclePlayer(player types.ObjID) error {
	return nil
}
func (m *evalCommandStubConnManager) SwitchPlayer(oldPlayer, newPlayer types.ObjID) error {
	return nil
}
func (m *evalCommandStubConnManager) GetListenPort() int { return 0 }
func (m *evalCommandStubConnManager) ListenerInfos() []listener.Info {
	return nil
}
func (m *evalCommandStubConnManager) AddListener(spec listener.Spec) (listener.Descriptor, error) {
	return listener.Descriptor{}, nil
}
func (m *evalCommandStubConnManager) RemoveListener(desc listener.Descriptor) error {
	return nil
}
func (m *evalCommandStubConnManager) OpenNetworkConnection(host string, port int64) (types.ObjID, error) {
	return types.ObjNothing, nil
}
func (m *evalCommandStubConnManager) ConnectionNameLookup(player types.ObjID, rewrite bool) (string, error) {
	return "", nil
}

const auditWaifCommandSource = `
obj = create($waif);
add_verb(obj, {player, "xd", ":audit_a"}, {"this", "none", "this"});
set_verb_code(obj, ":audit_a", {"return callers();"});
add_verb(obj, {player, "xd", ":audit_b"}, {"this", "none", "this"});
set_verb_code(obj, ":audit_b", {"return this:audit_a();"});
add_verb(obj, {player, "xd", ":audit_c"}, {"this", "none", "this"});
set_verb_code(obj, ":audit_c", {
  "c = this:audit_b();",
  "return {{c[1][2], typeof(c[1][1]) == WAIF, c[1][4] == this.class}, {c[2][2], typeof(c[2][1]) == WAIF, c[2][4] == this.class}};"
});
waif = obj:new();
result = waif:audit_c();
recycle(obj);
return result;
`

func TestEvalForkedSuspenderCanBeInspectedWithTaskStack(t *testing.T) {
	store := dbstore.NewStore()
	wizard := dbstore.NewObjectBuilder(3)
	wizard.SetOwner(3)
	wizard.SetFlags(dbstore.FlagWizard | dbstore.FlagProgrammer | dbstore.FlagUser)
	if err := store.Add(wizard.Build()); err != nil {
		t.Fatalf("Add wizard failed: %v", err)
	}
	obj, errCode := store.CreateObject(nil, 3, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject failed: %v", errCode)
	}
	verb := dbstore.NewVerb(
		"suspender",
		[]string{"suspender"},
		3,
		dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "this"},
		[]string{"suspend(100);", "return 42;"},
	)
	if _, errCode := store.AddVerb(obj, verb); errCode != types.E_NONE {
		t.Fatalf("AddVerb failed: %v", errCode)
	}

	s := NewRuntime(store)
	defer s.Stop()

	line := s.EvalCommandOutput(3, fmt.Sprintf(
		"fork id (0) "+
			"#%d:suspender(); "+
			"endfork "+
			"suspend(0); "+
			"s = task_stack(id);\n"+
			"kill_task(id);\n"+
			"return typeof(s);\n",
		obj,
	))

	if line != "{1, 4}" {
		t.Fatalf("eval result = %q, want {1, 4}", line)
	}
}

func TestCommandEvalWaifCallersPreserveThisAndVerbLocation(t *testing.T) {
	database, err := dbformat.LoadDatabase(filepath.Join("..", "Test_conf.db"))
	if err != nil {
		t.Fatalf("LoadDatabase failed: %v", err)
	}
	store := database.NewStoreFromDatabase()
	player, errCode := store.CreateObject([]types.ObjID{1}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject player failed: %v", errCode)
	}
	for _, flag := range []dbstore.ObjectFlags{dbstore.FlagWizard, dbstore.FlagProgrammer, dbstore.FlagUser, dbstore.FlagRead, dbstore.FlagWrite} {
		if errCode := store.SetObjectFlag(player, flag, true); errCode != types.E_NONE {
			t.Fatalf("SetObjectFlag %v failed: %v", flag, errCode)
		}
	}
	if errCode := store.MoveObject(player, 2, 0); errCode != types.E_NONE {
		t.Fatalf("MoveObject player failed: %v", errCode)
	}

	conn := &evalCommandStubConn{}
	s := NewRuntime(store)
	defer s.Stop()
	s.Registry().SetConnectionManager(&evalCommandStubConnManager{player: player, conn: conn})

	cmd := command.ParseCommand("eval " + auditWaifCommandSource)
	match := command.FindVerb(store, player, 2, cmd)
	if match == nil {
		t.Fatalf("FindVerb eval returned nil")
	}

	s.ExecuteVerbTaskSync(player, match, cmd, "")

	want := []string{
		"-=!-^-!=-",
		"{1, {{\":audit_b\", 1, 1}, {\":audit_c\", 1, 1}}}",
		"-=!-v-!=-",
	}
	if len(conn.sent) != len(want) {
		t.Fatalf("sent = %#v, want %#v", conn.sent, want)
	}
	for i := range want {
		if conn.sent[i] != want[i] {
			t.Fatalf("sent = %#v, want %#v", conn.sent, want)
		}
	}
}

func TestMoveAcceptUsesCurrentTaskTransaction(t *testing.T) {
	database, err := dbformat.LoadDatabase(filepath.Join("..", "Test_conf.db"))
	if err != nil {
		t.Fatalf("LoadDatabase failed: %v", err)
	}
	store := database.NewStoreFromDatabase()
	player, errCode := store.CreateObject([]types.ObjID{1}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject player failed: %v", errCode)
	}
	for _, flag := range []dbstore.ObjectFlags{dbstore.FlagWizard, dbstore.FlagProgrammer, dbstore.FlagUser, dbstore.FlagRead, dbstore.FlagWrite} {
		if errCode := store.SetObjectFlag(player, flag, true); errCode != types.E_NONE {
			t.Fatalf("SetObjectFlag %v failed: %v", flag, errCode)
		}
	}

	s := NewRuntime(store)
	defer s.Stop()
	line := s.EvalCommandOutput(player, `
try
  add_property(#0, "audit_move_accept_seen", $nothing, {#0, "rw"});
except (E_INVARG)
endtry
thing = create($nothing);
dest = create($nothing);
add_verb(dest, {player, "xd", "accept"}, {"this", "none", "this"});
set_verb_code(dest, "accept", {
  "#0.audit_move_accept_seen = args[1];",
  "return 0;"
});
move(thing, dest);
result = {#0.audit_move_accept_seen == thing, thing.location == dest};
recycle(thing);
recycle(dest);
delete_property(#0, "audit_move_accept_seen");
return result;
`)

	if line != "{1, {1, 1}}" {
		t.Fatalf("line = %q, want {1, {1, 1}}", line)
	}
}

func TestCommandEvalChparentPropertyResetUsesTransactionReseed(t *testing.T) {
	database, err := dbformat.LoadDatabase(filepath.Join("..", "Test_conf.db"))
	if err != nil {
		t.Fatalf("LoadDatabase failed: %v", err)
	}
	store := database.NewStoreFromDatabase()
	player, errCode := store.CreateObject([]types.ObjID{1}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject player failed: %v", errCode)
	}
	for _, flag := range []dbstore.ObjectFlags{dbstore.FlagWizard, dbstore.FlagProgrammer, dbstore.FlagUser, dbstore.FlagRead, dbstore.FlagWrite} {
		if errCode := store.SetObjectFlag(player, flag, true); errCode != types.E_NONE {
			t.Fatalf("SetObjectFlag %v failed: %v", flag, errCode)
		}
	}

	s := NewRuntime(store)
	defer s.Stop()
	conn := &evalCommandStubConn{}
	s.Registry().SetConnectionManager(&evalCommandStubConnManager{player: player, conn: conn})
	source := `
a = create($nothing);
b = create($nothing);
c = create($nothing);
add_property(a, "foo", "foo", {player, "c"});
add_property(b, "foo", "foo", {b, ""});
chparent(c, a);
c.foo = "bar";
chparent(c, b);
pi = property_info(c, "foo");
return {pi[1] == b && pi[2] == "", c.foo == "foo"};
`
	code := strings.Join(strings.FieldsFunc(source, func(r rune) bool {
		return r == '\n' || r == '\r'
	}), " ")
	cmd := command.ParseCommand("eval " + code)
	match := command.FindVerb(store, player, 2, cmd)
	if match == nil {
		t.Fatalf("FindVerb eval returned nil")
	}
	s.ExecuteVerbTaskSync(player, match, cmd, "")

	want := []string{
		"-=!-^-!=-",
		"{1, {1, 1}}",
		"-=!-v-!=-",
	}
	if len(conn.sent) != len(want) {
		t.Fatalf("sent = %#v, want %#v", conn.sent, want)
	}
	for i := range want {
		if conn.sent[i] != want[i] {
			t.Fatalf("sent = %#v, want %#v", conn.sent, want)
		}
	}
}

func TestCommandEvalRunGCCyclicAnonymousChainCompletes(t *testing.T) {
	for _, length := range []int{3, 5, 6} {
		t.Run(fmt.Sprintf("length_%d", length), func(t *testing.T) {
			database, err := dbformat.LoadDatabase(filepath.Join("..", "Test_conf.db"))
			if err != nil {
				t.Fatalf("LoadDatabase failed: %v", err)
			}
			store := database.NewStoreFromDatabase()
			s := NewRuntime(store)
			defer s.Stop()

			conn := &evalCommandStubConn{}
			s.Registry().SetConnectionManager(&evalCommandStubConnManager{player: 3, conn: conn})
			source := fmt.Sprintf(`
c = head = tail = #-1;
try
  c = create($nothing);
  add_property(c, "next", 0, {player, ""});
  x = y = create(c, 1);
  head = tail = x;
  for i in [1..%d]
    x.next = create(c, 1);
    x = x.next;
    tail = x;
  endfor
  x.next = y;
  run_gc();
  recycle(y);
  x = y = 0;
  head = tail = #-1;
  run_gc();
  recycle(c);
  run_gc();
  return 1;
finally
  if (valid(tail))
    recycle(tail);
  endif
  if (valid(head))
    recycle(head);
  endif
  if (valid(c))
    recycle(c);
  endif
endtry
`, length)
			code := strings.Join(strings.FieldsFunc(source, func(r rune) bool {
				return r == '\n' || r == '\r'
			}), " ")
			cmd := command.ParseCommand("eval " + code)
			match := command.FindVerb(store, 3, 2, cmd)
			if match == nil {
				t.Fatal("FindVerb eval returned nil")
			}
			s.ExecuteVerbTaskSync(3, match, cmd, "")

			want := []string{"-=!-^-!=-", "{1, 1}", "-=!-v-!=-"}
			if len(conn.sent) != len(want) {
				t.Fatalf("sent = %#v, want %#v", conn.sent, want)
			}
			for i := range want {
				if conn.sent[i] != want[i] {
					t.Fatalf("sent = %#v, want %#v", conn.sent, want)
				}
			}
		})
	}
}

func TestForkedZeroDelayResumeCommitsPostSuspendWrites(t *testing.T) {
	store := dbstore.NewStore()
	wizard := dbstore.NewObjectBuilder(0)
	wizard.SetOwner(0)
	wizard.SetFlags(dbstore.FlagWizard | dbstore.FlagProgrammer | dbstore.FlagUser | dbstore.FlagRead | dbstore.FlagWrite)
	if err := store.Add(wizard.Build()); err != nil {
		t.Fatalf("Add wizard failed: %v", err)
	}
	if errCode := store.DefineProperty(0, "fork_value", dbstore.NewProperty(types.NewList(nil), 0, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty failed: %v", errCode)
	}

	source := `
set_task_local({"parent", 7});
fork (0)
  suspend(0);
  #0.fork_value = task_local();
endfork
suspend(0);
suspend(0);
return #0.fork_value;
`

	s := NewRuntime(store)
	defer s.Stop()
	prog, diagnostics := s.registry.Compiler().CompileMOO(strings.Split(source, "\n"))
	if len(diagnostics) > 0 {
		t.Fatalf("CompileMOO failed: %v", diagnostics[0])
	}
	s.CreateForegroundTask(0, prog)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if s.ProcessReadyTasks() == 0 {
			time.Sleep(5 * time.Millisecond)
		}
		value, errCode := store.PropertyValue(0, "fork_value")
		if errCode != types.E_NONE {
			t.Fatalf("PropertyValue failed: %v", errCode)
		}
		if value.Type() == types.TYPE_MAP {
			return
		}
	}
	value, _ := store.PropertyValue(0, "fork_value")
	t.Fatalf("fork_value = %T %v, want empty map from fork task_local()", value, value)
}
