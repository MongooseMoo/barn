package format

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

func TestCheckpointRecoveryKeepsPairWhenRollbackAlsoFails(t *testing.T) {
	directory := t.TempDir()
	old := []types.Value{types.NewWaif(9, 3)}
	fresh := []types.Value{types.NewWaif(9, 3), types.NewWaif(9, 3)}
	out := publicationFixture(t, filepath.Join(directory, "out"), old)
	staged := publicationFixture(t, filepath.Join(directory, "next"), fresh)
	rollbackFailure := errors.New("injected rollback failure")
	publicationFailed := false
	fs := checkpointIO{rename: func(from, to string) error {
		if to == out+waifIdentitySidecarSuffix {
			publicationFailed = true
			return os.ErrPermission
		}
		if publicationFailed && to == out {
			return rollbackFailure
		}
		return renameCheckpointFile(from, to)
	}, syncDirectory: syncParentDirectory}
	err := publishCheckpointPair(out, staged, fs)
	if !errors.Is(err, os.ErrPermission) || !errors.Is(err, rollbackFailure) {
		t.Fatalf("lost publication/rollback causes: %v", err)
	}
	_, generation, err := readCheckpointJournal(out)
	if err != nil {
		t.Fatal(err)
	}
	assertCheckpointIdentities(t, filepath.Join(generation, "previous"), old)
	assertCheckpointIdentities(t, filepath.Join(generation, "next"), fresh)
	// Load uses production recovery, without the injected persistent failures.
	assertCheckpointIdentities(t, out, old)
	if _, err := os.Stat(out + checkpointJournalSuffix); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("journal remains: %v", err)
	}
}

func TestCheckpointPublicationDirectorySyncFailures(t *testing.T) {
	for failure := 1; failure <= 6; failure++ {
		t.Run(strconv.Itoa(failure), func(t *testing.T) {
			directory := t.TempDir()
			old := []types.Value{types.NewWaif(9, 3)}
			fresh := []types.Value{types.NewWaif(9, 3), types.NewWaif(9, 3)}
			out := publicationFixture(t, filepath.Join(directory, "out"), old)
			staged := publicationFixture(t, filepath.Join(directory, "next"), fresh)
			calls := 0
			fs := checkpointIO{rename: renameCheckpointFile, syncDirectory: func(path string) error {
				calls++
				if calls == failure {
					return os.ErrPermission
				}
				return syncParentDirectory(path)
			}}
			if err := publishCheckpointPair(out, staged, fs); !errors.Is(err, os.ErrPermission) {
				t.Fatalf("sync error = %v", err)
			}
			want := old
			if failure >= 4 {
				want = fresh
			}
			assertCheckpointIdentities(t, out, want)
		})
	}
}

func TestCheckpointRecoveryDistinguishesIdentitiesInIdenticalPortableDumps(t *testing.T) {
	directory := t.TempDir()
	old := []types.Value{types.NewWaif(9, 3)}
	fresh := []types.Value{types.NewWaif(9, 3)}
	out := publicationFixture(t, filepath.Join(directory, "out"), old)
	staged := publicationFixture(t, filepath.Join(directory, "next"), fresh)
	oldBytes, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	newBytes, err := os.ReadFile(staged)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(oldBytes, newBytes) {
		t.Fatal("fixture dumps differ; identity-only window was not exercised")
	}
	failed := false
	fs := checkpointIO{rename: func(from, to string) error {
		if to == out+waifIdentitySidecarSuffix && !failed {
			failed = true
			return os.ErrPermission
		}
		return renameCheckpointFile(from, to)
	}, syncDirectory: syncParentDirectory}
	if err := publishCheckpointPair(out, staged, fs); !errors.Is(err, os.ErrPermission) {
		t.Fatal(err)
	}
	assertCheckpointIdentities(t, out, old)
}

func TestCheckpointRecoveryRejectsCorruptSavedPairAndUnjournaledMismatch(t *testing.T) {
	directory := t.TempDir()
	old := []types.Value{types.NewWaif(9, 3)}
	fresh := []types.Value{types.NewWaif(9, 3), types.NewWaif(9, 3)}
	out := publicationFixture(t, filepath.Join(directory, "out"), old)
	staged := publicationFixture(t, filepath.Join(directory, "next"), fresh)
	fs := checkpointIO{rename: func(from, to string) error {
		if to == out+waifIdentitySidecarSuffix {
			return os.ErrPermission
		}
		return renameCheckpointFile(from, to)
	}, syncDirectory: syncParentDirectory}
	if err := publishCheckpointPair(out, staged, fs); !errors.Is(err, os.ErrPermission) {
		t.Fatal(err)
	}
	_, generation, err := readCheckpointJournal(out)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(generation, "previous")+waifIdentitySidecarSuffix, []byte("corrupt\n"), 0600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if err := RecoverCheckpoint(out); err == nil {
		t.Fatal("corrupt recovery pair accepted")
	}
	after, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("invalid recovery changed database")
	}
	if err := os.Remove(out + checkpointJournalSuffix); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadDatabase(out); err == nil {
		t.Fatal("unjournaled mismatch was ignored")
	}
}

func TestCheckpointRecoveryRejectsInvalidJournalWithoutTouchingFiles(t *testing.T) {
	for _, bad := range []string{
		`{"Version":2,"Generation":"outside","Next":{"HasSidecar":true}}`,
		`{"Version":1,"Generation":"../../outside","Next":{"HasSidecar":true}}`,
		`{"Version":1,"Generation":"out.new.recovery-fake","Next":{"HasSidecar":false}}`,
		`{"Version":1,"Unknown":true}`,
		`{} {}`,
		strings.Repeat("x", 4097),
	} {
		directory := t.TempDir()
		out := publicationFixture(t, filepath.Join(directory, "out"), []types.Value{types.NewWaif(9, 3)})
		before, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(out+checkpointJournalSuffix, []byte(bad), 0600); err != nil {
			t.Fatal(err)
		}
		if err := RecoverCheckpoint(out); err == nil {
			t.Fatal("invalid journal accepted")
		}
		after, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before, after) {
			t.Fatal("invalid journal changed output")
		}
	}
}

// A real test subprocess exits at journal/database/sidecar publication without
// unwinding defers. This verifies process interruption, not simulated power loss.
func TestCheckpointInterruptedPublisherHelper(t *testing.T) {
	if os.Getenv("BARN_CHECKPOINT_INTERRUPTION_HELPER") != "1" {
		return
	}
	out := os.Getenv("BARN_CHECKPOINT_OUTPUT")
	staged := os.Getenv("BARN_CHECKPOINT_STAGED")
	stop := os.Getenv("BARN_CHECKPOINT_STOP")
	fs := checkpointIO{rename: func(from, to string) error {
		if err := renameCheckpointFile(from, to); err != nil {
			return err
		}
		target := out + checkpointJournalSuffix
		if stop == "database" {
			target = out
		}
		if stop == "sidecar" {
			target = out + waifIdentitySidecarSuffix
		}
		if to == target {
			os.Exit(77)
		}
		return nil
	}, syncDirectory: syncParentDirectory}
	var err error
	if os.Getenv("BARN_CHECKPOINT_RECOVER") == "1" {
		err = recoverCheckpointPair(out, fs)
	} else {
		err = publishCheckpointPair(out, staged, fs)
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Fatal("interruption point not reached")
}

func TestCheckpointRecoveryAfterProcessInterruption(t *testing.T) {
	for _, previous := range []bool{false, true} {
		for _, stop := range []string{"journal", "database", "sidecar"} {
			name := stop
			if previous {
				name += "_replace"
			} else {
				name += "_first"
			}
			t.Run(name, func(t *testing.T) {
				directory := t.TempDir()
				old := []types.Value{types.NewWaif(9, 3)}
				fresh := []types.Value{types.NewWaif(9, 3), types.NewWaif(9, 3)}
				out := filepath.Join(directory, "out.new")
				if previous {
					publicationFixture(t, filepath.Join(directory, "out"), old)
				}
				staged := publicationFixture(t, filepath.Join(directory, "next"), fresh)
				command := exec.Command(os.Args[0], "-test.run=^TestCheckpointInterruptedPublisherHelper$")
				command.Env = append(os.Environ(), "BARN_CHECKPOINT_INTERRUPTION_HELPER=1", "BARN_CHECKPOINT_OUTPUT="+out, "BARN_CHECKPOINT_STAGED="+staged, "BARN_CHECKPOINT_STOP="+stop)
				output, err := command.CombinedOutput()
				var exitError *exec.ExitError
				if !errors.As(err, &exitError) || exitError.ExitCode() != 77 {
					t.Fatalf("child interruption: %v %s", err, output)
				}
				want := fresh
				if previous && stop != "sidecar" {
					want = old
				}
				assertCheckpointIdentities(t, out, want)
				// Recovery is idempotent once the journal is cleared.
				if err := RecoverCheckpoint(out); err != nil {
					t.Fatal(err)
				}
				assertCheckpointIdentities(t, out, want)
			})
		}
	}
}

func TestCheckpointPublicationOrderPreservesRecoveryBeforeOutput(t *testing.T) {
	directory := t.TempDir()
	out := publicationFixture(t, filepath.Join(directory, "out"), []types.Value{types.NewWaif(9, 3)})
	staged := publicationFixture(t, filepath.Join(directory, "next"), []types.Value{types.NewWaif(9, 3)})
	var events []string
	fs := checkpointIO{rename: func(from, to string) error {
		events = append(events, "rename:"+filepath.Base(to))
		if to == out {
			journal, generation, err := readCheckpointJournal(out)
			if err != nil {
				t.Fatal(err)
			}
			if journal.Previous == nil {
				t.Fatal("previous generation missing")
			}
			if err := checkCheckpointGeneration(filepath.Join(generation, "previous"), *journal.Previous); err != nil {
				t.Fatal(err)
			}
			if err := checkCheckpointGeneration(filepath.Join(generation, "next"), journal.Next); err != nil {
				t.Fatal(err)
			}
		}
		return renameCheckpointFile(from, to)
	}, syncDirectory: func(path string) error {
		events = append(events, "sync:"+filepath.Base(filepath.Dir(path)))
		return syncParentDirectory(path)
	}}
	if err := publishCheckpointPair(out, staged, fs); err != nil {
		t.Fatal(err)
	}
	journalIndex, databaseIndex := -1, -1
	for i, event := range events {
		if event == "rename:"+filepath.Base(out+checkpointJournalSuffix) {
			journalIndex = i
		}
		if event == "rename:"+filepath.Base(out) {
			databaseIndex = i
		}
	}
	if journalIndex < 2 || databaseIndex != journalIndex+2 || !strings.HasPrefix(events[journalIndex+1], "sync:") {
		t.Fatalf("publication order: %v", events)
	}
}

func TestCheckpointConcurrentLoadAndPublishKeepsOneGeneration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out")
	values := []types.Value{types.NewWaif(9, 3), types.NewWaif(9, 3), types.NewWaif(9, 3)}
	publicationFixture(t, path, values[:1])
	var workers sync.WaitGroup
	errorsFound := make(chan error, 40)
	for _, value := range values {
		workers.Add(1)
		go func(value types.Value) {
			defer workers.Done()
			objectStore := dbstore.NewStore()
			objectStore.SetPendingFinalizations([]types.Value{value})
			for range 8 {
				if err := WriteCheckpoint(path, objectStore, nil, nil, nil); err != nil {
					errorsFound <- err
					return
				}
			}
		}(value)
	}
	workers.Add(1)
	go func() {
		defer workers.Done()
		for range 16 {
			loaded, err := LoadDatabase(path + ".new")
			if err != nil {
				errorsFound <- err
				return
			}
			if len(loaded.PendingFinalizations) != 1 {
				errorsFound <- errors.New("mixed checkpoint count")
				return
			}
			identity := loaded.PendingFinalizations[0].WaifIdentity()
			found := false
			for _, value := range values {
				if identity == value.WaifIdentity() {
					found = true
				}
			}
			if !found {
				errorsFound <- errors.New("checkpoint identity changed")
				return
			}
		}
	}()
	workers.Wait()
	close(errorsFound)
	for err := range errorsFound {
		t.Error(err)
	}
}

func TestCheckpointPublicationLongPathAndPanicNamespace(t *testing.T) {
	directory := filepath.Join(t.TempDir(), strings.Repeat("nested-", 18), strings.Repeat("nested-", 18))
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "database")
	old := []types.Value{types.NewWaif(9, 3)}
	fresh := []types.Value{types.NewWaif(9, 3), types.NewWaif(9, 3)}
	publicationFixture(t, path, old)
	objectStore := dbstore.NewStore()
	objectStore.SetPendingFinalizations(fresh)
	if err := WritePanicCheckpoint(path, objectStore, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	assertCheckpointIdentities(t, path+".new", old)
	assertCheckpointIdentities(t, path+".new.PANIC", fresh)
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("input path modified: %v", err)
	}
}

func TestCheckpointRecoveryMissingGenerationIsAnError(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out.new")
	journal := checkpointJournal{Version: 1, Generation: "out.new.recovery-missing", Next: checkpointDigest{HasSidecar: true}}
	encoded, err := json.Marshal(journal)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out+checkpointJournalSuffix, encoded, 0600); err != nil {
		t.Fatal(err)
	}
	if err := RecoverCheckpoint(out); err == nil {
		t.Fatal("missing generation treated as absent journal")
	}
}

func TestCheckpointJournalRenameErrorAfterEffectRetainsRecovery(t *testing.T) {
	for _, previous := range []bool{false, true} {
		t.Run(strconv.FormatBool(previous), func(t *testing.T) {
			directory := t.TempDir()
			old := []types.Value{types.NewWaif(9, 3)}
			fresh := []types.Value{types.NewWaif(9, 3), types.NewWaif(9, 3)}
			out := filepath.Join(directory, "out.new")
			if previous {
				publicationFixture(t, filepath.Join(directory, "out"), old)
			}
			staged := publicationFixture(t, filepath.Join(directory, "next"), fresh)
			fs := checkpointIO{rename: func(from, to string) error {
				if err := renameCheckpointFile(from, to); err != nil {
					return err
				}
				if to == out+checkpointJournalSuffix {
					return os.ErrPermission
				}
				return nil
			}, syncDirectory: syncParentDirectory}
			if err := publishCheckpointPair(out, staged, fs); !errors.Is(err, os.ErrPermission) {
				t.Fatal(err)
			}
			journal, generation, err := readCheckpointJournal(out)
			if err != nil {
				t.Fatal(err)
			}
			if err := checkCheckpointGeneration(filepath.Join(generation, "next"), journal.Next); err != nil {
				t.Fatal(err)
			}
			want := fresh
			if previous {
				want = old
			}
			assertCheckpointIdentities(t, out, want)
		})
	}
}

func TestCheckpointRecoveryCanResumeInterruptedRollback(t *testing.T) {
	for _, stop := range []string{"database", "sidecar"} {
		t.Run(stop, func(t *testing.T) {
			directory := t.TempDir()
			old := []types.Value{types.NewWaif(9, 3)}
			fresh := []types.Value{types.NewWaif(9, 3), types.NewWaif(9, 3)}
			out := publicationFixture(t, filepath.Join(directory, "out"), old)
			staged := publicationFixture(t, filepath.Join(directory, "next"), fresh)
			fs := checkpointIO{rename: renameCheckpointFile, syncDirectory: syncParentDirectory}
			if _, _, err := prepareCheckpointJournal(out, staged, fs); err != nil {
				t.Fatal(err)
			}
			if err := renameCheckpointFile(staged, out); err != nil {
				t.Fatal(err)
			}
			command := exec.Command(os.Args[0], "-test.run=^TestCheckpointInterruptedPublisherHelper$")
			command.Env = append(os.Environ(), "BARN_CHECKPOINT_INTERRUPTION_HELPER=1", "BARN_CHECKPOINT_RECOVER=1", "BARN_CHECKPOINT_OUTPUT="+out, "BARN_CHECKPOINT_STOP="+stop)
			output, err := command.CombinedOutput()
			var exitError *exec.ExitError
			if !errors.As(err, &exitError) || exitError.ExitCode() != 77 {
				t.Fatalf("child recovery interruption: %v %s", err, output)
			}
			assertCheckpointIdentities(t, out, old)
		})
	}
}

func TestCheckpointWriterRecoversBeforeReusingLinkedStagingNames(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "out")
	out := path + ".new"
	staged := publicationFixture(t, filepath.Join(directory, "next"), []types.Value{types.NewWaif(9, 3)})
	if err := renameCheckpointFile(staged, path+".tmp"); err != nil {
		t.Fatal(err)
	}
	if err := renameCheckpointFile(staged+waifIdentitySidecarSuffix, path+".tmp"+waifIdentitySidecarSuffix); err != nil {
		t.Fatal(err)
	}
	fs := checkpointIO{rename: renameCheckpointFile, syncDirectory: syncParentDirectory}
	if _, _, err := prepareCheckpointJournal(out, path+".tmp", fs); err != nil {
		t.Fatal(err)
	}
	fresh := []types.Value{types.NewWaif(9, 3), types.NewWaif(9, 3)}
	objectStore := dbstore.NewStore()
	objectStore.SetPendingFinalizations(fresh)
	if err := WriteCheckpoint(path, objectStore, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	assertCheckpointIdentities(t, out, fresh)
}

func TestCheckpointRecoveryRestoresLegacyGenerationWithoutSidecar(t *testing.T) {
	directory := t.TempDir()
	out := publicationFixture(t, filepath.Join(directory, "out"), nil)
	if err := os.Remove(out + waifIdentitySidecarSuffix); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	staged := publicationFixture(t, filepath.Join(directory, "next"), []types.Value{types.NewWaif(9, 3)})
	fs := checkpointIO{rename: renameCheckpointFile, syncDirectory: syncParentDirectory}
	if _, _, err := prepareCheckpointJournal(out, staged, fs); err != nil {
		t.Fatal(err)
	}
	if err := renameCheckpointFile(staged, out); err != nil {
		t.Fatal(err)
	}
	if err := RecoverCheckpoint(out); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("legacy database not restored")
	}
	assertCheckpointIdentities(t, out, nil)
	if _, err := os.Stat(out + waifIdentitySidecarSuffix); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy sidecar recreated: %v", err)
	}
}
