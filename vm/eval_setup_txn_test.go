package vm

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/compiler"
	dbformat "github.com/MongooseMoo/barn/db/format"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

const auditProxySetupSource = `
for prop in ({"audit_proxy_seen", "audit_proxy_login_saved", "audit_proxy_trusted_saved"})
  try
    add_property(#0, prop, {}, {#0, "rw"});
  except (E_INVARG)
  endtry
endfor
trusted_exists = 1;
try
  trusted_saved = #6.trusted_proxies;
except (E_PROPNF)
  trusted_exists = 0;
  trusted_saved = {};
endtry
#0.audit_proxy_trusted_saved = {trusted_exists, trusted_saved};
try
  add_property(#6, "trusted_proxies", {}, {#0, "rw"});
except (E_INVARG)
endtry
#6.trusted_proxies = {"::1", "127.0.0.1"};
old_login_exists = 1;
try
  old_login_info = verb_info(#0, "do_login_command");
  old_login_args = verb_args(#0, "do_login_command");
  old_login_code = verb_code(#0, "do_login_command");
except (E_VERBNF)
  old_login_exists = 0;
  old_login_info = {};
  old_login_args = {};
  old_login_code = {};
endtry
#0.audit_proxy_login_saved = {old_login_exists, old_login_info, old_login_args, old_login_code};
try
  add_verb(#0, {#0, "rxd", "do_login_command"}, {"this", "none", "this"});
except (E_INVARG)
endtry
set_verb_info(#0, "do_login_command", {player, "rxd", "do_login_command"});
set_verb_args(#0, "do_login_command", {"this", "none", "this"});
set_verb_code(#0, "do_login_command", {
  "#0.audit_proxy_seen = {args, argstr};",
  "return 0;"
});
return 1;
`

const auditWaifCallersSource = `
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

func TestAuditProxySetupSourceCommitsInTransaction(t *testing.T) {
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

	registry := BuildVMRegistry()
	prog, diagnostics := compiler.CompileMOO(strings.Split(auditProxySetupSource, "\n"), registry)
	if len(diagnostics) > 0 {
		t.Fatalf("CompileMOO failed: %v", diagnostics[0])
	}

	ctx := kernel.NewTaskContext()
	ctx.Player = player
	ctx.Programmer = player
	ctx.IsWizard = true
	ctx.Store = store
	ctx.StoreTxn = store.BeginReadOnly(0)
	taskValue := task.NewTask(1, player, 30000, 1)

	machine := NewVM(store, registry)
	machine.Context = ctx
	machine.Task = taskValue
	frame := machine.PrepareVerbFrame(prog, types.ObjNothing, player, player, "", types.ObjNothing, []types.Value{})
	SetLocalByName(frame, prog, "this", types.NewObj(types.ObjNothing))
	SetLocalByName(frame, prog, "player", types.NewObj(player))
	SetLocalByName(frame, prog, "caller", types.NewObj(types.ObjNothing))
	SetLocalByName(frame, prog, "verb", types.NewStr(""))
	SetLocalByName(frame, prog, "args", types.NewList(nil))
	SetLocalByName(frame, prog, "argstr", types.NewStr(""))
	SetLocalByName(frame, prog, "dobjstr", types.NewStr(""))
	SetLocalByName(frame, prog, "iobjstr", types.NewStr(""))
	SetLocalByName(frame, prog, "prepstr", types.NewStr(""))
	SetLocalByName(frame, prog, "dobj", types.NewObj(types.ObjNothing))
	SetLocalByName(frame, prog, "iobj", types.NewObj(types.ObjNothing))
	result := machine.ExecuteLoop()
	for result.Flow == types.FlowSuspend || result.Flow == types.FlowFork {
		if result.Flow == types.FlowFork {
			machine.SetForkResult(0)
		}
		result = machine.Resume()
	}
	if result.Flow == types.FlowException {
		t.Fatalf("setup flow exception: %v", result.Error)
	}
	if errCode := ctx.StoreTxn.Commit(); errCode != types.E_NONE {
		t.Fatalf("Commit failed: %v", errCode)
	}
}

func TestWaifCallersPreserveThisAndVerbLocation(t *testing.T) {
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

	registry := BuildVMRegistry()
	prog, diagnostics := compiler.CompileMOO(strings.Split(auditWaifCallersSource, "\n"), registry)
	if len(diagnostics) > 0 {
		t.Fatalf("CompileMOO failed: %v", diagnostics[0])
	}

	ctx := kernel.NewTaskContext()
	ctx.Player = player
	ctx.Programmer = player
	ctx.IsWizard = true
	ctx.Store = store
	ctx.StoreTxn = store.BeginReadOnly(0)
	taskValue := task.NewTask(1, player, 30000, 1)

	machine := NewVM(store, registry)
	machine.Context = ctx
	machine.Task = taskValue
	frame := machine.PrepareVerbFrame(prog, types.ObjNothing, player, player, "", types.ObjNothing, []types.Value{})
	SetLocalByName(frame, prog, "this", types.NewObj(types.ObjNothing))
	SetLocalByName(frame, prog, "player", types.NewObj(player))
	SetLocalByName(frame, prog, "caller", types.NewObj(types.ObjNothing))
	SetLocalByName(frame, prog, "verb", types.NewStr(""))
	SetLocalByName(frame, prog, "args", types.NewList(nil))
	SetLocalByName(frame, prog, "argstr", types.NewStr(""))
	SetLocalByName(frame, prog, "dobjstr", types.NewStr(""))
	SetLocalByName(frame, prog, "iobjstr", types.NewStr(""))
	SetLocalByName(frame, prog, "prepstr", types.NewStr(""))
	SetLocalByName(frame, prog, "dobj", types.NewObj(types.ObjNothing))
	SetLocalByName(frame, prog, "iobj", types.NewObj(types.ObjNothing))

	result := machine.ExecuteLoop()
	if result.Flow == types.FlowException {
		t.Fatalf("waif callers flow exception: %v val=%v stack=%#v", result.Error, result.Val, result.CallStack)
	}
	if result.Val.Type() != types.TYPE_LIST {
		t.Fatalf("result = %T %v, want list", result.Val, result.Val)
	}
	got := result.Val
	want := types.NewList([]types.Value{
		types.NewList([]types.Value{types.NewStr(":audit_b"), types.NewInt(1), types.NewInt(1)}),
		types.NewList([]types.Value{types.NewStr(":audit_c"), types.NewInt(1), types.NewInt(1)}),
	})
	if !got.Equal(want) {
		t.Fatalf("result = %v, want %v", got, want)
	}
}
