package builtins

// TestReview_IO_* — analyst red tests for the I/O & system builtin review.
// Each test is expected to FAIL against current code, demonstrating a confirmed bug.
// Do NOT change these tests to make them pass by weakening the assertion;
// fix the production code instead.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// ---------------------------------------------------------------------------
// C3: SQLite sandbox escape
// builtinSqliteOpen calls sanitizeFilePath() but omits resolveFilePath(), so
// named SQLite databases are created in CWD rather than the files/ root that
// fileio builtins enforce.
// ---------------------------------------------------------------------------

func TestReview_IO_SqliteSandboxEscape(t *testing.T) {
	resetSQLiteTestState(t)
	t.Cleanup(func() { resetSQLiteTestState(t) })

	const dbName = "review_sandbox_escape_test.db"
	// Remove from both possible locations regardless of outcome.
	defer os.Remove(dbName)
	defer os.Remove(filepath.Join("files", dbName))

	// Ensure files/ exists so the fixed code can write there.
	_ = os.MkdirAll("files", 0o755)

	ctx := sqliteWizardCtx()
	handleVal := sqliteMustResult(t, builtinSqliteOpen(ctx, []types.Value{types.NewStr(dbName)}))
	handleID := sqliteMustInt(t, handleVal)
	builtinSqliteClose(ctx, []types.Value{types.NewInt(handleID)})

	// File MUST be inside the files/ sandbox, not in CWD.
	if _, err := os.Stat(filepath.Join("files", dbName)); os.IsNotExist(err) {
		t.Fatalf(
			"CONFIRMED BUG (C3): sqlite_open(%q) created the database in CWD (sandbox escape).\n"+
				"Expected path: files/%s\n"+
				"Cause: builtinSqliteOpen uses sanitizeFilePath() but never calls resolveFilePath().\n"+
				"All fileio builtins call resolveFilePath(); sqlite bypasses it.",
			dbName, dbName,
		)
	}
}

// ---------------------------------------------------------------------------
// M1: file_readlines() ignores the binary flag
// builtinFileReadlines uses bufio.Scanner unconditionally and returns
// scanner.Text() without calling filterTextMode (text mode) or
// encodeBinaryBytes (binary mode).  Binary-mode readlines must ~XX-encode
// non-printable bytes; it currently does not.
// ---------------------------------------------------------------------------

func TestReview_IO_FileReadlinesBinaryMode(t *testing.T) {
	// Ensure files/ directory exists (fileio sandbox root).
	if err := os.MkdirAll("files", 0o755); err != nil {
		t.Fatalf("setup MkdirAll: %v", err)
	}

	const fname = "review_readlines_binary_test.txt"
	fpath := filepath.Join("files", fname)
	defer os.Remove(fpath)

	// Line 1 contains a non-printable byte (0x01) between printable chars and
	// ends with a newline.  Toast applies the handle's file_type in_filter to
	// the raw getline() output (newline included):
	//   - binary mode (raw_bytes_to_binary): non-printable bytes -> "~XX", so
	//     "ab\x01c\n" -> "ab~01c~0A".  (Cf. ToastStunt test_fileio.rb:171
	//     test_that_reading_text_in_binary_mode_is_ok: "...five\n" -> "...five~0A".)
	//   - text mode (raw_bytes_to_clean): non-printable bytes are dropped, so
	//     "ab\x01c\n" -> "abc".
	// The old scanner.Text() code returned the raw "ab\x01c" in both modes,
	// which this test rejects.
	if err := os.WriteFile(fpath, []byte("ab\x01c\nline2\n"), 0o644); err != nil {
		t.Fatalf("setup WriteFile: %v", err)
	}

	ctx := newTestExecution()
	ctx.IsWizard = true

	readLine1 := func(mode string) string {
		t.Helper()
		openRes := builtinFileOpen(ctx, []types.Value{
			types.NewStr(fname),
			types.NewStr(mode),
		})
		if openRes.IsError() {
			t.Fatalf("file_open(%q) error: %v", mode, openRes.Error)
		}
		handleID := openRes.Val.Int()
		defer builtinFileClose(ctx, []types.Value{types.NewInt(handleID)})

		res := builtinFileReadlines(ctx, []types.Value{
			types.NewInt(handleID),
			types.NewInt(1),
			types.NewInt(1),
		})
		if res.IsError() {
			t.Fatalf("file_readlines(%q) error: %v", mode, res.Error)
		}
		list := res.Val
		if list.Type() != types.TYPE_LIST || list.Len() == 0 {
			t.Fatalf("expected a 1-element list, got %v", res.Val)
		}
		return list.Get(1).Str()
	}

	// Binary mode ("r-bf"): non-printable bytes (incl. the trailing newline)
	// must be ~XX-encoded.
	if got, want := readLine1("r-bf"), "ab~01c~0A"; got != want {
		t.Fatalf(
			"CONFIRMED BUG (M1): file_readlines in binary mode returned %q, want %q.\n"+
				"Cause: builtinFileReadlines ignores h.binary; it called scanner.Text() directly\n"+
				"without encodeBinaryBytes (binary path).",
			got, want,
		)
	}

	// Text mode ("r-tf"): non-printable bytes must be filtered out.
	if got, want := readLine1("r-tf"), "abc"; got != want {
		t.Fatalf(
			"CONFIRMED BUG (M1): file_readlines in text mode returned %q, want %q.\n"+
				"Cause: builtinFileReadlines ignores h.binary; it called scanner.Text() directly\n"+
				"without filterTextMode (text path).",
			got, want,
		)
	}
}

// ---------------------------------------------------------------------------
// H4: queued_tasks() sort order is inverted
// builtinQueuedTasks sorted with StartTime.After() which produces descending
// order (newest first).
//
// Toast authority (WSL oracle down — source is authoritative):
//   - bf_queued_tasks (tasks.cc:2496) builds the result by iterating the task
//     queues; it applies NO sort of its own.
//   - The forked/suspended tasks come from waiting_tasks (tasks.cc:2571-2581),
//     which enqueue_waiting (tasks.cc:1182-1205) keeps sorted ASCENDING by
//     start_tv — each task is inserted before the first existing task with a
//     strictly greater start time (timercmp(..., <), tasks.cc:1193,1200).
//   So queued_tasks() returns tasks earliest-start-time first (ascending).
//   The review's "ascending (oldest first)" expectation is CORRECT.
// ---------------------------------------------------------------------------

func TestReview_IO_QueuedTasksSortOrder(t *testing.T) {
	ctx := newTestExecution()
	ctx.IsWizard = true
	mgr := wireTestTaskManager(ctx)

	// Use large IDs unlikely to collide with other test tasks.
	const earlierID = int64(88801)
	const laterID = int64(88802)

	earlier := task.NewTask(earlierID, 1, 10000, 30)
	later := task.NewTask(laterID, 1, 10000, 30)

	// Set a non-empty verb name so GetQueuedTasks does not filter them as
	// eval scaffolding (which filters IsForked && VerbName == "").
	earlier.VerbName = "review_earlier_verb"
	later.VerbName = "review_later_verb"

	mgr.RegisterTask(earlier)
	mgr.RegisterTask(later)
	mgr.SuspendTask(earlier, -1)
	mgr.SuspendTask(later, -1)
	// Give them clearly distinct start times. SuspendTask(-1) stamps the
	// indefinite-suspend sentinel onto StartTime, so override it AFTERWARDS to
	// exercise the ascending comparator with two distinct, finite start times.
	earlier.StartTime = time.Now().Add(-10 * time.Second)
	later.StartTime = time.Now()
	defer mgr.RemoveTask(earlierID)
	defer mgr.RemoveTask(laterID)

	res := builtinQueuedTasks(ctx, []types.Value{})
	if res.IsError() {
		t.Fatalf("queued_tasks() error: %v", res.Error)
	}

	list := res.Val
	if list.Type() != types.TYPE_LIST {
		t.Fatalf("expected list result, got %v", res.Val)
	}

	// Locate our two tasks in the result.
	earlierIdx, laterIdx := 0, 0
	for i := 1; i <= list.Len(); i++ {
		entry := list.Get(i)
		if entry.Type() != types.TYPE_LIST {
			continue
		}
		idVal := entry.Get(1)
		if idVal.Type() != types.TYPE_INT {
			continue
		}
		switch idVal.Int() {
		case earlierID:
			earlierIdx = i
		case laterID:
			laterIdx = i
		}
	}

	if earlierIdx == 0 || laterIdx == 0 {
		t.Fatalf("could not find both test tasks in queued_tasks() output (earlierIdx=%d laterIdx=%d)",
			earlierIdx, laterIdx)
	}

	// Ascending order: the older task must appear before the newer one.
	if earlierIdx > laterIdx {
		t.Fatalf(
			"CONFIRMED BUG (H4): queued_tasks() returned newer task at index %d before older task at index %d.\n"+
				"Cause: builtinQueuedTasks sorts with StartTime.After() (descending);\n"+
				"Toast returns tasks in ascending StartTime order (oldest first).\n"+
				"Fix: change After to Before in the sort.SliceStable call.",
			laterIdx, earlierIdx,
		)
	}
}

// ---------------------------------------------------------------------------
// F28v2: an indefinitely-suspended task must sort LAST in queued_tasks().
//
// ToastStunt's enqueue_suspended_task sets start_tv.tv_sec = INTNUM_MAX for an
// indefinite suspend (no/negative seconds; tasks.cc:1306-1307), so it sorts
// after every timed task. Under Barn's ascending comparator (F28), an
// indefinitely-suspended task must therefore appear AFTER a later-scheduled
// timed (e.g. fork) task. Regression guard for verify-F28.md's
// waif::queued_tasks_returns_valid_waifs_for_programmers failure: a fork(100)
// task (:c) and an indefinite suspend() task (:a) must come out :c then :a.
// ---------------------------------------------------------------------------

func TestReview_IO_QueuedTasksIndefiniteSuspendSortsLast(t *testing.T) {
	ctx := newTestExecution()
	ctx.IsWizard = true
	mgr := wireTestTaskManager(ctx)

	const timedID = int64(88811)
	const indefID = int64(88812)

	timed := task.NewTask(timedID, 1, 10000, 30)
	indef := task.NewTask(indefID, 1, 10000, 30)

	timed.VerbName = "review_timed_verb"
	indef.VerbName = "review_indef_verb"

	mgr.RegisterTask(timed)
	mgr.RegisterTask(indef)

	// timed: a future-scheduled task (mirrors fork(100)). Finite start time.
	timed.StartTime = time.Now().Add(100 * time.Second)
	// indef: indefinite suspend() — SuspendTask(-1) stamps the far-future
	// IndefiniteSuspendStartTime sentinel onto StartTime.
	mgr.SuspendTask(indef, -1)
	timed.SetState(task.TaskSuspended)

	defer mgr.RemoveTask(timedID)
	defer mgr.RemoveTask(indefID)

	// Sanity: the indefinite suspend got the far-future sentinel and no wake
	// deadline (so the scheduler never auto-wakes it).
	if !indef.StartTime.Equal(task.IndefiniteSuspendStartTime) {
		t.Fatalf("indefinite-suspended task StartTime = %v, want sentinel %v",
			indef.StartTime, task.IndefiniteSuspendStartTime)
	}
	if indef.WakeDue(time.Now()) {
		t.Fatalf("indefinite-suspended task reported WakeDue=true; it must never auto-wake")
	}

	res := builtinQueuedTasks(ctx, []types.Value{})
	if res.IsError() {
		t.Fatalf("queued_tasks() error: %v", res.Error)
	}
	list := res.Val
	if list.Type() != types.TYPE_LIST {
		t.Fatalf("expected list result, got %v", res.Val)
	}

	timedIdx, indefIdx := 0, 0
	for i := 1; i <= list.Len(); i++ {
		entry := list.Get(i)
		if entry.Type() != types.TYPE_LIST {
			continue
		}
		idVal := entry.Get(1)
		if idVal.Type() != types.TYPE_INT {
			continue
		}
		switch idVal.Int() {
		case timedID:
			timedIdx = i
		case indefID:
			indefIdx = i
		}
	}

	if timedIdx == 0 || indefIdx == 0 {
		t.Fatalf("could not find both tasks in queued_tasks() (timedIdx=%d indefIdx=%d)", timedIdx, indefIdx)
	}
	if indefIdx < timedIdx {
		t.Fatalf("indefinite-suspended task sorted at %d BEFORE timed task at %d; "+
			"it must sort LAST (mirror Toast INTNUM_MAX start_tv)", indefIdx, timedIdx)
	}
}

// ---------------------------------------------------------------------------
// C1/C2: crypt() SHA256/SHA512 rounds are silently capped at 1000
// cryptSHA256 and cryptSHA512 cap actualRounds at 1000 regardless of the
// requested count, but emit a prefix that claims the original round count.
// Two calls with different rounds (both > 1000) produce identical hash bytes
// while claiming different rounds, proving the cap is applied silently.
// ---------------------------------------------------------------------------

func TestReview_IO_CryptSHA256RoundsSilentlyCapped(t *testing.T) {
	// Both salts request more than 1000 rounds; the implementation caps them.
	// If rounds were actually respected the two salts would produce different
	// hashes.  Because they are both capped to 1000 the hash bytes match
	// even though the embedded round counts differ.
	res5000, err5000 := cryptSHA256("testpassword", "$5$rounds=5000$testsalt$")
	res2000, err2000 := cryptSHA256("testpassword", "$5$rounds=2000$testsalt$")
	if err5000 != nil || err2000 != nil {
		t.Fatalf("cryptSHA256 errors: %v / %v", err5000, err2000)
	}

	// Extract the hash portion (everything after the third '$...salt$' segment).
	hashOf := func(s string) string {
		// Format: $5$rounds=N$salt$HASH
		parts := strings.SplitN(s, "$", 5)
		if len(parts) < 5 {
			return s
		}
		return parts[4]
	}

	hash5000 := hashOf(res5000)
	hash2000 := hashOf(res2000)

	if hash5000 == hash2000 {
		t.Fatalf(
			"CONFIRMED BUG (C1): crypt() SHA256 rounds are silently capped at 1000.\n"+
				"rounds=5000 and rounds=2000 produced identical hash bytes %q\n"+
				"(prefix claims different rounds but computation used the same 1000).\n"+
				"Full results:\n  rounds=5000: %s\n  rounds=2000: %s",
			hash5000, res5000, res2000,
		)
	}
}
