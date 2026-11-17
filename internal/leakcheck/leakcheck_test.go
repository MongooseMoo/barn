package leakcheck

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The intentional permanent leak lives only in a child, so it cannot contaminate
// other tests, a subsequent healthy control, or a caller's baseline profile.
func TestNativeGoroutineControl(t *testing.T) {
	if os.Getenv("BARN_LEAK_CONTROL_CHILD") == "1" {
		ready := make(chan struct{})
		go func() {
			blocked := make(chan struct{})
			close(ready)
			<-blocked
		}()
		<-ready
		s, err := Capture(t.TempDir(), "intentional-permanent-leak", nil)
		if err != nil || s.Leaked == 0 {
			t.Fatalf("native leak control: leaked=%d error=%v", s.Leaked, err)
		}
		t.Logf("native control detected %d permanently blocked goroutines", s.Leaked)
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestNativeGoroutineControl$", "-test.v")
	cmd.Env = append(os.Environ(), "BARN_LEAK_CONTROL_CHILD=1")
	output, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(output), "native control detected") {
		t.Fatalf("native control subprocess: %v\n%s", err, output)
	}
	t.Logf("%s", output)
}

func TestReplayControls(t *testing.T) {
	for _, kind := range []string{"healthy", "heap", "bounded-cache", "descriptor"} {
		t.Run(kind, func(t *testing.T) {
			if kind == "descriptor" && runtime.GOOS != "linux" {
				t.Skip("descriptor oracle uses Linux /proc")
			}
			var retained [][]byte
			var files []*os.File
			defer func() {
				for _, f := range files {
					f.Close()
				}
			}()
			file := filepath.Join(t.TempDir(), "descriptor-control")
			if err := os.WriteFile(file, nil, 0600); err != nil {
				t.Fatal(err)
			}
			result, err := Replay(t.TempDir(), func() {
				switch kind {
				case "heap":
					retained = append(retained, make([]byte, 128*1024))
				case "bounded-cache":
					if len(retained) == 0 {
						retained = append(retained, make([]byte, 2*1024*1024))
					}
				case "healthy", "descriptor":
					f, err := os.Open(file)
					if err != nil {
						t.Fatal(err)
					}
					if kind == "healthy" {
						f.Close()
					} else {
						files = append(files, f)
					}
				}
			}, nil)
			runtime.KeepAlive(retained)
			runtime.KeepAlive(files)
			if err != nil {
				t.Fatal(err)
			}
			want := "healthy"
			if kind == "heap" {
				want = "heap-candidate"
			} else if kind == "descriptor" {
				want = "leak"
			}
			if result.Verdict != want {
				t.Fatalf("%s verdict=%s want=%s reasons=%v samples=%+v", kind, result.Verdict, want, result.Reasons, result.Samples)
			}
			t.Logf("%s: %s %v", kind, result.Verdict, result.Reasons)
		})
	}
}

func TestMainPreservesFailureAndRejectsMissingEvidence(t *testing.T) {
	t.Setenv("BARN_LEAK_OUTPUT", "")
	if code := Main(func() int { return 9 }); code != 9 {
		t.Fatalf("lost test failure: %d", code)
	}
	file := filepath.Join(t.TempDir(), "not-directory")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BARN_LEAK_OUTPUT", file)
	called := false
	if code := Main(func() int { called = true; return 0 }); code != 1 || !called {
		t.Fatalf("missing evidence must fail without suppressing tests: code=%d called=%v", code, called)
	}
}
