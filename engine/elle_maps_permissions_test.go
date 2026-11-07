package engine

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/config"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/internal/isolationprobe"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// Each case changes exactly one observed row from {} to {TOKEN}. The reader
// observes that row before the writer reads its marker and publishes; it then
// appends to the marker. A missing dependency makes both miss the other's write.
// Internal map entries not named by the row are deliberately not observations.
func elleMapPermissionHistories(t *testing.T, store *dbstore.Store, recorder *elleRecorder) []string {
	t.Helper()
	fault := isolationprobe.ActiveFault()
	switch fault {
	case "", "map-key-metadata", "map-key-presence", "map-key-whole-read":
	default:
		// Existing faults retain their own bounded witness schedules. Running
		// unrelated scenarios under them can corrupt later fixture state.
		return nil
	}
	cases := []struct {
		name, setup, read, write string
		ordinary                 bool
	}{
		{
			name:  "map-keyed-vs-whole",
			read:  `v=#0.MAP["a"];`,
			write: `#0.MAP=["a" -> {TOKEN}, "b" -> {}];`,
		},
		{
			name:  "map-whole-vs-keyed",
			read:  `m=#0.MAP; v=m["b"];`,
			write: `#0.MAP["b"]={TOKEN};`,
		},
		{
			name:  "map-whole-after-keyed",
			read:  `#0.MAP["a"]={TEMP}; m=#0.MAP; v=m["b"];`,
			write: `#0.MAP["b"]={TOKEN};`,
		},
		{
			name:  "map-absent-entry",
			read:  `try v=#0.MAP["missing"]; except (E_RANGE) v={}; endtry`,
			write: `#0.MAP["missing"]={TOKEN};`,
		},
		{
			name:  "map-inherited-entry",
			setup: `p=create(#0); add_property(p,"elle_mp_value",["a" -> {}],{#0,"rw"}); c=create(p); add_property(#0,"PARENT",p,{#0,"rw"}); add_property(#0,"CHILD",c,{#0,"rw"});`,
			read:  `v=#0.CHILD.elle_mp_value["a"];`,
			write: `#0.PARENT.elle_mp_value["a"]={TOKEN};`,
		},
		{
			name:  "map-clear-override",
			setup: `p=create(#0); add_property(p,"elle_mp_value",["a" -> {TOKEN}],{#0,"rw"}); c=create(p); c.elle_mp_value=["a" -> {}]; add_property(#0,"CHILD",c,{#0,"rw"});`,
			read:  `v=#0.CHILD.elle_mp_value["a"];`,
			write: `clear_property(#0.CHILD,"elle_mp_value");`,
		},
		{
			name:     "map-permission-revoke",
			read:     `try v=#0.MAP["a"]; except (E_PERM) v={TOKEN}; endtry`,
			write:    `set_property_info(#0,"MAP",{#0,""});`,
			ordinary: true,
		},
		{
			name:     "map-owner-revoke",
			setup:    `set_property_info(#0,"MAP",{#47,""});`,
			read:     `try v=#0.MAP["a"]; except (E_PERM) v={TOKEN}; endtry`,
			write:    `set_property_info(#0,"MAP",{#0,""});`,
			ordinary: true,
		},
	}
	var keys []string
	for i, scenario := range cases {
		switch fault {
		case "map-key-metadata":
			if scenario.name != "map-permission-revoke" && scenario.name != "map-owner-revoke" {
				continue
			}
		case "map-key-presence":
			if scenario.name != "map-absent-entry" {
				continue
			}
		case "map-key-whole-read":
			if scenario.name != "map-whole-after-keyed" {
				continue
			}
		}
		row, marker := scenario.name, scenario.name+"-marker"
		keys = append(keys, row, marker)
		token := 600000 + i*2
		replace := strings.NewReplacer("MAP", fmt.Sprintf("elle_mp_map%d", i),
			"PARENT", fmt.Sprintf("elle_mp_parent%d", i), "CHILD", fmt.Sprintf("elle_mp_child%d", i),
			"TOKEN", fmt.Sprint(token), "TEMP", fmt.Sprint(token+1))
		mapName, markerName := replace.Replace("MAP"), fmt.Sprintf("elle_mp_marker%d", i)
		read, published := make(chan struct{}), make(chan struct{})
		var readOnce, publishedOnce sync.Once
		var step builtins.BuiltinFunc = func(ctx *builtins.Execution, args []types.Value) types.Result {
			wait := read
			switch args[0].Int() {
			case 1:
				readOnce.Do(func() { close(read) })
				wait = published
			case 2:
				ctx.PendingEffects = append(ctx.PendingEffects, task.PendingEffect{
					Kind:  task.PendingEffectAsyncStart,
					Start: func() { publishedOnce.Do(func() { close(published) }) },
				})
				return types.Ok(types.NewInt(0))
			}
			select {
			case <-wait:
				return types.Ok(types.NewInt(0))
			case <-time.After(10 * time.Second):
				return types.Err(types.E_QUOTA)
			}
		}
		var observe builtins.BuiltinFunc = recorder.observe
		rt := newTestRuntimeWithWorkersAndBuiltins(t, store, config.Options{AdmissionLimit: 4}, 4,
			testBuiltinSlot("elle_observe", 3, 3, []int64{2, 2, -1}, &observe),
			testBuiltinSlot("elle_map_step", 1, 1, []int64{int64(types.TYPE_INT)}, &step))
		func() {
			defer rt.Stop()
			id := int64(900000 + i*10)
			setup := fmt.Sprintf(`add_property(#0,%q,["a" -> {},"b" -> {}],{#0,"rw"}); add_property(#0,%q,{},{#0,"rw"});`, mapName, markerName)
			elleDrain(t, rt, []*Task{elleTask(t, rt, id, strings.Join([]string{setup, replace.Replace(scenario.setup)}, "\n"))})
			reader := elleTask(t, rt, id+1, strings.Join([]string{
				replace.Replace(scenario.read),
				fmt.Sprintf(`elle_observe("r",%q,v); elle_map_step(1); m=#0.%s; elle_observe("r",%q,m); #0.%s={@m,%d}; elle_observe("append",%q,%d);`, row, markerName, marker, markerName, token+1, marker, token+1),
			}, "\n"))
			if scenario.ordinary {
				reader.Owner, reader.Programmer = 47, 47
				reader.Context.Player, reader.Context.Programmer, reader.Context.IsWizard = 47, 47, false
			}
			writer := elleTask(t, rt, id+2, strings.Join([]string{
				fmt.Sprintf(`elle_map_step(0); m=#0.%s; elle_observe("r",%q,m);`, markerName, marker),
				replace.Replace(scenario.write),
				fmt.Sprintf(`elle_observe("append",%q,%d); elle_map_step(2);`, row, token),
			}, "\n"))
			elleDrain(t, rt, []*Task{reader, writer})
			final := elleTask(t, rt, id+3, strings.Join([]string{
				replace.Replace(scenario.read),
				fmt.Sprintf(`elle_observe("r",%q,v); if (!equal(v,{%d})) raise(E_INVARG); endif elle_observe("r",%q,#0.%s);`, row, token, marker, markerName),
			}, "\n"))
			if scenario.ordinary {
				final.Owner, final.Programmer = 47, 47
				final.Context.Player, final.Context.Programmer, final.Context.IsWizard = 47, 47, false
			}
			elleDrain(t, rt, []*Task{final})
		}()
	}
	return keys
}
