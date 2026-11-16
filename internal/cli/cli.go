// Package cli gives every Barn command the same command-line shape: GNU-style
// flags (--long, -s), --version/-V from the build's git revision, help in one
// style, and, for the commands that log, -v/-q/--log-level.
//
// Exit status is uniform: 0 on success, 2 when the command line itself is
// wrong, 1 when the command fails, or the code a command chooses with Exit.
package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/MongooseMoo/barn/internal/buildinfo"
)

// Exit codes shared by every command.
const (
	ExitOK     = 0
	ExitFailed = 1
	ExitUsage  = 2
)

// NewRoot returns the root command for the binary called name, with
// --version/-V. Cobra prints the version itself when the flag is set; defining
// the flag here keeps -v free for --verbose.
func NewRoot(name, short string) *cobra.Command {
	root := &cobra.Command{
		Use:           name,
		Short:         short,
		Version:       buildinfo.Current().Line(name),
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.SetVersionTemplate("{{.Version}}\n")
	root.Flags().BoolP("version", "V", false, "Print the version and exit")
	return root
}

// AddVersionCommand adds a "version" subcommand to a root that has
// subcommands. Single-command tools leave it out: there "version" may be an
// argument, such as a file name.
func AddVersionCommand(root *cobra.Command) {
	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print the version and exit",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintln(cmd.OutOrStdout(), root.Version)
		},
	})
}

// exitError ends a command with a chosen status and no further message.
type exitError struct{ code int }

func (e exitError) Error() string { return fmt.Sprintf("exit status %d", e.code) }

// Exit returns an error that makes Execute return code without printing
// anything; the command has already said what it needs to. Exit(0) is nil.
func Exit(code int) error {
	if code == ExitOK {
		return nil
	}
	return exitError{code}
}

// runError marks a failure of the command's own work, as opposed to a command
// line cobra could not accept.
type runError struct{ err error }

func (e runError) Error() string { return e.err.Error() }
func (e runError) Unwrap() error { return e.err }

// usageError is a command-line mistake found by the command body, such as two
// flags that cannot be combined.
type usageError struct{ err error }

func (e usageError) Error() string { return e.err.Error() }
func (e usageError) Unwrap() error { return e.err }

// UsageErrorf reports a command-line mistake from a command body; Execute
// treats it like cobra's own parse errors (exit 2, pointer to --help).
func UsageErrorf(format string, args ...any) error {
	return usageError{fmt.Errorf(format, args...)}
}

// RunE adapts a command body: its failures exit 1 (or the code given to Exit),
// while every error cobra raises before the body runs (unknown flag or command,
// wrong number of arguments) and every UsageErrorf exits 2.
func RunE(fn func(cmd *cobra.Command, args []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		err := fn(cmd, args)
		var usage usageError
		if err == nil || errors.As(err, &usage) {
			return err
		}
		return runError{err}
	}
}

// Execute runs root with args (normally os.Args[1:]) and returns the process
// exit status. Errors are printed to stderr prefixed with the command path;
// command-line errors add a pointer to --help.
func Execute(root *cobra.Command, args []string, stdout, stderr io.Writer) int {
	if args == nil {
		args = []string{}
	}
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)
	cmd, err := root.ExecuteC()
	if err == nil {
		return ExitOK
	}
	var run runError
	if errors.As(err, &run) {
		var exit exitError
		if errors.As(run.err, &exit) {
			return exit.code
		}
		fmt.Fprintf(stderr, "%s: %v\n", cmd.CommandPath(), run.err)
		return ExitFailed
	}
	fmt.Fprintf(stderr, "%s: %v\nRun '%s --help' for usage.\n", cmd.CommandPath(), err, cmd.CommandPath())
	return ExitUsage
}

// LogFlags are -v/--verbose, -q/--quiet and --log-level, for commands that log.
type LogFlags struct {
	flags   *pflag.FlagSet
	verbose int
	quiet   int
	level   string
}

// AddLogFlags registers the logging flags on flags (usually a root's
// persistent flags) with defaultLevel as the --log-level default.
func AddLogFlags(flags *pflag.FlagSet, defaultLevel string) *LogFlags {
	l := &LogFlags{flags: flags}
	flags.CountVarP(&l.verbose, "verbose", "v", "Log debug records too (same as --log-level debug)")
	flags.CountVarP(&l.quiet, "quiet", "q", "Log only warnings and errors; -qq logs only errors")
	flags.StringVar(&l.level, "log-level", defaultLevel, "Minimum log `level`: debug, info, warn, or error")
	return l
}

// Level returns the log level the flags select. Giving more than one of
// --log-level, -v and -q is an error, since they would disagree.
func (l *LogFlags) Level() (string, error) {
	given := 0
	for _, changed := range []bool{l.flags.Changed("log-level"), l.verbose > 0, l.quiet > 0} {
		if changed {
			given++
		}
	}
	if given > 1 {
		return "", UsageErrorf("use only one of --log-level, -v/--verbose and -q/--quiet")
	}
	switch {
	case l.verbose > 0:
		return "debug", nil
	case l.quiet == 1:
		return "warn", nil
	case l.quiet > 1:
		return "error", nil
	}
	return l.level, nil
}
