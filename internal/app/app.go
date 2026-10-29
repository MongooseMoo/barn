package app

import (
	"context"
	"errors"
	"expvar"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"os"
	"os/exec"
	"runtime/debug"
	"strings"
	"time"

	"github.com/MongooseMoo/barn/config"
	"github.com/MongooseMoo/barn/internal/dbtool"
	"github.com/MongooseMoo/barn/internal/listener"
	"github.com/MongooseMoo/barn/logging"
	"github.com/MongooseMoo/barn/profile"
	"github.com/MongooseMoo/barn/server"
	"github.com/MongooseMoo/barn/trace"
)

// logLevelHandler reads and sets the log level of the running server, so that a
// server can be turned up to debug while it misbehaves and turned back down
// afterwards — without a restart, which would destroy the state you wanted to
// look at. This lives on the admin endpoint rather than in a MOO builtin because
// Barn's MOO surface is ToastStunt's, and Toast has no such function.
//
//	curl localhost:PORT/debug/loglevel
//	curl -X POST 'localhost:PORT/debug/loglevel?level=debug'
func logLevelHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost || r.Method == http.MethodPut {
		want := r.URL.Query().Get("level")
		level, err := logging.ParseLevel(want)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		was := logging.LevelName()
		logging.Level.Set(level)
		slog.Warn("log level changed",
			slog.String("from", was),
			slog.String("to", logging.LevelName()))
	}
	fmt.Fprintln(w, logging.LevelName())
}

// startDebugEndpoint serves pprof and expvar on a local address. The default
// port is ephemeral so that several servers (the conformance runner starts them
// in parallel) never collide; the address that was actually bound is logged, and
// that log line is how you find it.
//
// The handlers are registered explicitly rather than by importing net/http/pprof
// for its side effects, because that only populates http.DefaultServeMux — and
// exposing pprof on a mux the rest of the program might serve publicly is how it
// ends up on the open internet.
func debugMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/debug/vars", expvar.Handler())
	mux.HandleFunc("/debug/loglevel", logLevelHandler)
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	mux.HandleFunc("/debug/pprof/moo", mooProfileHandler)

	return mux
}

func BuildListenerSpecs(port int, listenFlags []string, portProvided bool) ([]listener.Spec, error) {
	if len(listenFlags) == 0 {
		return listener.DefaultSpecs(port), nil
	}
	if portProvided {
		return nil, fmt.Errorf("cannot combine -port with -listen")
	}

	listenerSpecs := make([]listener.Spec, 0, len(listenFlags))
	for _, raw := range listenFlags {
		spec, err := listener.ParseSpec(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid -listen value: %w", err)
		}
		listenerSpecs = append(listenerSpecs, spec)
	}
	return listenerSpecs, nil
}

func formatListenerSpecs(specs []listener.Spec) string {
	parts := make([]string, 0, len(specs))
	for _, spec := range specs {
		if spec.Path != "" {
			parts = append(parts, fmt.Sprintf("%s://%s:%d%s", spec.Protocol, spec.Interface, spec.Port, spec.Path))
			continue
		}
		parts = append(parts, fmt.Sprintf("%s://%s:%d", spec.Protocol, spec.Interface, spec.Port))
	}
	return strings.Join(parts, ", ")
}

func gitImplementationRef() string {
	// Optional provenance must not indefinitely delay loading or listening.
	// Both commands consume this one budget, including time spent reading HEAD.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	commit, err := gitOutput(ctx, "rev-parse", "HEAD")
	if err != nil || !validGitCommit(commit) {
		return "unknown tracked_dirty=unknown"
	}
	dirty := "unknown"
	cmd := exec.CommandContext(ctx, "git", "diff", "--quiet", "--")
	cmd.WaitDelay = 100 * time.Millisecond
	err = cmd.Run()
	if ctx.Err() == nil {
		if err == nil {
			dirty = "false"
		} else {
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
				dirty = "true"
			}
		}
	}
	return fmt.Sprintf("%s tracked_dirty=%s", commit, dirty)
}

func gitOutput(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	// A child can exit while a descendant still holds its output pipe open.
	cmd.WaitDelay = 100 * time.Millisecond
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func validGitCommit(commit string) bool {
	if len(commit) != 40 && len(commit) != 64 {
		return false
	}
	for i := range len(commit) {
		c := commit[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}

func printProfiles(out, errOut io.Writer, registry profile.Registry) {
	for _, entry := range registry.SortedProfiles() {
		fmt.Fprintf(out, "%s\t%s\t%s\t%s\t%s\n",
			entry.ProfileID,
			entry.Implementation,
			entry.RuntimeOS,
			entry.DatabaseFixture,
			entry.SupportStatus)
	}
}

// Config describes one Barn invocation independently of command-line parsing.
type Config struct {
	DatabasePath                                                               string
	Port                                                                       int
	Listen                                                                     []string
	PortProvided                                                               bool
	ConfigPath, ProfileID, ProfileManifest, ProfileRegistry                    string
	ListProfiles                                                               bool
	LogLevel, LogDir, DebugAddr, OperatorAddr                                  string
	TraceEnabled                                                               bool
	TraceFilter                                                                string
	VerbCode, ListVerbs, ObjectInfo, Eval, DumpObjectRaw, VerbLookup, Ancestry string
	EvalFile                                                                   string
	StartLine, StartScript                                                     *string
	StartScriptFirst                                                           bool
	DumpPath                                                                   string
	MemoryReport                                                               bool
	MemoryProfile                                                              string
	CheckpointInterval                                                         int
	PromoteNumbers                                                             bool
	OutboundProvided, NoOutboundProvided, Outbound, NoOutbound                 bool
}

func DefaultConfig() Config {
	return Config{DatabasePath: "Test.db", Port: 7777, ProfileRegistry: "profiles/barn/profiles.json", LogLevel: "info", LogDir: "logs", DebugAddr: "127.0.0.1:0", OperatorAddr: "127.0.0.1:0", CheckpointInterval: 3600}
}

// Run executes a configured Barn invocation. It never terminates the process;
// all application failures are returned to the caller.
func Run(ctx context.Context, cfg Config, out, errOut io.Writer) error {
	if out == nil {
		out = io.Discard
	}
	if errOut == nil {
		errOut = io.Discard
	}
	if cfg.ListProfiles {
		registry, err := profile.LoadRegistry(cfg.ProfileRegistry)
		if err != nil {
			return fmt.Errorf("load profile registry: %w", err)
		}
		printProfiles(out, errOut, registry)
		return nil
	}
	options := config.DefaultOptions()
	if cfg.ConfigPath != "" {
		loaded, err := config.LoadFile(cfg.ConfigPath)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		options = loaded
	}
	if cfg.OutboundProvided && cfg.NoOutboundProvided {
		return errors.New("cannot combine --outbound and --no-outbound")
	}
	if cfg.OutboundProvided && cfg.Outbound {
		options.OutboundNetwork = true
	}
	if cfg.NoOutboundProvided && cfg.NoOutbound {
		options.OutboundNetwork = false
	}
	if cfg.PromoteNumbers {
		options.PromoteNumbers = true
	}
	if err := options.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	if cfg.ProfileManifest != "" && cfg.ProfileID == "" {
		return errors.New("--profile-id is required with --profile-manifest")
	}
	if cfg.ProfileID != "" && cfg.ConfigPath == "" {
		return errors.New("--config is required with --profile-id")
	}
	if cfg.DumpPath != "" {
		return dbtool.DumpDatabase(cfg.DatabasePath, cfg.DumpPath)
	}
	if cfg.MemoryProfile != "" && !cfg.MemoryReport {
		return errors.New("--memory-profile requires --memory-report")
	}
	if cfg.MemoryReport {
		return dbtool.MemoryReport(out, cfg.DatabasePath, cfg.MemoryProfile)
	}
	if cfg.VerbCode != "" || cfg.ListVerbs != "" || cfg.ObjectInfo != "" || cfg.Eval != "" || cfg.EvalFile != "" || cfg.DumpObjectRaw != "" || cfg.VerbLookup != "" || cfg.Ancestry != "" {
		store, err := dbtool.LoadStore(cfg.DatabasePath, options)
		if err != nil {
			return err
		}
		// As the server does after loading: return what the load allocated
		// and no longer needs, so -eval 'memory_usage()' reports what a
		// started server holds.
		debug.FreeOSMemory()
		// Inspection helpers retain the established textual contracts.
		if cfg.VerbCode != "" {
			err := dbtool.DumpVerbCode(out, errOut, store, cfg.VerbCode)
			if err != nil {
				return err
			}
		}
		if cfg.ListVerbs != "" {
			err := dbtool.DumpListVerbs(out, errOut, store, cfg.ListVerbs)
			if err != nil {
				return err
			}
		}
		if cfg.ObjectInfo != "" {
			err := dbtool.DumpObjInfo(out, errOut, store, cfg.ObjectInfo)
			if err != nil {
				return err
			}
		}
		if cfg.Eval != "" {
			err := dbtool.EvalExpression(out, errOut, store, cfg.Eval, options)
			if err != nil {
				return err
			}
		}
		if cfg.EvalFile != "" {
			err := dbtool.EvalFile(out, errOut, store, cfg.EvalFile, options)
			if err != nil {
				return err
			}
		}
		if cfg.DumpObjectRaw != "" {
			err := dbtool.DumpObjRawCommand(out, errOut, store, cfg.DumpObjectRaw)
			if err != nil {
				return err
			}
		}
		if cfg.VerbLookup != "" {
			err := dbtool.VerbLookupCommand(out, errOut, store, cfg.VerbLookup)
			if err != nil {
				return err
			}
		}
		if cfg.Ancestry != "" {
			err := dbtool.AncestryCommand(out, errOut, store, cfg.Ancestry)
			if err != nil {
				return err
			}
		}
		return nil
	}
	specs, err := BuildListenerSpecs(cfg.Port, cfg.Listen, cfg.PortProvided)
	if err != nil {
		return err
	}
	slog.Info("Barn MOO Server", slog.String("database", cfg.DatabasePath), slog.String("listeners", formatListenerSpecs(specs)))
	if cfg.TraceEnabled {
		filters := strings.Split(cfg.TraceFilter, ",")
		trace.Init(true, filters, errOut)
	} else {
		trace.Init(false, nil, nil)
	}
	state := newLifecycle()
	srv, err := server.NewServerWithOptions(cfg.DatabasePath, specs, cfg.CheckpointInterval, options)
	if err != nil {
		state.Failed()
		return fmt.Errorf("create server: %w", err)
	}
	// Managed clients use a TCP connection as their startup signal. Publish
	// complete profile evidence before reserving a connectable game socket.
	if cfg.ProfileManifest != "" {
		manifest, err := profile.BuildManifest(profile.BuildInput{ProfileID: cfg.ProfileID, ImplementationRef: gitImplementationRef(), DatabasePath: cfg.DatabasePath, ConfigPath: cfg.ConfigPath, Options: options, Registry: srv.BuiltinRegistry()})
		if err != nil {
			return fmt.Errorf("build profile manifest: %w", err)
		}
		if err := profile.WriteManifest(cfg.ProfileManifest, manifest); err != nil {
			return fmt.Errorf("write profile manifest: %w", err)
		}
	}
	releaseListeners, err := srv.ReserveListeners()
	if err != nil {
		state.Failed()
		return fmt.Errorf("reserve listeners: %w", err)
	}
	defer releaseListeners()
	if cfg.DebugAddr != "off" && cfg.DebugAddr != "" {
		debugSrv, err := startHTTPEndpoint("debug", cfg.DebugAddr, debugMux())
		if err != nil {
			slog.Warn("debug endpoint unavailable", slog.Any("err", err))
		} else {
			defer debugSrv.shutdown()
			if cfg.LogDir != "" {
				writeDebugAddr(cfg.LogDir, debugSrv.addr)
			}
		}
	}
	if cfg.OperatorAddr != "off" && cfg.OperatorAddr != "" {
		operatorSrv, err := startHTTPEndpoint("operator", cfg.OperatorAddr, operatorMux(state))
		if err != nil {
			slog.Warn("operator endpoint unavailable", slog.Any("err", err))
		} else {
			defer operatorSrv.shutdown()
		}
	}
	srv.SetLifecycleObserver(state)
	if err := srv.LoadDatabase(); err != nil {
		state.Failed()
		return fmt.Errorf("load database: %w", err)
	}
	watchCtx, stopWatch := context.WithCancel(ctx)
	defer stopWatch()
	startMemWatch(watchCtx, srv.Runtime(), cfg.LogDir)
	var scriptLines []string
	if cfg.StartScript != nil {
		data, err := os.ReadFile(*cfg.StartScript)
		if err != nil {
			return fmt.Errorf("read startup script: %w", err)
		}
		scriptLines = []string{}
		if len(data) > 0 {
			scriptLines = strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
		}
	}
	if cfg.StartScriptFirst && cfg.StartScript != nil {
		srv.StartScripts = append(srv.StartScripts, scriptLines)
	}
	if cfg.StartLine != nil {
		srv.StartScripts = append(srv.StartScripts, []string{*cfg.StartLine})
	}
	if !cfg.StartScriptFirst && cfg.StartScript != nil {
		srv.StartScripts = append(srv.StartScripts, scriptLines)
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			srv.Shutdown("Server shutdown")
		case <-done:
		}
	}()
	err = srv.Start()
	close(done)
	if err != nil {
		return fmt.Errorf("server: %w", err)
	}
	return nil
}
