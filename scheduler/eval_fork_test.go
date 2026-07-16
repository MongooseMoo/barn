package scheduler

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"barn/builtins"
	"barn/command"
	"barn/compiler"
	dbformat "barn/db/format"
	dbstore "barn/db/store"
	"barn/types"
)

type evalCommandStubConn struct {
	sent []string
}

func (c *evalCommandStubConn) Send(message string) error {
	c.sent = append(c.sent, message)
	return nil
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
func (c *evalCommandStubConn) ListenerPort() int64       { return 0 }

type evalCommandStubConnManager struct {
	player types.ObjID
	conn   *evalCommandStubConn
}

func (m *evalCommandStubConnManager) GetConnection(player types.ObjID) builtins.Connection {
	if player == m.player {
		return m.conn
	}
	return nil
}
func (m *evalCommandStubConnManager) ConnectedPlayers(showAll bool) []types.ObjID {
	return []types.ObjID{m.player}
}
func (m *evalCommandStubConnManager) BootPlayer(player types.ObjID) error { return nil }
func (m *evalCommandStubConnManager) RecyclePlayer(player types.ObjID) error {
	return nil
}
func (m *evalCommandStubConnManager) SwitchPlayer(oldPlayer, newPlayer types.ObjID) error {
	return nil
}
func (m *evalCommandStubConnManager) GetListenPort() int { return 0 }
func (m *evalCommandStubConnManager) ListenerInfos() []builtins.ListenerInfo {
	return nil
}
func (m *evalCommandStubConnManager) AddListener(spec builtins.ListenerSpec) (builtins.ListenerDescriptor, error) {
	return builtins.ListenerDescriptor{}, nil
}
func (m *evalCommandStubConnManager) RemoveListener(desc builtins.ListenerDescriptor) error {
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

	s := NewScheduler(store)
	defer s.Stop()

	lines := s.EvalCommandOutput(3, fmt.Sprintf(
		"fork id (0) "+
			"#%d:suspender(); "+
			"endfork "+
			"suspend(0); "+
			"s = task_stack(id);\n"+
			"kill_task(id);\n"+
			"return typeof(s);\n",
		obj,
	), "-=!-^-!=-", "-=!-v-!=-")

	if len(lines) != 3 {
		t.Fatalf("lines = %#v, want prefix/result/suffix", lines)
	}
	if lines[1] != "{1, 4}" {
		t.Fatalf("eval result = %q, want {1, 4}", lines[1])
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
	s := NewScheduler(store)
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

	s := NewScheduler(store)
	defer s.Stop()
	lines := s.EvalCommandOutput(player, `
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
`, "", "")

	if len(lines) != 1 || lines[0] != "{1, {1, 1}}" {
		t.Fatalf("lines = %#v, want {1, {1, 1}}", lines)
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

	s := NewScheduler(store)
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

	s := NewScheduler(store)
	defer s.Stop()
	prog, diagnostics := compiler.CompileMOO(strings.Split(source, "\n"), s.registry)
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
