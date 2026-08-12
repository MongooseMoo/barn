package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"

	"github.com/MongooseMoo/barn/internal/app"
	"github.com/MongooseMoo/barn/internal/listener"
	"github.com/MongooseMoo/barn/logging"
	"github.com/MongooseMoo/barn/server"
)

const defaultGOGCPercent = 400

type stringListFlag []string

func (f *stringListFlag) String() string         { return fmt.Sprint([]string(*f)) }
func (f *stringListFlag) Set(value string) error { *f = append(*f, value); return nil }

func main() {
	cfg := app.DefaultConfig()
	flag.StringVar(&cfg.DatabasePath, "db", cfg.DatabasePath, "Database file path")
	flag.IntVar(&cfg.Port, "port", cfg.Port, "Listen port")
	flag.StringVar(&cfg.ConfigPath, "config", "", "Server config file path")
	flag.StringVar(&cfg.ProfileID, "profile-id", "", "Managed profile identifier")
	flag.StringVar(&cfg.ProfileManifest, "profile-manifest", "", "Path to write managed profile metadata")
	flag.StringVar(&cfg.ProfileRegistry, "profile-registry", cfg.ProfileRegistry, "Managed profile registry path")
	flag.BoolVar(&cfg.ListProfiles, "list-profiles", false, "List known managed profiles and exit")
	var listens stringListFlag
	flag.Var(&listens, "listen", "Listener URL; repeatable, e.g. tcp://:7777")
	flag.StringVar(&cfg.LogLevel, "log-level", cfg.LogLevel, "Minimum log level: debug, info, warn, or error")
	flag.StringVar(&cfg.LogDir, "log-dir", cfg.LogDir, "Directory for JSON log files (empty disables the file sink)")
	flag.StringVar(&cfg.DebugAddr, "debug-addr", cfg.DebugAddr, "Address for the pprof/expvar endpoint (\"off\" to disable)")
	flag.StringVar(&cfg.OperatorAddr, "operator-addr", cfg.OperatorAddr, "Address for passive liveness/readiness probes (\"off\" to disable)")
	flag.BoolVar(&cfg.TraceEnabled, "trace", false, "Enable execution tracing")
	flag.StringVar(&cfg.TraceFilter, "trace-filter", "", "Trace filter pattern")
	flag.StringVar(&cfg.VerbCode, "verb-code", "", "Dump verb code for #obj:verb")
	flag.StringVar(&cfg.ListVerbs, "list-verbs", "", "List all verbs on an object")
	flag.StringVar(&cfg.ObjectInfo, "obj-info", "", "Show object info")
	flag.StringVar(&cfg.Eval, "eval", "", "Evaluate a MOO expression")
	flag.StringVar(&cfg.DumpObjectRaw, "dump-obj-raw", "", "Dump raw database fields for an object")
	flag.StringVar(&cfg.VerbLookup, "verb-lookup", "", "Show where a verb would be found")
	flag.StringVar(&cfg.Ancestry, "ancestry", "", "Show full parent chain")
	flag.StringVar(&cfg.DumpPath, "dump", "", "Dump database to path and exit")
	flag.IntVar(&cfg.CheckpointInterval, "checkpoint-interval", cfg.CheckpointInterval, "Checkpoint interval in seconds (0=disabled)")
	flag.BoolVar(&cfg.PromoteNumbers, "promote-numbers", false, "Enable numeric promotion")
	var outbound, outboundShort, noOutbound, noOutboundShort bool
	flag.BoolVar(&outbound, "outbound", false, "Enable outbound network connections, overriding --config")
	flag.BoolVar(&outboundShort, "o", false, "Alias for --outbound")
	flag.BoolVar(&noOutbound, "no-outbound", false, "Disable outbound network connections, overriding --config")
	flag.BoolVar(&noOutboundShort, "O", false, "Alias for --no-outbound")
	gomem := flag.Int("gomemlimit-mib", 0, "Soft memory limit in MiB")
	gogc := flag.Int("gogc", -1, "GC target percentage")
	flag.Parse()
	cfg.Listen = listens
	cfg.PortProvided = flagWasProvided("port")
	cfg.OutboundProvided = flagWasProvided("outbound") || flagWasProvided("o")
	cfg.NoOutboundProvided = flagWasProvided("no-outbound") || flagWasProvided("O")
	cfg.Outbound = outbound || outboundShort
	cfg.NoOutbound = noOutbound || noOutboundShort

	closeLogs, err := logging.Setup(logging.Options{LevelStr: cfg.LogLevel, Dir: cfg.LogDir})
	if err != nil {
		fmt.Fprintf(os.Stderr, "logging setup failed: %v\n", err)
		os.Exit(1)
	}
	defer closeLogs()
	if *gomem > 0 {
		debug.SetMemoryLimit(int64(*gomem) * 1024 * 1024)
	}
	if *gogc >= 0 {
		debug.SetGCPercent(*gogc)
	} else if os.Getenv("GOGC") == "" {
		debug.SetGCPercent(defaultGOGCPercent)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	err = app.Run(ctx, cfg, os.Stdout, os.Stderr)
	if errors.Is(err, server.ErrPanicShutdown) {
		slog.Error("server panic shutdown", slog.Any("err", err))
		closeLogs()
		terminatePanicShutdown()
	}
	if err != nil {
		slog.Error("Barn failed", slog.Any("err", err))
		os.Exit(1)
	}
}

func flagWasProvided(name string) bool {
	short, long := "-"+name, "--"+name
	for _, arg := range os.Args[1:] {
		if arg == short || arg == long || strings.HasPrefix(arg, short+"=") || strings.HasPrefix(arg, long+"=") {
			return true
		}
	}
	return false
}

func buildListenerSpecs(port int, listens []string, provided bool) ([]listener.Spec, error) {
	return app.BuildListenerSpecs(port, listens, provided)
}
