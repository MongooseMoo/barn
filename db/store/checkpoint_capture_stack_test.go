package store

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime/debug"
	"strings"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/types"
)

func TestCheckpointCaptureAnonymousChainUsesBoundedStack(t *testing.T) {
	const childMarker = "BARN_CHECKPOINT_STACK_CHILD"
	if os.Getenv(childMarker) != "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCheckpointCaptureAnonymousChainUsesBoundedStack$", "-test.v")
		cmd.Env = append(os.Environ(), childMarker+"=1")
		output, err := cmd.CombinedOutput()
		if err != nil {
			if len(output) > 2048 {
				output = output[:2048]
			}
			t.Fatalf("bounded-stack checkpoint subprocess: %v\n%s", err, output)
		}
		if !strings.Contains(string(output), "captured_anonymous=2000") {
			t.Fatalf("subprocess did not confirm complete graph: %s", output)
		}
		return
	}

	s := NewStore()
	const count = 2000
	for id := types.ObjID(0); id < count; id++ {
		obj := NewObject(id, 0)
		obj.anonymous = true
		if id+1 < count {
			obj.properties.put("next", NewProperty(types.NewAnon(id+1), 0, PropRead|PropWrite, false, true))
			obj.propOrder = []string{"next"}
			obj.propDefsCount = 1
		}
		s.AddAnonymous(obj)
	}
	// Limit only the disposable process, after fixture construction. Every MOO
	// value has depth one; anonymous object graph length must not consume stack.
	debug.SetMaxStack(128 << 10)
	capture := s.CaptureCheckpoint([]types.Value{types.NewAnon(0)})
	defer capture.Close()
	snapshot, _, err := capture.Materialize()
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.AnonymousObjects) != count {
		t.Fatalf("captured %d anonymous objects, want %d", len(snapshot.AnonymousObjects), count)
	}
	capture.Close()
	if n := s.activeFloorCount(); n != 0 {
		t.Fatalf("retained floors: %d", n)
	}
	fmt.Println("captured_anonymous=2000")
}
