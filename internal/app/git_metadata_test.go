package app

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

const testGitHead = "0123456789abcdef0123456789abcdef01234567"

// The copied test executable supplies deterministic Git subprocess behavior.
// No shell, repository, server or external Git installation is involved.
func TestMain(m *testing.M) {
	if os.Getenv("BARN_TEST_GIT_METADATA") == "1" {
		if len(os.Args) > 1 && os.Args[1] == "metadata-pipe-child" {
			time.Sleep(2500 * time.Millisecond)
			if err := os.WriteFile(os.Getenv("BARN_TEST_GIT_PIPE_DONE"), nil, 0600); err != nil {
				os.Exit(90)
			}
			os.Exit(0)
		}
		if len(os.Args) > 1 && (os.Args[1] == "rev-parse" || os.Args[1] == "diff") {
			prefix := "BARN_TEST_GIT_DIFF"
			if os.Args[1] == "rev-parse" {
				prefix = "BARN_TEST_GIT_HEAD"
			}
			delay, _ := strconv.Atoi(os.Getenv(prefix + "_DELAY_MS"))
			time.Sleep(time.Duration(delay) * time.Millisecond)
			if os.Args[1] == "rev-parse" {
				fmt.Println(os.Getenv("BARN_TEST_GIT_HEAD_VALUE"))
				if os.Getenv("BARN_TEST_GIT_PIPE_DONE") != "" {
					child := exec.Command(os.Args[0], "metadata-pipe-child")
					child.Stdout, child.Stderr = os.Stdout, os.Stderr
					if err := child.Start(); err != nil {
						os.Exit(91)
					}
				}
			}
			code, _ := strconv.Atoi(os.Getenv(prefix + "_EXIT"))
			os.Exit(code)
		}
	}
	os.Exit(m.Run())
}

func installMetadataGit(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	name := "git"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.Open(executable)
	if err != nil {
		t.Fatal(err)
	}
	target, err := os.OpenFile(filepath.Join(dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0700)
	if err != nil {
		_ = source.Close()
		t.Fatal(err)
	}
	_, copyErr := io.Copy(target, source)
	sourceErr, targetErr := source.Close(), target.Close()
	if copyErr != nil || sourceErr != nil || targetErr != nil {
		t.Fatalf("copy helper: %v; close source: %v; close target: %v", copyErr, sourceErr, targetErr)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	// This affects subsequently exec'd helper runtimes only: the current test
	// process has already initialized its race options. Keep all caller options
	// while removing the race runtime's default one-second helper exit delay.
	t.Setenv("GORACE", strings.TrimSpace(os.Getenv("GORACE")+" atexit_sleep_ms=0"))
	t.Setenv("BARN_TEST_GIT_METADATA", "1")
	t.Setenv("BARN_TEST_GIT_HEAD_VALUE", testGitHead)
	for _, name := range []string{"BARN_TEST_GIT_HEAD_EXIT", "BARN_TEST_GIT_DIFF_EXIT", "BARN_TEST_GIT_HEAD_DELAY_MS", "BARN_TEST_GIT_DIFF_DELAY_MS", "BARN_TEST_GIT_PIPE_DONE"} {
		t.Setenv(name, "")
	}
}

func TestGitImplementationRefClassifiesResults(t *testing.T) {
	for _, tc := range []struct {
		name, head, headExit, diffExit, want string
	}{
		{"clean", testGitHead, "0", "0", testGitHead + " tracked_dirty=false"},
		{"dirty", testGitHead, "0", "1", testGitHead + " tracked_dirty=true"},
		{"operational-error", testGitHead, "0", "128", testGitHead + " tracked_dirty=unknown"},
		{"head-failure", testGitHead, "128", "0", "unknown tracked_dirty=unknown"},
		{"invalid-head", "not-a-commit", "0", "0", "unknown tracked_dirty=unknown"},
		{"extra-head-line", testGitHead + "\nother", "0", "0", "unknown tracked_dirty=unknown"},
		{"sha256-head", strings.Repeat("a", 64), "0", "0", strings.Repeat("a", 64) + " tracked_dirty=false"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			installMetadataGit(t)
			t.Setenv("BARN_TEST_GIT_HEAD_VALUE", tc.head)
			t.Setenv("BARN_TEST_GIT_HEAD_EXIT", tc.headExit)
			t.Setenv("BARN_TEST_GIT_DIFF_EXIT", tc.diffExit)
			if got := gitImplementationRef(); got != tc.want {
				t.Fatalf("implementation ref = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGitImplementationRefSharesDeadline(t *testing.T) {
	installMetadataGit(t)
	t.Setenv("BARN_TEST_GIT_HEAD_DELAY_MS", "1300")
	t.Setenv("BARN_TEST_GIT_DIFF_DELAY_MS", "1300")
	start := time.Now()
	got := gitImplementationRef()
	elapsed := time.Since(start)
	if want := testGitHead + " tracked_dirty=unknown"; got != want {
		t.Errorf("implementation ref = %q, want %q", got, want)
	}
	if elapsed > 2400*time.Millisecond {
		t.Errorf("combined metadata calls took %v, want shared 2s budget", elapsed)
	}
	t.Logf("combined calls: %v; result %s", elapsed, got)
}

func TestGitImplementationRefBoundsInheritedPipe(t *testing.T) {
	installMetadataGit(t)
	done := filepath.Join(t.TempDir(), "descendant-done")
	t.Setenv("BARN_TEST_GIT_PIPE_DONE", done)
	// The descendant always exits after 2.5s. Wait for its completion marker so
	// this fixture leaves no deliberately running helper after the test ends.
	t.Cleanup(func() {
		deadline := time.Now().Add(4 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(done); err == nil {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Error("bounded pipe helper did not finish")
	})
	start := time.Now()
	got := gitImplementationRef()
	elapsed := time.Since(start)
	if got != "unknown tracked_dirty=unknown" {
		t.Errorf("implementation ref = %q, want unavailable head after pipe timeout", got)
	}
	if elapsed > time.Second {
		t.Errorf("inherited output pipe took %v, want bounded 100ms cleanup", elapsed)
	}
	t.Logf("inherited pipe: %v; result %s", elapsed, got)
}
