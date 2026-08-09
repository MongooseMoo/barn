package builtins

import (
	"testing"

	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/types"
)

func TestCryptRejectsWorkAboveServerLimits(t *testing.T) {
	cacheServerOptionsDefaults()
	t.Cleanup(cacheServerOptionsDefaults)

	ctx := kernel.NewTaskContext()
	ctx.IsWizard = true
	for _, salt := range []string{
		"$2y$15$......................",
		"$5$rounds=1000001$salt",
		"$6$rounds=999999999$salt",
	} {
		result := builtinCrypt(ctx, []types.Value{types.NewStr("password"), types.NewStr(salt)})
		if result.Flow != types.FlowException || result.Error != types.E_QUOTA {
			t.Errorf("crypt with salt %q = flow %v, error %s; want E_QUOTA", salt, result.Flow, result.Error)
		}
	}
}

func TestCryptChargesTicksBeforeHashing(t *testing.T) {
	cacheServerOptionsDefaults()
	t.Cleanup(cacheServerOptionsDefaults)

	ctx := kernel.NewTaskContext()
	ctx.IsWizard = true
	ctx.TicksRemaining = 4
	result := builtinCrypt(ctx, []types.Value{
		types.NewStr("password"),
		types.NewStr("$5$rounds=5000$salt"),
	})
	if result.Flow != types.FlowException || result.Error != types.E_QUOTA {
		t.Fatalf("crypt exceeding remaining ticks = flow %v, error %s; want E_QUOTA", result.Flow, result.Error)
	}
	if ctx.TicksRemaining != 4 {
		t.Fatalf("rejected crypt changed remaining ticks to %d, want 4", ctx.TicksRemaining)
	}
}

func TestCryptWorkLimitsApplyFromServerOptionsSnapshot(t *testing.T) {
	snapshot := defaultServerOptionsSnapshot()
	snapshot.MaxCryptBcryptCost = 8
	snapshot.MaxCryptSHARounds = 20_000
	applyServerOptionsSnapshot(&snapshot)
	t.Cleanup(cacheServerOptionsDefaults)

	bcryptCost, shaRounds := GetCryptWorkLimits()
	if bcryptCost != 8 || shaRounds != 20_000 {
		t.Fatalf("crypt limits = (%d, %d), want (8, 20000)", bcryptCost, shaRounds)
	}
}

func TestCryptUsesPendingServerOptions(t *testing.T) {
	cacheServerOptionsDefaults()
	t.Cleanup(cacheServerOptionsDefaults)

	snapshot := defaultServerOptionsSnapshot()
	snapshot.MaxCryptBcryptCost = 4
	ctx := kernel.NewTaskContext()
	ctx.IsWizard = true
	ctx.PendingEffects = []kernel.PendingEffect{{
		Kind:          kernel.PendingEffectServerOptions,
		ServerOptions: snapshot,
	}}
	result := builtinCrypt(ctx, []types.Value{
		types.NewStr("password"),
		types.NewStr("$2y$05$......................"),
	})
	if result.Flow != types.FlowException || result.Error != types.E_QUOTA {
		t.Fatalf("crypt above pending limit = flow %v, error %s; want E_QUOTA", result.Flow, result.Error)
	}
}
