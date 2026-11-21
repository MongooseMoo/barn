package dbtool

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/config"
	dbformat "github.com/MongooseMoo/barn/db/format"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// barnFrameTaskShapes are forked tasks, each suspended with no timeout. Those
// with a reason are kept in Barn's own frame form, the reason being part of
// the refusal Barn gives; the others Barn writes in Toast's form. #0.p's
// initialize, #0.rc's and #0.rc2's recycle, #0.room's exitfunc, #0.parent's
// pp and the WAIF class's _set_index suspend.
var barnFrameTaskShapes = []struct {
	name, body, reason string
}{
	{"plain suspend", "suspend();", ""},
	{"finally a raise entered", "try raise(E_PERM); finally suspend(); endtry", ""},
	{"eval", `eval("suspend();");`, ""},
	{"call_function eval", `call_function("eval", "suspend();");`, ""},
	{"move exitfunc", "move(#0.thing, #0.room2);", ""},
	{"pass", "#0.thing:pp();", ""},
	{"path-dependent clear", "n = 1; m = n; while ((n = suspend()) == 0) endwhile", "depends on the path into its block"},
	{"create", "create(#0.p);", "create: stock Toast does not resume"},
	{"call_function create", `call_function("create", #0.p);`, "create: stock Toast does not resume"},
	{"recreate", "recreate(#0.gone, #0.p);", "recreate: stock Toast does not resume"},
	{"recycle", "recycle(#0.rc);", "recycle: stock Toast does not resume"},
	{"call_function recycle", `call_function("recycle", #0.rc2);`, "recycle: stock Toast does not resume"},
	{"WAIF index set", "w = #0.wv; w[1] = 2;", "is not paused at a call"},
}

// The objects and verbs the shapes use. Property names avoid the builtin
// properties (r, w, f, ...).
var barnFrameTaskWorld = []string{
	`p = create(#-1);`,
	`add_verb(p, {#3, "xd", "initialize"}, {"this", "none", "this"});`,
	`set_verb_code(p, "initialize", {"suspend();"});`,
	`add_property(#0, "p", p, {#3, "rw"});`,
	`gone = create(#-1);`,
	`recycle(gone);`,
	`add_property(#0, "gone", gone, {#3, "rw"});`,
	`for name in ({"rc", "rc2"})`,
	`  o = create(#-1);`,
	`  add_verb(o, {#3, "xd", "recycle"}, {"this", "none", "this"});`,
	`  set_verb_code(o, "recycle", {"suspend();"});`,
	`  add_property(#0, name, o, {#3, "rw"});`,
	`endfor`,
	`room = create(#-1);`,
	`add_verb(room, {#3, "xd", "exitfunc"}, {"this", "none", "this"});`,
	`set_verb_code(room, "exitfunc", {"suspend();"});`,
	`parent = create(#-1);`,
	`add_verb(parent, {#3, "xd", "pp"}, {"this", "none", "this"});`,
	`set_verb_code(parent, "pp", {"suspend();", "return 1;"});`,
	`thing = create(parent);`,
	`add_verb(thing, {#3, "xd", "pp"}, {"this", "none", "this"});`,
	`set_verb_code(thing, "pp", {"return pass(@args);"});`,
	`move(thing, room);`,
	`add_property(#0, "thing", thing, {#3, "rw"});`,
	`add_property(#0, "room2", create(#-1), {#3, "rw"});`,
	`wc = create(#-1);`,
	`add_verb(wc, {#3, "xd", "new"}, {"this", "none", "this"});`,
	`set_verb_code(wc, "new", {"return new_waif();"});`,
	`add_verb(wc, {#3, "xd", ":_set_index"}, {"this", "none", "this"});`,
	`set_verb_code(wc, ":_set_index", {"suspend();"});`,
	`add_property(#0, "wv", wc:new(), {#3, "rw"});`,
}

// writeBarnFrameTaskCheckpoint has a runtime start every barnFrameTaskShapes
// task and writes the checkpoint a running Barn server writes for them. It
// returns the checkpoint path.
func writeBarnFrameTaskCheckpoint(t *testing.T) string {
	t.Helper()
	store := evalStore(t)
	session, err := NewEvalSession(store, config.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := session.Close(); err != nil {
			t.Errorf("close eval session: %v", err)
		}
	}()
	outcome := session.runtime.Eval(3, append(append([]string(nil), barnFrameTaskWorld...), "return 1;"))
	if !completed(outcome) || outcome.Result.Flow != types.FlowReturn {
		t.Fatalf("setup eval: %+v", outcome)
	}
	// One program per fork: Toast clears only variable slots below 32, so a
	// shape's variables must not follow the world's.
	for _, shape := range barnFrameTaskShapes {
		outcome := session.runtime.Eval(3, []string{"fork (0)", "  " + shape.body, "endfork", "return 1;"})
		if !completed(outcome) || outcome.Result.Flow != types.FlowReturn {
			t.Fatalf("%s eval: %+v", shape.name, outcome)
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		queued, suspended := session.runtime.TaskSnapshots()
		settled := len(queued) == 0 && len(suspended) == len(barnFrameTaskShapes)
		for _, snapshot := range suspended {
			settled = settled && snapshot.State == task.TaskSuspended
		}
		if settled {
			source := filepath.Join(t.TempDir(), "barn-frame-tasks.db")
			if err := dbformat.WriteCheckpoint(source, store, queued, suspended, nil); err != nil {
				t.Fatalf("write source checkpoint: %v", err)
			}
			return source + ".new"
		}
		if time.Now().After(deadline) {
			t.Fatalf("tasks never settled: %d queued, %d suspended", len(queued), len(suspended))
		}
		session.runtime.ProcessReadyTasks()
	}
}

var omittedLine = regexp.MustCompile(`(?m)^omitted task (\d+): #-?\d+:\S*, this #-?\d+: (.+)$`)

// --toast-loadable on a Barn-written file leaves out each task Barn kept in its
// own frame form, names it with its reason, and keeps every task in Toast's
// form; without the flag every task is kept. With BARN_TOAST_LOADABLE_KEEP set
// to a directory, the Barn-written file, its converted copy and the command's
// output are copied there, for loading in Toast.
func TestOfflineCheckpointToastLoadableOmitsOnlyBarnFormTasks(t *testing.T) {
	source := writeBarnFrameTaskCheckpoint(t)
	written, err := dbformat.LoadDatabaseReadOnly(source)
	if err != nil {
		t.Fatal(err)
	}
	var barnForm, toastForm []int64
	for _, saved := range written.SuspendedTasks {
		if saved.Snapshot.VM.Frames[0].Native == nil {
			barnForm = append(barnForm, saved.Snapshot.ID)
		} else {
			toastForm = append(toastForm, saved.Snapshot.ID)
		}
	}
	wantBarn := 0
	for _, shape := range barnFrameTaskShapes {
		if shape.reason != "" {
			wantBarn++
		}
	}
	if len(barnForm) != wantBarn || len(toastForm) != len(barnFrameTaskShapes)-wantBarn {
		t.Fatalf("Barn wrote %d tasks in its own form and %d in Toast's, want %d and %d", len(barnForm), len(toastForm), wantBarn, len(barnFrameTaskShapes)-wantBarn)
	}

	var out bytes.Buffer
	if err := WriteOfflineCheckpoint(&out, source, config.DefaultOptions(), false, true, ""); err != nil {
		t.Fatalf("toast-loadable checkpoint: %v", err)
	}
	var omitted []int64
	var reasons []string
	for _, match := range omittedLine.FindAllStringSubmatch(out.String(), -1) {
		id, err := strconv.ParseInt(match[1], 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		omitted = append(omitted, id)
		reasons = append(reasons, match[2])
	}
	slices.Sort(omitted)
	slices.Sort(barnForm)
	if !slices.Equal(omitted, barnForm) {
		t.Fatalf("omitted %v, want the Barn-form tasks %v; output:\n%s", omitted, barnForm, out.String())
	}
	for _, shape := range barnFrameTaskShapes {
		if shape.reason != "" && !slices.ContainsFunc(reasons, func(reason string) bool { return strings.Contains(reason, shape.reason) }) {
			t.Errorf("no omitted task gives the %s reason %q; output:\n%s", shape.name, shape.reason, out.String())
		}
	}
	want := fmt.Sprintf(": 0 queued, %d suspended or interrupted tasks, 0 connections, %d tasks omitted for Toast", len(toastForm), len(barnForm))
	if !strings.Contains(out.String(), want) {
		t.Fatalf("output lacks the written counts %q:\n%s", want, out.String())
	}
	loaded, err := dbformat.LoadDatabaseReadOnly(source + ".new")
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	var kept []int64
	for _, saved := range loaded.SuspendedTasks {
		if saved.Snapshot.VM.Frames[0].Native == nil {
			t.Fatalf("Toast-loadable copy holds task %d in Barn's frame form", saved.Snapshot.ID)
		}
		kept = append(kept, saved.Snapshot.ID)
	}
	slices.Sort(kept)
	slices.Sort(toastForm)
	if !slices.Equal(kept, toastForm) {
		t.Fatalf("Toast-loadable copy holds tasks %v, want every Toast-form task %v", kept, toastForm)
	}
	if keep := os.Getenv("BARN_TOAST_LOADABLE_KEEP"); keep != "" {
		for _, file := range []string{source, source + ".new"} {
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(keep, filepath.Base(file)), data, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.WriteFile(filepath.Join(keep, "convert-output.txt"), out.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := WriteOfflineCheckpoint(&out, source, config.DefaultOptions(), false, false, ""); err != nil {
		t.Fatalf("plain checkpoint: %v", err)
	}
	all, err := dbformat.LoadDatabaseReadOnly(source + ".new")
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(all.SuspendedTasks) != len(barnFrameTaskShapes) {
		t.Fatalf("checkpoint without --toast-loadable holds %d tasks, want all %d", len(all.SuspendedTasks), len(barnFrameTaskShapes))
	}
}
