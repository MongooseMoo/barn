package format

import (
	"bytes"
	"github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
	"os"
	"strings"
	"testing"
)

// TestRoundTripPreservesEmptyVerbCodeSection is the B6 regression guard.
//
// Toast (v17) emits one verb-program entry (#obj:verbidx + source + ".") for
// every verb that has a program, INCLUDING verbs whose program source is empty
// (a verb that has had set_verb_code applied with an empty body). A verb that
// was merely add_verb'd and never compiled has NO entry. Barn previously gated
// the verb-code section on len(Code)>0, dropping empty-but-present programs and
// undercounting (1949 vs Toast's 1950 on the canonical toastcore.db, the
// missing one being #10:special_action).
//
// This test builds those three cases and asserts the empty-program verb keeps
// its program entry across load->write->load, distinct from the never-programmed
// verb which must NOT gain one.
func TestRoundTripPreservesEmptyVerbCodeSection(t *testing.T) {
	s := store.NewStore()

	objID, errCode := s.CreateObject(nil, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject failed: %v", errCode)
	}

	mkVerb := func(name string) store.Verb {
		return store.NewVerb(name, []string{name}, 0, store.VerbPerms(0),
			store.VerbArgs{This: "none", Prep: "none", That: "none"}, nil)
	}

	// 0: real program; 1: empty program (set_verb_code with {}); 2: never programmed.
	if _, errCode = s.AddVerb(objID, mkVerb("with_code")); errCode != types.E_NONE {
		t.Fatalf("AddVerb(with_code): %v", errCode)
	}
	if _, errCode = s.AddVerb(objID, mkVerb("empty_program")); errCode != types.E_NONE {
		t.Fatalf("AddVerb(empty_program): %v", errCode)
	}
	if _, errCode = s.AddVerb(objID, mkVerb("never_programmed")); errCode != types.E_NONE {
		t.Fatalf("AddVerb(never_programmed): %v", errCode)
	}

	if errCode = s.SetVerbCodeByIndex(objID, 0, []string{"return 1;"}); errCode != types.E_NONE {
		t.Fatalf("SetVerbCode(with_code): %v", errCode)
	}
	// Empty program: set_verb_code with an empty body. This must still produce a
	// program entry on write even though the source is empty.
	if errCode = s.SetVerbCodeByIndex(objID, 1, []string{}); errCode != types.E_NONE {
		t.Fatalf("SetVerbCode(empty_program): %v", errCode)
	}
	// index 2 (never_programmed) is intentionally left unprogrammed.

	// Sanity: the snapshot must reflect the three HasProgram states.
	snap := s.Snapshot()
	obj := snap.Objects[objID]
	if obj == nil {
		t.Fatalf("object #%d missing from snapshot", objID)
	}
	if len(obj.VerbList) != 3 {
		t.Fatalf("verb count = %d, want 3", len(obj.VerbList))
	}
	if !obj.VerbList[0].HasProgram {
		t.Fatalf("with_code: HasProgram = false, want true")
	}
	if !obj.VerbList[1].HasProgram {
		t.Fatalf("empty_program: HasProgram = false, want true (empty body is still a program)")
	}
	if obj.VerbList[2].HasProgram {
		t.Fatalf("never_programmed: HasProgram = true, want false")
	}

	// Round-trip: write then reload.
	tmpFile, err := os.CreateTemp(t.TempDir(), "verb-program-*.db")
	if err != nil {
		t.Fatalf("CreateTemp failed: %v", err)
	}
	defer tmpFile.Close()

	writer := NewWriter(tmpFile, snap)
	if err := writer.WriteDatabase(); err != nil {
		t.Fatalf("WriteDatabase failed: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	reloaded, err := LoadDatabase(tmpFile.Name())
	if err != nil {
		t.Fatalf("Reload failed: %v", err)
	}
	rStore := reloaded.NewStoreFromDatabase()
	rSnap := rStore.Snapshot()
	rObj := rSnap.Objects[objID]
	if rObj == nil {
		t.Fatalf("reloaded object #%d missing", objID)
	}
	if len(rObj.VerbList) != 3 {
		t.Fatalf("reloaded verb count = %d, want 3", len(rObj.VerbList))
	}

	// The empty program must survive: HasProgram stays true, Code stays empty.
	if !rObj.VerbList[1].HasProgram {
		t.Fatalf("reloaded empty_program: HasProgram = false, want true (program entry was dropped on round-trip)")
	}
	if len(rObj.VerbList[1].Code) != 0 {
		t.Fatalf("reloaded empty_program: Code = %v, want empty", rObj.VerbList[1].Code)
	}
	// The never-programmed verb must NOT have gained a program entry.
	if rObj.VerbList[2].HasProgram {
		t.Fatalf("reloaded never_programmed: HasProgram = true, want false (spurious program entry)")
	}
	// The real program must round-trip its source.
	if !rObj.VerbList[0].HasProgram || len(rObj.VerbList[0].Code) != 1 || rObj.VerbList[0].Code[0] != "return 1;" {
		t.Fatalf("reloaded with_code: HasProgram=%v Code=%v, want true {return 1;}",
			rObj.VerbList[0].HasProgram, rObj.VerbList[0].Code)
	}
}

func TestWriteVerbCodeSectionsIncludesAnonymousObjects(t *testing.T) {
	var buf bytes.Buffer
	writer := NewWriter(&buf, store.Snapshot{
		AnonymousObjects: []*store.SnapshotObject{{
			ID:        41,
			Anonymous: true,
			VerbList: []store.VerbView{{
				Code:       []string{"return 1;"},
				HasProgram: true,
			}},
		}},
	})

	if err := writer.writeVerbCodeSections(); err != nil {
		t.Fatalf("writeVerbCodeSections: %v", err)
	}
	if err := writer.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	const want = "1\n#41:0\nreturn 1;\n.\n"
	if got := buf.String(); !strings.Contains(got, want) {
		t.Fatalf("anonymous verb program missing from output:\n%s", got)
	}
}

func TestWriteVerbCodeSectionsUsesToastCanonicalProgramText(t *testing.T) {
	var buf bytes.Buffer
	writer := NewWriter(&buf, store.Snapshot{AllObjects: []*store.SnapshotObject{{
		ID: 0,
		VerbList: []store.VerbView{{
			HasProgram: true,
			Code: []string{
				"waif = 1;",
				"anon = 2;",
				"fork (0)",
				"  state = {waif, anon};",
				"endfork",
			},
		}},
	}}})
	if err := writer.writeVerbCodeSections(); err != nil {
		t.Fatalf("writeVerbCodeSections: %v", err)
	}
	if err := writer.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	want := strings.Join([]string{
		"1",
		"#0:0",
		"WAIF = 1;",
		"ANON = 2;",
		"fork (0)",
		"state = {WAIF, ANON};",
		"endfork",
		".",
		"",
	}, "\n")
	if got := buf.String(); got != want {
		t.Fatalf("verb code section = %q, want %q", got, want)
	}
}
