// barn is the Barn MOO server and its database tools.
//
//	barn serve --db game.db --listen tcp://:7777
//	barn eval --db game.db '1 + 2'
//	barn verb-code --db game.db '#0:do_login_command'
//	barn --version
//
// Run "barn help" or "barn <command> --help" for every command and flag.
package main

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime/debug"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/MongooseMoo/barn/config"
	"github.com/MongooseMoo/barn/internal/app"
	"github.com/MongooseMoo/barn/internal/cli"
	"github.com/MongooseMoo/barn/internal/isolationprobe"
	"github.com/MongooseMoo/barn/internal/listener"
	"github.com/MongooseMoo/barn/logging"
	"github.com/MongooseMoo/barn/server"
)

func main() {
	os.Exit(cli.Execute(newRootCommand(), os.Args[1:], os.Stdout, os.Stderr))
}

// barnCommand holds what the flags of one invocation set.
type barnCommand struct {
	cfg    app.Config
	logs   *cli.LogFlags
	gomem  int
	gogc   int
	listen []string
}

func newRootCommand() *cobra.Command {
	b := &barnCommand{cfg: app.DefaultConfig()}
	root := cli.NewRoot("barn", "Barn, a ToastStunt-compatible MOO server, and its database tools")
	root.Long = "Barn runs ToastStunt-format MOO databases (serve) and inspects or rewrites them offline (every other command)."
	root.Example = `  barn serve --db game.db --listen tcp://:7777
  barn eval --db game.db 'return #0.name;'
  barn verb-code --db game.db '#0:do_login_command'`

	flags := root.PersistentFlags()
	flags.StringVar(&b.cfg.DatabasePath, "db", b.cfg.DatabasePath, "Database `file`")
	b.logs = cli.AddLogFlags(flags, b.cfg.LogLevel)
	flags.StringVar(&b.cfg.LogDir, "log-dir", b.cfg.LogDir, "JSON log file `dir`ectory (empty disables the file sink)")
	flags.IntVar(&b.gomem, "gomemlimit-mib", 0, "Go soft memory limit in `MiB` (0 leaves it unset)")
	flags.IntVar(&b.gogc, "gogc", -1, "Go GC target `percent`age (-1 keeps $GOGC or Barn's default)")

	root.AddGroup(
		&cobra.Group{ID: "server", Title: "Server:"},
		&cobra.Group{ID: "inspect", Title: "Inspect a database:"},
		&cobra.Group{ID: "files", Title: "Database files:"},
	)
	root.AddCommand(b.serveCommand(), b.evalCommand())
	root.AddCommand(b.inspectCommands()...)
	root.AddCommand(b.toastCheckCommand())
	root.AddCommand(b.dumpCommand(), b.offlineCheckpointCommand(), b.memoryReportCommand(), b.listProfilesCommand())
	cli.AddVersionCommand(root)
	return root
}

// addOptionFlags adds the flags that shape the runtime options a loaded
// database runs with.
func (b *barnCommand) addOptionFlags(flags *pflag.FlagSet) {
	flags.StringVar(&b.cfg.ConfigPath, "config", "", "Server config `file`")
	flags.BoolVar(&b.cfg.PromoteNumbers, "promote-numbers", false, "Enable numeric promotion")
	flags.BoolVarP(&b.cfg.Outbound, "outbound", "o", false, "Enable outbound network connections, overriding --config")
	flags.BoolVarP(&b.cfg.NoOutbound, "no-outbound", "O", false, "Disable outbound network connections, overriding --config")
}

func (b *barnCommand) serveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "serve",
		Short:   "Run the MOO server",
		GroupID: "server",
		Args:    cobra.NoArgs,
		Example: `  barn serve --db game.db --port 7777
  barn serve --db game.db --listen tcp://127.0.0.1:7777 --listen ws://:7778/moo -v`,
	}
	flags := cmd.Flags()
	flags.IntVarP(&b.cfg.Port, "port", "p", b.cfg.Port, "Listen `port` for the default TCP listener (not with --listen)")
	flags.StringArrayVar(&b.listen, "listen", nil, "Listener `URL`, repeatable, e.g. tcp://:7777 or ws://:7778/moo (not with --port)")
	b.addOptionFlags(flags)
	flags.IntVar(&b.cfg.CheckpointInterval, "checkpoint-interval", b.cfg.CheckpointInterval, "Checkpoint interval in `seconds` (0 disables)")
	flags.StringVar(&b.cfg.DebugAddr, "debug-addr", b.cfg.DebugAddr, "Listen `addr`ess for the pprof/expvar endpoint (\"off\" disables)")
	flags.StringVar(&b.cfg.OperatorAddr, "operator-addr", b.cfg.OperatorAddr, "Listen `addr`ess for passive liveness/readiness probes (\"off\" disables)")
	flags.BoolVar(&b.cfg.TraceEnabled, "trace", false, "Trace execution metadata without argument, result, or message payloads")
	flags.StringVar(&b.cfg.TraceFilter, "trace-filter", "", "Comma-separated verb glob `patterns` for metadata tracing")
	flags.Func("start-line", "Pass one `line` to #0:do_start_script before startup", func(value string) error {
		if b.cfg.StartScript == nil {
			b.cfg.StartScriptFirst = false
		}
		b.cfg.StartLine = &value
		return nil
	})
	flags.Func("start-script", "Pass the lines of `file` to #0:do_start_script before startup", func(value string) error {
		if b.cfg.StartLine == nil {
			b.cfg.StartScriptFirst = true
		}
		b.cfg.StartScript = &value
		return nil
	})
	flags.StringVar(&b.cfg.ProfileID, "profile-id", "", "Managed profile `id`entifier")
	flags.StringVar(&b.cfg.ProfileManifest, "profile-manifest", "", "Write managed profile metadata to this `path`")
	cmd.RunE = b.run(nil)
	return cmd
}

func (b *barnCommand) evalCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "eval [code]",
		Short: "Evaluate MOO code as the database's first wizard",
		Long: `Evaluate MOO code (statements, or one expression) as the database's first wizard
and print its result line: "=> VALUE", "Error: CODE" or "Compile error: ...".
With --file, evaluate one input per line of a file instead.`,
		GroupID: "inspect",
		Args:    cobra.MaximumNArgs(1),
		Example: `  barn eval --db game.db '1 + 2'
  barn eval --db game.db 'x = 1; return x + 1;'
  barn eval --db game.db --file probes.moo
  barn eval --db game.db -- '-1 + 2'   # code starting with "-" goes after --`,
	}
	cmd.Flags().StringVarP(&b.cfg.EvalFile, "file", "f", "", "Evaluate one MOO input per line of this `file`")
	b.addOptionFlags(cmd.Flags())
	cmd.RunE = b.run(func(_ *cobra.Command, args []string) error {
		if (len(args) == 1) == (b.cfg.EvalFile != "") {
			return cli.UsageErrorf("give either code or --file")
		}
		if len(args) == 1 {
			b.cfg.Eval = args[0]
		}
		return nil
	})
	return cmd
}

// inspectCommands are the read-only database inspections that each take one
// object or pattern argument.
func (b *barnCommand) inspectCommands() []*cobra.Command {
	specs := []struct {
		use, short, example string
		field               *string
	}{
		{"verb-code <#obj:verb>", "Print a verb's source", "barn verb-code --db game.db '#0:do_login_command'", &b.cfg.VerbCode},
		{"list-verbs <#obj>", "List the verbs defined on an object", "barn list-verbs --db game.db '#0'", &b.cfg.ListVerbs},
		{"obj-info <#obj>", "Show an object's fields", "barn obj-info --db game.db '#2'", &b.cfg.ObjectInfo},
		{"dump-obj-raw <#obj>", "Dump an object's raw database fields", "barn dump-obj-raw --db game.db '#2'", &b.cfg.DumpObjectRaw},
		{"verb-lookup <#obj:verb>", "Show where a verb would be found", "barn verb-lookup --db game.db '#2:look'", &b.cfg.VerbLookup},
		{"ancestry <#obj>", "Show an object's full parent chain", "barn ancestry --db game.db '#2'", &b.cfg.Ancestry},
		{"grep-verbs <regexp>", "Print every verb source line matching a regular expression, as #obj:verb:line: text", `barn grep-verbs --db game.db 'notify\('`, &b.cfg.GrepVerbs},
	}
	commands := make([]*cobra.Command, 0, len(specs))
	for _, spec := range specs {
		field := spec.field
		cmd := &cobra.Command{
			Use:     spec.use,
			Short:   spec.short,
			GroupID: "inspect",
			Args:    cobra.ExactArgs(1),
			Example: "  " + spec.example,
		}
		b.addOptionFlags(cmd.Flags())
		cmd.RunE = b.run(func(_ *cobra.Command, args []string) error {
			*field = args[0]
			return nil
		})
		commands = append(commands, cmd)
	}
	return commands
}

func (b *barnCommand) toastCheckCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "toast-check",
		Short: "Name every verb whose code a Toast cannot run as written",
		Long: `Name every verb whose code calls a builtin the target Toast does not register,
or does not parse, then print a count line. --builtins lists the target Toast's
builtin names, one per line, as its function_info() gives them. Calls through
eval or a computed call_function name are listed as not checkable. Exits 1 when
any verb calls a builtin Barn registers and that Toast does not, or does not
parse; a call to a name neither registers is listed but does not fail.`,
		GroupID: "inspect",
		Args:    cobra.NoArgs,
		Example: "  barn toast-check --db game.db --builtins toast-builtins.txt",
	}
	cmd.Flags().StringVar(&b.cfg.ToastBuiltins, "builtins", "", "The target Toast's builtin names, one per line, in this `file`")
	b.addOptionFlags(cmd.Flags())
	cmd.RunE = b.run(func(*cobra.Command, []string) error {
		if b.cfg.ToastBuiltins == "" {
			return cli.UsageErrorf("--builtins is required")
		}
		return nil
	})
	return cmd
}

func (b *barnCommand) dumpCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "dump <output.db>",
		Short:   "Write the database to a new file, reload it, and verify it matches",
		GroupID: "files",
		Args:    cobra.ExactArgs(1),
		Example: "  barn dump --db game.db copy.db",
	}
	cmd.RunE = b.run(func(_ *cobra.Command, args []string) error {
		b.cfg.DumpPath = args[0]
		return nil
	})
	return cmd
}

func (b *barnCommand) offlineCheckpointCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "offline-checkpoint",
		Short: "Write the database as a full checkpoint without running anything",
		Long: `Write the database as a full checkpoint (the --db path plus .new, with its tasks,
connections and WAIF sidecar) without running any MOO code.`,
		GroupID: "files",
		Args:    cobra.NoArgs,
		Example: "  barn offline-checkpoint --db game.db --omit-suspended",
	}
	cmd.Flags().BoolVar(&b.cfg.OmitSuspended, "omit-suspended", false, "Leave suspended and interrupted tasks out of the written file")
	b.addOptionFlags(cmd.Flags())
	cmd.RunE = b.run(func(*cobra.Command, []string) error {
		b.cfg.OfflineCheckpoint = true
		return nil
	})
	return cmd
}

func (b *barnCommand) memoryReportCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "memory-report",
		Short:   "Load the database, collect garbage, and print retained heap and property-slot counts",
		GroupID: "files",
		Args:    cobra.NoArgs,
		Example: "  barn memory-report --db game.db --profile heap.pprof",
	}
	cmd.Flags().StringVar(&b.cfg.MemoryProfile, "profile", "", "Also write a pprof heap profile to this `path`")
	cmd.RunE = b.run(func(*cobra.Command, []string) error {
		b.cfg.MemoryReport = true
		return nil
	})
	return cmd
}

func (b *barnCommand) listProfilesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list-profiles",
		Short:   "List the known managed conformance profiles",
		GroupID: "server",
		Args:    cobra.NoArgs,
	}
	cmd.Flags().StringVar(&b.cfg.ProfileRegistry, "registry", b.cfg.ProfileRegistry, "Managed profile registry `file`")
	cmd.RunE = b.run(func(*cobra.Command, []string) error {
		b.cfg.ListProfiles = true
		return nil
	})
	return cmd
}

// run returns a command body that applies set (the command's own arguments),
// sets up logging and the Go runtime, and runs app.Run.
func (b *barnCommand) run(set func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
	return cli.RunE(func(cmd *cobra.Command, args []string) error {
		if set != nil {
			if err := set(cmd, args); err != nil {
				return err
			}
		}
		level, err := b.logs.Level()
		if err != nil {
			return err
		}
		b.cfg.LogLevel = level
		flags := cmd.Flags()
		b.cfg.Listen = b.listen
		b.cfg.PortProvided = flags.Changed("port")
		b.cfg.OutboundProvided = flags.Changed("outbound")
		b.cfg.NoOutboundProvided = flags.Changed("no-outbound")
		return b.execute(cmd.OutOrStdout(), cmd.ErrOrStderr())
	})
}

func (b *barnCommand) execute(out, errOut io.Writer) error {
	closeLogs, err := logging.Setup(logging.Options{LevelStr: b.cfg.LogLevel, Dir: b.cfg.LogDir})
	if err != nil {
		return fmt.Errorf("logging setup failed: %w", err)
	}
	defer closeLogs()
	// Only the elle checker build has this test-only stop; elsewhere it is a no-op.
	if err := isolationprobe.InstallCheckpointStop(errOut); err != nil {
		slog.Error("checkpoint stop setup failed", slog.Any("err", err))
		return cli.Exit(cli.ExitFailed)
	}
	if b.gomem > 0 {
		debug.SetMemoryLimit(int64(b.gomem) * 1024 * 1024)
	}
	if b.gogc >= 0 {
		debug.SetGCPercent(b.gogc)
	} else if os.Getenv("GOGC") == "" {
		debug.SetGCPercent(config.DefaultGCPercent)
	}

	ctx, stop := interruptContext()
	defer stop()
	err = app.Run(ctx, b.cfg, out, errOut)
	if errors.Is(err, server.ErrPanicShutdown) {
		slog.Error("server panic shutdown", slog.Any("err", err))
		closeLogs()
		terminatePanicShutdown()
	}
	if err != nil {
		slog.Error("Barn failed", slog.Any("err", err))
		return cli.Exit(cli.ExitFailed)
	}
	return nil
}

func buildListenerSpecs(port int, listens []string, provided bool) ([]listener.Spec, error) {
	return app.BuildListenerSpecs(port, listens, provided)
}
