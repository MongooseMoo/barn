package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/MongooseMoo/barn/internal/buildinfo"
)

func execute(root *cobra.Command, args ...string) (int, string, string) {
	var out, errOut bytes.Buffer
	code := Execute(root, args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestVersionFlagsPrintTheBuildLine(t *testing.T) {
	want := buildinfo.Current().Line("tool") + "\n"
	for _, args := range [][]string{{"--version"}, {"-V"}} {
		root := NewRoot("tool", "test tool")
		root.RunE = RunE(func(*cobra.Command, []string) error { return errors.New("body ran") })
		code, out, errOut := execute(root, args...)
		if code != ExitOK || out != want || errOut != "" {
			t.Fatalf("%v: code %d out %q err %q, want %q", args, code, out, errOut, want)
		}
	}
}

func TestVersionSubcommand(t *testing.T) {
	root := NewRoot("tool", "test tool")
	AddVersionCommand(root)
	code, out, _ := execute(root, "version")
	if code != ExitOK || out != buildinfo.Current().Line("tool")+"\n" {
		t.Fatalf("code %d out %q", code, out)
	}
}

func TestExitCodes(t *testing.T) {
	cases := []struct {
		name    string
		body    error
		args    []string
		code    int
		stderr  string
		hasHelp bool
	}{
		{"success", nil, nil, ExitOK, "", false},
		{"failure", errors.New("boom"), nil, ExitFailed, "tool: boom\n", false},
		{"chosen", Exit(7), nil, 7, "", false},
		{"usage from body", UsageErrorf("bad combo"), nil, ExitUsage, "tool: bad combo\n", true},
		{"unknown flag", nil, []string{"--nope"}, ExitUsage, "tool: unknown flag: --nope\n", true},
		{"single-dash long flag", nil, []string{"-db"}, ExitUsage, "tool: unknown shorthand flag: 'd' in -db\n", true},
	}
	for _, tc := range cases {
		root := NewRoot("tool", "test tool")
		body := tc.body
		root.RunE = RunE(func(*cobra.Command, []string) error { return body })
		code, _, errOut := execute(root, tc.args...)
		if code != tc.code || !strings.HasPrefix(errOut, tc.stderr) {
			t.Errorf("%s: code %d stderr %q, want %d %q", tc.name, code, errOut, tc.code, tc.stderr)
		}
		if strings.Contains(errOut, "Run 'tool --help' for usage.") != tc.hasHelp {
			t.Errorf("%s: help pointer in %q, want %v", tc.name, errOut, tc.hasHelp)
		}
	}
}

func TestLogFlagsSelectLevel(t *testing.T) {
	cases := []struct {
		args  []string
		level string
		usage bool
	}{
		{nil, "info", false},
		{[]string{"-v"}, "debug", false},
		{[]string{"-vv"}, "debug", false},
		{[]string{"--verbose"}, "debug", false},
		{[]string{"-q"}, "warn", false},
		{[]string{"-qq"}, "error", false},
		{[]string{"--quiet", "--quiet"}, "error", false},
		{[]string{"--log-level", "error"}, "error", false},
		{[]string{"--log-level=debug"}, "debug", false},
		{[]string{"-v", "-q"}, "", true},
		{[]string{"-v", "--log-level", "warn"}, "", true},
	}
	for _, tc := range cases {
		root := NewRoot("tool", "test tool")
		logs := AddLogFlags(root.PersistentFlags(), "info")
		var got string
		root.RunE = RunE(func(*cobra.Command, []string) error {
			var err error
			got, err = logs.Level()
			return err
		})
		code, _, errOut := execute(root, tc.args...)
		if tc.usage {
			if code != ExitUsage {
				t.Errorf("%v: code %d stderr %q, want usage error", tc.args, code, errOut)
			}
			continue
		}
		if code != ExitOK || got != tc.level {
			t.Errorf("%v: code %d level %q stderr %q, want %q", tc.args, code, got, errOut, tc.level)
		}
	}
}

func TestLogFlagsReachSubcommands(t *testing.T) {
	root := NewRoot("tool", "test tool")
	logs := AddLogFlags(root.PersistentFlags(), "info")
	var got string
	root.AddCommand(&cobra.Command{Use: "sub", RunE: RunE(func(*cobra.Command, []string) error {
		var err error
		got, err = logs.Level()
		return err
	})})
	if code, _, errOut := execute(root, "sub", "-q"); code != ExitOK || got != "warn" {
		t.Fatalf("code %d level %q stderr %q", code, got, errOut)
	}
}
