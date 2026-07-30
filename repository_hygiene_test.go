package barn

import (
	"os/exec"
	"strings"
	"testing"
)

func TestRootRepositoryHygiene(t *testing.T) {
	obsolete := []string{
		"barn_look_me.txt",
		"barn_look_output.txt",
		"check_programmers.txt",
		"create_test_verb.txt",
		"sample_trace.txt",
		"toast_look_me.txt",
		"toast_look_output.txt",
		"toast_test.txt",
		"test_caller_perms.txt",
		"test_catch.txt",
		"test_catch2.txt",
		"test_catch_expr.txt",
		"test_connect.txt",
		"test_debug_limit.txt",
		"test_exact_mimic.txt",
		"test_exact_mimic2.txt",
		"test_exact_mimic3.txt",
		"test_hook_error.txt",
		"test_is_clear.txt",
		"test_limit_boundary.txt",
		"test_limit_debug.txt",
		"test_news.txt",
		"test_object_bytes.txt",
		"test_object_bytes_full.txt",
		"test_pass_simple.txt",
		"test_primit2.txt",
		"test_primitives.txt",
		"test_prop_check.txt",
		"test_queued_format.txt",
		"test_range.txt",
		"test_simple.txt",
		"test_simple_error.txt",
		"test_simple_limit.txt",
		"test_toast_range.txt",
		"test_toast_simple.txt",
		"test_toast_wildcard.txt",
		"test_traceback.txt",
		"test_value_bytes_check.txt",
		"test_vb.txt",
		"test_vb2.txt",
		"test_verb_multiline.txt",
		"test_waif_nested.txt",
		"test_wildcard.txt",
	}
	args := append([]string{"ls-files", "--"}, obsolete...)
	output, err := exec.Command("git", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("list obsolete tracked files: %v\n%s", err, output)
	}
	if tracked := strings.TrimSpace(string(output)); tracked != "" {
		t.Errorf("obsolete root debug artifacts are still tracked:\n%s", tracked)
	}

	rootScratch := []string{
		"test_probe.txt",
		"toast_probe.txt",
		"barn_look_probe.txt",
		"tmp_probe.txt",
		"probe.db",
		"test_probe.db",
		"probe.exe",
		"probe.log",
	}
	for _, path := range rootScratch {
		if output, err := exec.Command("git", "check-ignore", "--no-index", "-q", "--", path).CombinedOutput(); err != nil {
			t.Errorf("root scratch path %q is not ignored: %v\n%s", path, err, output)
		}
	}

	nestedScratch := []string{
		"nested/test_probe.txt",
		"nested/toast_probe.txt",
		"nested/barn_look_probe.txt",
		"nested/tmp_probe.txt",
		"nested/probe.db",
	}
	for _, path := range nestedScratch {
		err := exec.Command("git", "check-ignore", "--no-index", "-q", "--", path).Run()
		if err == nil {
			t.Errorf("root scratch ignore pattern unexpectedly matches %q", path)
			continue
		}
		exitErr, ok := err.(*exec.ExitError)
		if !ok || exitErr.ExitCode() != 1 {
			t.Fatalf("check nested scratch path %q: %v", path, err)
		}
	}

	protected := []string{
		"Test.db",
		"Test_conf.db",
		"Test_fresh.db",
		"Test_fresh2.db",
		"Test_full.db",
		"Test_waif.db",
		"toastcore.db",
		"argon2.dll",
		"sqlite3.dll",
		"pcre.dll",
		"nettle-8.dll",
		"libgcc_s_seh-1.dll",
		"libstdc++-6.dll",
		"libwinpthread-1.dll",
	}
	args = append([]string{"ls-files", "--error-unmatch", "--"}, protected...)
	if output, err := exec.Command("git", args...).CombinedOutput(); err != nil {
		t.Fatalf("protected fixtures and runtime DLLs must remain tracked: %v\n%s", err, output)
	}
	for _, path := range protected {
		err := exec.Command("git", "check-ignore", "--no-index", "-q", "--", path).Run()
		if err == nil {
			t.Errorf("protected path %q is ignored", path)
			continue
		}
		exitErr, ok := err.(*exec.ExitError)
		if !ok || exitErr.ExitCode() != 1 {
			t.Fatalf("check protected path %q: %v", path, err)
		}
	}
}
