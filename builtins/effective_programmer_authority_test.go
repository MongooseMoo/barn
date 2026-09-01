package builtins

import (
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

func loweredPermsExecution(t *testing.T) *Execution {
	t.Helper()
	store := dbstore.NewStore()
	for _, spec := range []struct {
		id    types.ObjID
		owner types.ObjID
		flags dbstore.ObjectFlags
	}{
		{id: 0, owner: 0, flags: dbstore.FlagWizard},
		{id: 2, owner: 2, flags: dbstore.FlagProgrammer},
		{id: 3, owner: 3},
	} {
		builder := dbstore.NewObjectBuilder(spec.id)
		builder.SetOwner(spec.owner)
		builder.SetFlags(spec.flags)
		if err := store.Add(builder.Build()); err != nil {
			t.Fatalf("add object #%d: %v", spec.id, err)
		}
	}
	ctx := newTestExecution()
	ctx.Store = store
	ctx.StoreTxn = store.BeginReadOnly(0)
	ctx.Player = 0
	ctx.Programmer = 2
	ctx.IsWizard = false
	return ctx
}

func TestLoweredTaskPermissionsRejectPlayerWizardAuthority(t *testing.T) {
	tests := []struct {
		name string
		call func(*Execution) types.Result
	}{
		{
			name: "create with another owner",
			call: func(ctx *Execution) types.Result {
				return builtinCreate(ctx, []types.Value{types.NewObj(2), types.NewObj(3)})
			},
		},
		{
			name: "crypt with custom rounds",
			call: func(ctx *Execution) types.Result {
				return builtinCrypt(ctx, []types.Value{types.NewStr("password"), types.NewStr("$5$rounds=1001$salt")})
			},
		},
		{
			name: "object_bytes",
			call: func(ctx *Execution) types.Result {
				return builtinObjectBytes(ctx, []types.Value{types.NewObj(2)})
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := test.call(loweredPermsExecution(t))
			if !result.IsError() || result.Error != types.E_PERM {
				t.Fatalf("result = %+v, want E_PERM", result)
			}
		})
	}
}

func TestRespondToUsesTransactionAwareObjectReadAuthority(t *testing.T) {
	ctx := loweredPermsExecution(t)
	verb := dbstore.NewVerb("secret", []string{"secret"}, 0, dbstore.VerbRead|dbstore.VerbExecute, dbstore.VerbArgs{}, nil)
	if _, errCode := ctx.Store.AddVerb(3, verb); errCode != types.E_NONE {
		t.Fatalf("add verb: %s", errCode)
	}
	ctx.StoreTxn = ctx.Store.BeginReadOnly(0)

	result := builtinRespondTo(ctx, []types.Value{types.NewObj(3), types.NewStr("secret")})
	if result.IsError() || result.Val.Type() != types.TYPE_INT || result.Val.Int() != 1 {
		t.Fatalf("unreadable object result = %+v, want summary 1", result)
	}

	if errCode := ctx.StoreTxn.SetObjectFlag(3, dbstore.FlagRead, true); errCode != types.E_NONE {
		t.Fatalf("stage read flag: %s", errCode)
	}
	result = builtinRespondTo(ctx, []types.Value{types.NewObj(3), types.NewStr("secret")})
	if result.IsError() || result.Val.Type() != types.TYPE_LIST {
		t.Fatalf("staged-readable object result = %+v, want detailed list", result)
	}
}

func TestLoweredTaskPermissionsUseProgrammerForConnectionAuthority(t *testing.T) {
	ctx := ctxWithConnManager(&stubConnManager{conn: &stubConn{remote: "127.0.0.1:7777"}})
	ctx.Player = 7
	ctx.Programmer = 8
	ctx.IsWizard = false
	ctx.Session.setConnectionOption(7, "binary", types.NewInt(0))

	tests := []struct {
		name string
		call func() types.Result
	}{
		{"flush_input", func() types.Result { return builtinFlushInput(ctx, []types.Value{types.NewObj(7)}) }},
		{"force_input", func() types.Result {
			return builtinForceInput(ctx, []types.Value{types.NewObj(7), types.NewStr("look")})
		}},
		{"buffered_output_length", func() types.Result { return builtinBufferedOutputLength(ctx, []types.Value{types.NewObj(7)}) }},
		{"connection_options", func() types.Result { return builtinConnectionOptions(ctx, []types.Value{types.NewObj(7)}) }},
		{"output_delimiters", func() types.Result { return builtinOutputDelimiters(ctx, []types.Value{types.NewObj(7)}) }},
		{"boot_player", func() types.Result { return builtinBootPlayer(ctx, []types.Value{types.NewObj(7)}) }},
		{"set_connection_option", func() types.Result {
			return builtinSetConnectionOption(ctx, []types.Value{types.NewObj(7), types.NewStr("binary"), types.NewInt(1)})
		}},
		{"connection_option", func() types.Result {
			return builtinConnectionOption(ctx, []types.Value{types.NewObj(7), types.NewStr("binary")})
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := test.call()
			if !result.IsError() || result.Error != types.E_PERM {
				t.Fatalf("result = %+v, want E_PERM", result)
			}
		})
	}
}
