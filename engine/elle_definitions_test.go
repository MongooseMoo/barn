package engine

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/builtins"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/internal/isolationprobe"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// Every definition scenario has one state transition, from {} to {TOKEN}.
// Presence/deletion observations normalize the real lookup result just as the
// valid/recycle pair does. Dispatch and property values return the list itself.
// There is no inferred clock, internal version, or task result in the history.
type elleDefinitionScenario struct {
	name, setup, read, write string
	memo                     bool
}

var elleDefinitionScenarios = []elleDefinitionScenario{
	{name: "property-absent", read: "v=`#O.p ! E_PROPNF => {}';", write: `add_property(#O,"p",{TOKEN},{#0,"rw"});`},
	{name: "property-scan-add", read: `v="p" in properties(#O) ? {TOKEN} | {};`, write: `add_property(#O,"p",{TOKEN},{#0,"rw"});`},
	{name: "property-scan-delete", setup: `add_property(#O,"p",0,{#0,"rw"});`, read: `v="p" in properties(#O) ? {} | {TOKEN};`, write: `delete_property(#O,"p");`},
	{name: "property-scan-rename", setup: `add_property(#O,"old",0,{#0,"rw"});`, read: `v="new" in properties(#O) ? {TOKEN} | {};`, write: `set_property_info(#O,"old",{#0,"rw","new"});`},
	{name: "inherited-override", setup: `add_property(#P,"p",{},{#0,"rw"}); chparent(#O,#P);`, read: `v=#O.p;`, write: `#O.p={TOKEN};`},
	{name: "inherited-clear", setup: `add_property(#P,"p",{TOKEN},{#0,"rw"}); chparent(#O,#P); #O.p={};`, read: `v=#O.p;`, write: `clear_property(#O,"p");`},
	{name: "inherited-reparent", setup: `add_property(#P,"p",{},{#0,"rw"}); add_property(#Q,"p",{TOKEN},{#0,"rw"}); chparent(#O,#P);`, read: `v=#O.p;`, write: `chparent(#O,#Q);`},
	{name: "verb-absent", read: `v=respond_to(#O,"probe") ? {TOKEN} | {};`, write: `add_verb(#O,{#0,"rxd","probe"},{"this","none","none"});`},
	{name: "verb-scan-add", read: `v="probe" in verbs(#O) ? {TOKEN} | {};`, write: `add_verb(#O,{#0,"rxd","probe"},{"this","none","none"});`},
	{name: "verb-scan-delete", setup: `add_verb(#O,{#0,"rxd","probe"},{"this","none","none"});`, read: `v="probe" in verbs(#O) ? {} | {TOKEN};`, write: `delete_verb(#O,"probe");`},
	{name: "verb-scan-rename", setup: `add_verb(#O,{#0,"rxd","old"},{"this","none","none"});`, read: `v="new" in verbs(#O) ? {TOKEN} | {};`, write: `set_verb_info(#O,"old",{#0,"rxd","new"});`},
	{name: "dispatch-code", setup: `add_verb(#O,{#0,"rxd","probe"},{"this","none","none"}); set_verb_code(#O,"probe",{"return {};"});`, read: `v=#O:probe();`, write: `set_verb_code(#O,"probe",{"return {TOKEN};"});`},
	{name: "dispatch-shadow", setup: `add_verb(#P,{#0,"rxd","probe"},{"this","none","none"}); set_verb_code(#P,"probe",{"return {};"}); chparent(#O,#P);`, read: `v=#O:probe();`, write: `add_verb(#O,{#0,"rxd","probe"},{"this","none","none"}); set_verb_code(#O,"probe",{"return {TOKEN};"});`, memo: true},
	{name: "dispatch-delete", setup: `add_verb(#P,{#0,"rxd","probe"},{"this","none","none"}); set_verb_code(#P,"probe",{"return {TOKEN};"}); chparent(#O,#P); add_verb(#O,{#0,"rxd","probe"},{"this","none","none"}); set_verb_code(#O,"probe",{"return {};"});`, read: `v=#O:probe();`, write: `delete_verb(#O,"probe");`},
	{name: "dispatch-reparent", setup: `add_verb(#P,{#0,"rxd","probe"},{"this","none","none"}); set_verb_code(#P,"probe",{"return {};"}); add_verb(#Q,{#0,"rxd","probe"},{"this","none","none"}); set_verb_code(#Q,"probe",{"return {TOKEN};"}); chparent(#O,#P);`, read: `v=#O:probe();`, write: `chparent(#O,#Q);`, memo: true},
	{name: "verb-index-shift", setup: `add_verb(#O,{#0,"rxd","first"},{"this","none","none"}); add_verb(#O,{#0,"rxd","second"},{"this","none","none"});`, read: `v=verb_info(#O,1)[3] == "second" ? {TOKEN} | {};`, write: `delete_verb(#O,1);`},
}

func elleDefinitionWorld(t *testing.T, store *dbstore.Store) {
	t.Helper()
	if ec := store.DirectTxn().DefineProperty(0, "elle_definition_ledger", dbstore.NewProperty(types.NewList(nil), 0, dbstore.PropRead|dbstore.PropWrite, false, true)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	for index := range elleDefinitionScenarios {
		for offset := 0; offset < 3; offset++ {
			b := dbstore.NewObjectBuilder(types.ObjID(400 + index*3 + offset))
			b.SetOwner(0)
			b.SetFlags(dbstore.FlagRead | dbstore.FlagWrite | dbstore.FlagFertile)
			if err := store.Add(b.Build()); err != nil {
				t.Fatal(err)
			}
		}
	}
}

type elleDefinitionPair struct {
	read, published         chan struct{}
	readOnce, publishedOnce sync.Once
	name                    string
	memo                    bool
	memoBefore              uint64
}

type elleDefinitions struct {
	pair *elleDefinitionPair
}

func (d *elleDefinitions) step(ctx *builtins.Execution, args []types.Value) types.Result {
	pair := d.pair
	wait := pair.read
	switch args[0].Int() {
	case 1:
		// The mutator is still waiting on read, so no other task can have warmed
		// the memo since this pair's baseline. This tests the observer's path.
		if pair.memo && isolationprobe.Snapshot()["verb_memo"] <= pair.memoBefore {
			return types.Err(types.E_INVARG)
		}
		pair.readOnce.Do(func() { close(pair.read) })
		wait = pair.published
	case 2:
		ctx.PendingEffects = append(ctx.PendingEffects, task.PendingEffect{
			Kind:  task.PendingEffectAsyncStart,
			Start: func() { pair.publishedOnce.Do(func() { close(pair.published) }) },
		})
		return types.Ok(types.NewInt(0))
	case 3:
		ctx.PendingEffects = append(ctx.PendingEffects, task.PendingEffect{
			Kind:  task.PendingEffectAsyncStart,
			Start: func() { isolationprobe.Note("definition/" + pair.name) },
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

func (d *elleDefinitions) run(t *testing.T, rt *Runtime, id *int64) {
	t.Helper()
	for index, scenario := range elleDefinitionScenarios {
		row := "definition-" + scenario.name
		token := 81000000 + index
		object := 400 + index*3
		replace := strings.NewReplacer("#O", fmt.Sprintf("#%d", object), "#P", fmt.Sprintf("#%d", object+1), "#Q", fmt.Sprintf("#%d", object+2), "TOKEN", fmt.Sprint(token))
		read := replace.Replace(scenario.read) + fmt.Sprintf(`elle_observe("r","%s",v);`, row)
		if scenario.setup != "" {
			elleDrain(t, rt, []*Task{elleTask(t, rt, *id, replace.Replace(scenario.setup))})
			*id++
		}
		// Warm on the committed fixture: a task that just changed verb shape
		// cannot populate the store dispatch memo from its private definitions.
		elleDrain(t, rt, []*Task{elleTask(t, rt, *id, read+`if (!equal(v,{})) raise(E_INVARG); endif`)})
		*id++
		d.pair = &elleDefinitionPair{read: make(chan struct{}), published: make(chan struct{}), name: scenario.name, memo: scenario.memo, memoBefore: isolationprobe.Snapshot()["verb_memo"]}
		// Stage the observer's property append first for ordinary scenarios. A
		// warmed dispatch then uses precise ancestry scans; the memo scenarios
		// deliberately resolve before their first write to exercise clock validation.
		appendProperty := fmt.Sprintf(`p=#0.elle_definition_ledger; elle_observe("r","definition-ledger",p); #0.elle_definition_ledger={@p,%d}; elle_observe("append","definition-ledger",%d);`, 80000000+index, 80000000+index)
		observer := appendProperty + read + `elle_definition(1); elle_definition(3);`
		if scenario.memo {
			observer = read + `elle_definition(1);` + appendProperty + `elle_definition(3);`
		}
		writer := `elle_definition(0);` + `v=#0.elle_definition_ledger; elle_observe("r","definition-ledger",v);` + read + replace.Replace(scenario.write) + fmt.Sprintf(`elle_observe("append","%s",%d); elle_definition(2);`, row, token)
		elleDrain(t, rt, []*Task{elleTask(t, rt, *id, observer), elleTask(t, rt, *id+1, writer)})
		*id += 2
		elleDrain(t, rt, []*Task{elleTask(t, rt, *id, read+fmt.Sprintf(`if (!equal(v,{%d})) raise(E_INVARG); endif`, token))})
		*id++
		if isolationprobe.Snapshot()["definition/"+scenario.name] == 0 {
			t.Errorf("definition scenario did not commit: %s", scenario.name)
		}
	}
}
