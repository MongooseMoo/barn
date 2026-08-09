package main

import (
	"errors"
	"expvar"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"os/exec"
	"os/signal"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/compiler"
	"github.com/MongooseMoo/barn/config"
	dbformat "github.com/MongooseMoo/barn/db/format"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/logging"
	"github.com/MongooseMoo/barn/profile"
	"github.com/MongooseMoo/barn/server"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/trace"
	"github.com/MongooseMoo/barn/types"
	"github.com/MongooseMoo/barn/vm"
)

// defaultGOGCPercent is Barn's on-by-default GC budget. The Go default
// (GOGC=100) makes the collector the binding constraint on 32-core throughput
// because the VM heap-boxes values; a measured 32-worker sweep on the realistic
// verb-call path showed parallel speedup rising with the budget:
// 100->4.56x, 200->6.78x, 400->8.50x, 800->9.76x, off->11.41x. 400 buys ~8.5x
// at ~4x heap growth between collections — a balanced default for unknown
// deployment RAM (operators can push -gogc toward 800 with -gomemlimit-mib as a
// backstop). The durable fix (unboxing values) is deferred.
const defaultGOGCPercent = 400

// fatalf logs a fatal startup error and exits. Unlike log.Fatalf it goes
// through slog, so the failure that killed a run is in the run's log file.
func fatalf(format string, args ...any) {
	slog.Error(fmt.Sprintf(format, args...))
	os.Exit(1)
}

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
func startDebugEndpoint(addr string) (*http.Server, error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", addr, err)
	}

	mux := http.NewServeMux()
	mux.Handle("/debug/vars", expvar.Handler())
	mux.HandleFunc("/debug/loglevel", logLevelHandler)
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	srv := &http.Server{Handler: mux}
	slog.Info("debug endpoint listening", slog.String("addr", listener.Addr().String()))

	go func() {
		if err := srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Warn("debug endpoint stopped", slog.Any("err", err))
		}
	}()
	return srv, nil
}

type stringListFlag []string

func (f *stringListFlag) String() string {
	return strings.Join(*f, ",")
}

func (f *stringListFlag) Set(value string) error {
	*f = append(*f, value)
	return nil
}

func main() {
	dbPath := flag.String("db", "Test.db", "Database file path")
	port := flag.Int("port", 7777, "Listen port")
	configPath := flag.String("config", "", "Server config file path")
	profileID := flag.String("profile-id", "", "Managed profile identifier")
	profileManifest := flag.String("profile-manifest", "", "Path to write managed profile metadata")
	profileRegistry := flag.String("profile-registry", "profiles/barn/profiles.json", "Managed profile registry path")
	listProfiles := flag.Bool("list-profiles", false, "List known managed profiles and exit")
	var listenFlags stringListFlag
	flag.Var(&listenFlags, "listen", "Listener URL; repeatable, e.g. tcp://:7777")

	// Logging flags
	logLevel := flag.String("log-level", "info", "Minimum log level: debug, info, warn, or error")
	logDir := flag.String("log-dir", "logs", "Directory for JSON log files (empty disables the file sink)")
	debugAddr := flag.String("debug-addr", "127.0.0.1:0", "Address for the pprof/expvar endpoint (\"off\" to disable)")

	// Trace flags
	traceEnabled := flag.Bool("trace", false, "Enable execution tracing")
	traceFilter := flag.String("trace-filter", "", "Trace filter pattern (glob, e.g., 'do_*' or 'user_*')")

	// Inspection flags
	verbCode := flag.String("verb-code", "", "Dump verb code for #obj:verb (e.g., #0:do_login_command)")
	listVerbs := flag.String("list-verbs", "", "List all verbs on an object (e.g., #0)")
	objInfo := flag.String("obj-info", "", "Show object info (e.g., #0)")
	evalExpr := flag.String("eval", "", "Evaluate a MOO expression (e.g., \"1 + 2\")")
	dumpObjRaw := flag.String("dump-obj-raw", "", "Dump raw database fields for an object (e.g., #39)")
	verbLookup := flag.String("verb-lookup", "", "Show where a verb would be found (e.g., #39:find_exact)")
	ancestry := flag.String("ancestry", "", "Show full parent chain for an object (e.g., #39)")

	// Database operations
	dumpPath := flag.String("dump", "", "Dump database to path and exit")
	checkpointInterval := flag.Int("checkpoint-interval", 3600, "Checkpoint interval in seconds (0=disabled)")

	// Numeric semantics
	promoteNumbers := flag.Bool("promote-numbers", false, "Enable ToastStunt mongoose PROMOTE_NUMBERS: auto-promote int to float in mixed int/float arithmetic and comparison (default off = strict E_TYPE)")
	outbound := flag.Bool("outbound", false, "Enable outbound network connections, overriding --config")
	outboundShort := flag.Bool("o", false, "Alias for --outbound")
	noOutbound := flag.Bool("no-outbound", false, "Disable outbound network connections, overriding --config")
	noOutboundShort := flag.Bool("O", false, "Alias for --no-outbound")

	// Runtime GC tuning (0/-1 leave Go's GOMEMLIMIT/GOGC env honoring intact)
	gomemlimitMiB := flag.Int("gomemlimit-mib", 0, "Soft memory limit in MiB (0=unset, honor GOMEMLIMIT env)")
	gogc := flag.Int("gogc", -1, "GC target percentage (-1 = use Barn default 400 or GOGC env)")

	flag.Parse()

	closeLogs, err := logging.Setup(logging.Options{LevelStr: *logLevel, Dir: *logDir})
	if err != nil {
		fmt.Fprintf(os.Stderr, "logging setup failed: %v\n", err)
		os.Exit(1)
	}
	defer closeLogs()

	// Apply GC overrides only when explicitly set; otherwise leave Go's
	// automatic GOMEMLIMIT/GOGC env-var honoring untouched.
	if *gomemlimitMiB > 0 {
		debug.SetMemoryLimit(int64(*gomemlimitMiB) * 1024 * 1024)
		slog.Info("GC memory limit set", slog.Int("mib", *gomemlimitMiB))
	}
	// GC-percent precedence: explicit -gogc flag > GOGC env > Barn default 400.
	if *gogc >= 0 {
		// Explicit override wins.
		debug.SetGCPercent(*gogc)
		slog.Info("GC target set", slog.Int("percent", *gogc))
	} else if os.Getenv("GOGC") != "" {
		// Operator set GOGC in the environment; Go already honored it at
		// startup, so respect it and do nothing.
	} else {
		// Barn's on-by-default tuned budget.
		debug.SetGCPercent(defaultGOGCPercent)
		slog.Info("GC target set (default)", slog.Int("percent", defaultGOGCPercent))
	}

	if *listProfiles {
		registry, err := profile.LoadRegistry(*profileRegistry)
		if err != nil {
			fatalf("Failed to load profile registry: %v", err)
		}
		printProfiles(registry)
		return
	}

	options := config.DefaultOptions()
	if *configPath != "" {
		loaded, err := config.LoadFile(*configPath)
		if err != nil {
			fatalf("Failed to load config: %v", err)
		}
		options = loaded
	}
	outboundProvided := flagWasProvided("outbound") || flagWasProvided("o")
	noOutboundProvided := flagWasProvided("no-outbound") || flagWasProvided("O")
	if outboundProvided && noOutboundProvided {
		fatalf("cannot combine --outbound and --no-outbound")
	}
	if outboundProvided && (*outbound || *outboundShort) {
		options.OutboundNetwork = true
	}
	if noOutboundProvided && (*noOutbound || *noOutboundShort) {
		options.OutboundNetwork = false
	}
	if *promoteNumbers {
		options.PromoteNumbers = true
	}
	if err := options.Validate(); err != nil {
		fatalf("Invalid config: %v", err)
	}
	if *profileManifest != "" && *profileID == "" {
		fatalf("--profile-id is required with --profile-manifest")
	}
	if *profileID != "" && *configPath == "" {
		fatalf("--config is required with --profile-id")
	}

	// Handle -dump flag: dump database and exit
	if *dumpPath != "" {
		database, err := dbformat.LoadDatabase(*dbPath)
		if err != nil {
			fatalf("Failed to load database: %v", err)
		}
		store := database.NewStoreFromDatabase()

		f, err := os.Create(*dumpPath)
		if err != nil {
			fatalf("Failed to create dump file: %v", err)
		}

		writer := dbformat.NewWriter(f, store.Snapshot())
		if err := writer.WriteDatabase(); err != nil {
			f.Close()
			fatalf("Failed to write database: %v", err)
		}
		f.Close()

		slog.Info("database dumped", slog.String("path", *dumpPath))
		return
	}

	// Check if any inspection flag is set
	isInspection := *verbCode != "" || *listVerbs != "" || *objInfo != "" || *evalExpr != "" ||
		*dumpObjRaw != "" || *verbLookup != "" || *ancestry != ""

	if isInspection {
		// Load database for inspection
		database, err := dbformat.LoadDatabase(*dbPath)
		if err != nil {
			fatalf("Failed to load database: %v", err)
		}
		store := database.NewStoreFromDatabase()

		if *verbCode != "" {
			dumpVerbCode(store, *verbCode)
		}
		if *listVerbs != "" {
			dumpListVerbs(store, *listVerbs)
		}
		if *objInfo != "" {
			dumpObjInfo(store, *objInfo)
		}
		if *evalExpr != "" {
			evalExpression(store, *evalExpr, options)
		}
		if *dumpObjRaw != "" {
			dumpObjRawCommand(store, *dumpObjRaw)
		}
		if *verbLookup != "" {
			verbLookupCommand(store, *verbLookup)
		}
		if *ancestry != "" {
			ancestryCommand(store, *ancestry)
		}
		return
	}

	// Normal server startup
	listenerSpecs, err := buildListenerSpecs(*port, listenFlags, flagWasProvided("port"))
	if err != nil {
		fatalf("%v", err)
	}
	startup := []any{
		slog.String("database", *dbPath),
		slog.String("listeners", formatListenerSpecs(listenerSpecs)),
	}
	if *configPath != "" {
		startup = append(startup, slog.String("config", *configPath))
	}
	slog.Info("Barn MOO Server", startup...)

	// Initialize tracer
	if *traceEnabled {
		var filters []string
		if *traceFilter != "" {
			filters = strings.Split(*traceFilter, ",")
			for i := range filters {
				filters[i] = strings.TrimSpace(filters[i])
			}
		}
		trace.Init(true, filters, os.Stderr)
		slog.Info("tracing enabled", slog.Any("filters", filters))
	} else {
		trace.Init(false, nil, nil)
	}

	if *debugAddr != "off" && *debugAddr != "" {
		debugSrv, err := startDebugEndpoint(*debugAddr)
		if err != nil {
			// A missing debug endpoint is not worth refusing to serve MOO over.
			slog.Warn("debug endpoint unavailable", slog.Any("err", err))
		} else {
			defer debugSrv.Close()
		}
	}

	srv, err := server.NewServerWithOptions(*dbPath, listenerSpecs, *checkpointInterval, options)
	if err != nil {
		fatalf("Failed to create server: %v", err)
	}

	if *profileManifest != "" {
		manifest, err := profile.BuildManifest(profile.BuildInput{
			ProfileID:         *profileID,
			ImplementationRef: gitImplementationRef(),
			DatabasePath:      *dbPath,
			ConfigPath:        *configPath,
			Options:           options,
		})
		if err != nil {
			fatalf("Failed to build profile manifest: %v", err)
		}
		if err := profile.WriteManifest(*profileManifest, manifest); err != nil {
			fatalf("Failed to write profile manifest: %v", err)
		}
		slog.Info("profile manifest written", slog.String("path", *profileManifest))
	}

	if err := srv.LoadDatabase(); err != nil {
		fatalf("Failed to load database: %v", err)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	signalDone := make(chan struct{})
	go func() {
		select {
		case <-sigChan:
			slog.Info("received shutdown signal")
			srv.Shutdown("Server shutdown")
		case <-signalDone:
		}
	}()

	slog.Info("starting server")
	err = srv.Start()
	close(signalDone)
	signal.Stop(sigChan)
	if err != nil {
		if errors.Is(err, server.ErrPanicShutdown) {
			slog.Error("server panic shutdown", slog.Any("err", err))
			closeLogs()
			os.Exit(1)
		}
		fatalf("Server error: %v", err)
	}
}

func buildListenerSpecs(port int, listenFlags []string, portProvided bool) ([]builtins.ListenerSpec, error) {
	if len(listenFlags) == 0 {
		return server.DefaultListenSpecs(port), nil
	}
	if portProvided {
		return nil, fmt.Errorf("cannot combine -port with -listen")
	}

	listenerSpecs := make([]builtins.ListenerSpec, 0, len(listenFlags))
	for _, raw := range listenFlags {
		spec, err := server.ParseListenSpec(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid -listen value: %w", err)
		}
		listenerSpecs = append(listenerSpecs, spec)
	}
	return listenerSpecs, nil
}

func flagWasProvided(name string) bool {
	short := "-" + name
	long := "--" + name
	for _, arg := range os.Args[1:] {
		if arg == short || arg == long || strings.HasPrefix(arg, short+"=") || strings.HasPrefix(arg, long+"=") {
			return true
		}
	}
	return false
}

func formatListenerSpecs(specs []builtins.ListenerSpec) string {
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
	commit, err := gitOutput("rev-parse", "HEAD")
	if err != nil {
		return "unknown tracked_dirty=unknown"
	}
	dirty := "false"
	if err := exec.Command("git", "diff", "--quiet", "--").Run(); err != nil {
		dirty = "true"
	}
	return fmt.Sprintf("%s tracked_dirty=%s", commit, dirty)
}

func gitOutput(args ...string) (string, error) {
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func printProfiles(registry profile.Registry) {
	for _, entry := range registry.SortedProfiles() {
		fmt.Printf("%s\t%s\t%s\t%s\t%s\n",
			entry.ProfileID,
			entry.Implementation,
			entry.RuntimeOS,
			entry.DatabaseFixture,
			entry.SupportStatus)
	}
}

// parseObjID parses "#N" or "N" to types.ObjID
func parseObjID(s string) (types.ObjID, error) {
	s = strings.TrimPrefix(s, "#")
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid object ID: %s", s)
	}
	return types.ObjID(id), nil
}

// parseObjVerb parses "#N:verbname" to (objID, verbName)
func parseObjVerb(s string) (types.ObjID, string, error) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return 0, "", fmt.Errorf("invalid format, expected #obj:verb (e.g., #0:do_login_command)")
	}
	objID, err := parseObjID(parts[0])
	if err != nil {
		return 0, "", err
	}
	return objID, parts[1], nil
}

// dumpVerbCode dumps verb source code
func dumpVerbCode(store *dbstore.Store, spec string) {
	objID, verbName, err := parseObjVerb(spec)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	verb, defObjID, err := store.FindVerb(objID, verbName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("=== #%d:%s ===\n", defObjID, verbName)
	fmt.Printf("Names: %s\n", strings.Join(verb.Names, " "))
	fmt.Printf("Owner: #%d\n", verb.Owner)
	fmt.Printf("Perms: %s\n", verb.Perms.String())
	fmt.Printf("--- Code (%d lines) ---\n", len(verb.Code))
	for i, line := range verb.Code {
		fmt.Printf("%4d: %s\n", i+1, line)
	}
}

// dumpListVerbs lists all verbs on an object
func dumpListVerbs(store *dbstore.Store, spec string) {
	objID, err := parseObjID(spec)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	obj, ok := store.Get(objID)
	if !ok {
		fmt.Fprintf(os.Stderr, "Error: object #%d not found\n", objID)
		os.Exit(1)
	}

	fmt.Printf("=== Verbs on #%d (%s) ===\n", objID, obj.Name)
	fmt.Printf("Count: %d\n\n", obj.VerbCount)

	for i := 0; i < obj.VerbCount; i++ {
		view, errCode := store.VerbByIndex(objID, i)
		if errCode != types.E_NONE {
			continue
		}
		fmt.Printf("%3d. %-30s owner=#%-6d perms=%-4s lines=%d\n",
			i, strings.Join(view.Names, " "), view.Owner, view.Perms.String(), len(view.Code))
	}
}

// dumpObjInfo shows detailed object info
func dumpObjInfo(store *dbstore.Store, spec string) {
	objID, err := parseObjID(spec)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	obj, ok := store.Get(objID)
	if !ok {
		fmt.Fprintf(os.Stderr, "Error: object #%d not found\n", objID)
		os.Exit(1)
	}

	fmt.Printf("=== Object #%d ===\n", objID)
	fmt.Printf("Name:     %s\n", obj.Name)
	fmt.Printf("Owner:    #%d\n", obj.Owner)
	fmt.Printf("Location: #%d\n", obj.Location)
	fmt.Printf("Flags:    0x%x", obj.Flags)

	// Decode flags
	var flagNames []string
	if obj.Flags.Has(dbstore.FlagUser) {
		flagNames = append(flagNames, "player")
	}
	if obj.Flags.Has(dbstore.FlagProgrammer) {
		flagNames = append(flagNames, "programmer")
	}
	if obj.Flags.Has(dbstore.FlagWizard) {
		flagNames = append(flagNames, "wizard")
	}
	if obj.Flags.Has(dbstore.FlagRead) {
		flagNames = append(flagNames, "r")
	}
	if obj.Flags.Has(dbstore.FlagWrite) {
		flagNames = append(flagNames, "w")
	}
	if obj.Flags.Has(dbstore.FlagFertile) {
		flagNames = append(flagNames, "f")
	}
	if len(flagNames) > 0 {
		fmt.Printf(" (%s)", strings.Join(flagNames, ", "))
	}
	fmt.Println()

	// Parents
	parents, _ := store.Parents(objID)
	fmt.Printf("Parents:  ")
	if len(parents) == 0 {
		fmt.Println("(none)")
	} else {
		for i, p := range parents {
			if i > 0 {
				fmt.Print(", ")
			}
			fmt.Printf("#%d", p)
		}
		fmt.Println()
	}

	// Children
	children, _ := store.Children(objID)
	fmt.Printf("Children: ")
	if len(children) == 0 {
		fmt.Println("(none)")
	} else {
		for i, c := range children {
			if i > 0 {
				fmt.Print(", ")
			}
			fmt.Printf("#%d", c)
		}
		fmt.Println()
	}

	// Properties
	propNames, _ := store.DefinedPropertyNames(objID)
	fmt.Printf("\n--- Properties (%d) ---\n", len(propNames))
	sort.Strings(propNames)
	for _, name := range propNames {
		prop, ok, _ := store.LocalProperty(objID, name)
		if !ok {
			continue
		}
		valStr := fmt.Sprintf("%v", prop.Value)
		if len(valStr) > 60 {
			valStr = valStr[:57] + "..."
		}
		fmt.Printf("  %-25s = %-60s  owner=#%-6d perms=%s\n",
			name, valStr, prop.Owner, prop.Perms.String())
	}

	// Verbs
	fmt.Printf("\n--- Verbs (%d) ---\n", obj.VerbCount)
	for i := 0; i < obj.VerbCount; i++ {
		view, errCode := store.VerbByIndex(objID, i)
		if errCode != types.E_NONE {
			continue
		}
		fmt.Printf("  %3d. %-30s owner=#%-6d perms=%-4s lines=%d\n",
			i, strings.Join(view.Names, " "), view.Owner, view.Perms.String(), len(view.Code))
	}
}

// evalExpression parses and evaluates a MOO expression
func evalExpression(store *dbstore.Store, expr string, options config.Options) {
	registry := vm.BuildVMRegistry()
	registry.SetTaskManager(task.NewManager())
	prog, diagnostics := compiler.CompileMOO([]string{"return " + expr + ";"}, registry)
	if len(diagnostics) > 0 {
		fmt.Fprintf(os.Stderr, "Compile error: %s\n", diagnostics[0].Error())
		os.Exit(1)
	}

	ctx := kernel.NewTaskContext()
	ctx.Store = store
	ctx.Registry = registry
	ctx.RuntimeOptions = options

	machine := vm.NewVM(store, registry)
	machine.Context = ctx
	result := machine.Run(prog)

	if result.Flow == types.FlowReturn || result.Flow == types.FlowNormal {
		if result.Val.IsNone() {
			result.Val = types.NewInt(0)
		}
		fmt.Printf("=> %s\n", result.Val.String())
	} else {
		fmt.Printf("Error: %s\n", result.Error.String())
	}
}

// dumpObjRawCommand dumps raw database fields for debugging
func dumpObjRawCommand(store *dbstore.Store, spec string) {
	objID, err := parseObjID(spec)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	obj, ok := store.Get(objID)
	if !ok {
		fmt.Fprintf(os.Stderr, "Error: object #%d not found\n", objID)
		os.Exit(1)
	}

	parents, _ := store.Parents(objID)
	children, _ := store.Children(objID)
	contents, _ := store.Contents(objID)

	fmt.Printf("=== Raw Object Data #%d ===\n", objID)
	fmt.Printf("ID:         %d\n", obj.ID)
	fmt.Printf("Name:       %q\n", obj.Name)
	fmt.Printf("Owner:      #%d\n", obj.Owner)
	fmt.Printf("Location:   #%d\n", obj.Location)
	fmt.Printf("Flags:      0x%x (%d)\n", obj.Flags, obj.Flags)
	fmt.Printf("Anonymous:  %v\n", obj.Anonymous)

	fmt.Printf("\nParents:    [")
	for i, p := range parents {
		if i > 0 {
			fmt.Print(", ")
		}
		fmt.Printf("#%d", p)
	}
	fmt.Printf("] (count=%d)\n", len(parents))

	fmt.Printf("Children:   [")
	for i, c := range children {
		if i > 0 {
			fmt.Print(", ")
		}
		fmt.Printf("#%d", c)
	}
	fmt.Printf("] (count=%d)\n", len(children))

	fmt.Printf("Contents:   [")
	for i, c := range contents {
		if i > 0 {
			fmt.Print(", ")
		}
		fmt.Printf("#%d", c)
	}
	fmt.Printf("] (count=%d)\n", len(contents))

	fmt.Printf("\nVerbList:   %d verbs\n", obj.VerbCount)
	for i := 0; i < obj.VerbCount; i++ {
		view, errCode := store.VerbByIndex(objID, i)
		if errCode != types.E_NONE {
			continue
		}
		fmt.Printf("  [%d] %q (names=%d, owner=#%d, code=%d lines)\n",
			i, view.Name, len(view.Names), view.Owner, len(view.Code))
	}

	verbNames, _ := store.VerbNames(objID)
	fmt.Printf("\nVerbs map:  %d entries\n", len(verbNames))

	propNames, _ := store.DefinedPropertyNames(objID)
	fmt.Printf("\nProperties: %d entries\n", len(propNames))
	for _, name := range propNames {
		prop, ok, _ := store.LocalProperty(objID, name)
		if !ok {
			continue
		}
		fmt.Printf("  %q: owner=#%d perms=%s type=%T\n",
			name, prop.Owner, prop.Perms.String(), prop.Value)
	}
}

// verbLookupCommand shows where a verb would be found (which parent)
func verbLookupCommand(store *dbstore.Store, spec string) {
	objID, verbName, err := parseObjVerb(spec)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("=== Verb Lookup: #%d:%s ===\n\n", objID, verbName)

	// Check if object exists
	obj, ok := store.Get(objID)
	if !ok {
		fmt.Fprintf(os.Stderr, "Error: object #%d not found\n", objID)
		os.Exit(1)
	}

	fmt.Printf("Starting object: #%d (%s)\n", objID, obj.Name)

	// Try to find the verb
	verb, defObjID, err := store.FindVerb(objID, verbName)
	if err != nil {
		fmt.Printf("\nResult: NOT FOUND\n")
		fmt.Printf("Error: %v\n", err)

		// Show the search path
		fmt.Printf("\nSearch path:\n")
		current := objID
		visited := make(map[types.ObjID]bool)
		depth := 0
		for {
			if visited[current] {
				fmt.Printf("  [cycle detected at #%d]\n", current)
				break
			}
			visited[current] = true

			currentObj, ok := store.Get(current)
			if !ok {
				fmt.Printf("  #%d (NOT FOUND)\n", current)
				break
			}

			indent := strings.Repeat("  ", depth)
			fmt.Printf("%s#%d (%s) - %d verbs\n", indent, current, currentObj.Name, currentObj.VerbCount)

			currentParents, _ := store.Parents(current)
			if len(currentParents) == 0 {
				break
			}
			current = currentParents[0]
			depth++
		}
		os.Exit(1)
	}

	fmt.Printf("\nResult: FOUND on #%d\n", defObjID)

	if defObjID == objID {
		fmt.Printf("  (defined directly on this object)\n")
	} else {
		fmt.Printf("  (inherited from parent)\n")

		// Show the inheritance chain to the definition
		fmt.Printf("\nInheritance chain:\n")
		current := objID
		visited := make(map[types.ObjID]bool)
		depth := 0
		for current != defObjID {
			if visited[current] {
				fmt.Printf("  [cycle detected]\n")
				break
			}
			visited[current] = true

			currentObj, ok := store.Get(current)
			if !ok {
				fmt.Printf("  #%d (NOT FOUND)\n", current)
				break
			}

			indent := strings.Repeat("  ", depth)
			fmt.Printf("%s#%d (%s)\n", indent, current, currentObj.Name)

			currentParents, _ := store.Parents(current)
			if len(currentParents) == 0 {
				fmt.Printf("  [no parent, but verb is on #%d?]\n", defObjID)
				break
			}
			current = currentParents[0]
			depth++
		}

		// Print the defining object
		defObj, _ := store.Get(defObjID)
		indent := strings.Repeat("  ", depth)
		fmt.Printf("%s#%d (%s) *** VERB DEFINED HERE ***\n", indent, defObjID, defObj.Name)
	}

	fmt.Printf("\nVerb details:\n")
	fmt.Printf("  Name:    %s\n", verb.Name)
	fmt.Printf("  Names:   %s\n", strings.Join(verb.Names, " "))
	fmt.Printf("  Owner:   #%d\n", verb.Owner)
	fmt.Printf("  Perms:   %s\n", verb.Perms.String())
	fmt.Printf("  ArgSpec: %s %s %s\n", verb.ArgSpec.This, verb.ArgSpec.Prep, verb.ArgSpec.That)
	fmt.Printf("  Code:    %d lines\n", len(verb.Code))
}

// ancestryCommand shows the full parent chain
func ancestryCommand(store *dbstore.Store, spec string) {
	objID, err := parseObjID(spec)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	obj, ok := store.Get(objID)
	if !ok {
		fmt.Fprintf(os.Stderr, "Error: object #%d not found\n", objID)
		os.Exit(1)
	}

	fmt.Printf("=== Ancestry for #%d (%s) ===\n\n", objID, obj.Name)

	current := objID
	visited := make(map[types.ObjID]bool)
	depth := 0

	for {
		if visited[current] {
			fmt.Printf("%s[CYCLE DETECTED: #%d already visited]\n", strings.Repeat("  ", depth), current)
			break
		}
		visited[current] = true

		currentObj, ok := store.Get(current)
		if !ok {
			fmt.Printf("%s#%d (NOT FOUND)\n", strings.Repeat("  ", depth), current)
			break
		}

		indent := strings.Repeat("  ", depth)
		fmt.Printf("%s#%d - %s\n", indent, current, currentObj.Name)
		fmt.Printf("%s       owner=#%d, verbs=%d, props=%d\n",
			indent, currentObj.Owner, currentObj.VerbCount, currentObj.PropertyCount)

		currentParents, _ := store.Parents(current)
		if len(currentParents) == 0 {
			fmt.Printf("%s       (root object - no parent)\n", indent)
			break
		}

		if len(currentParents) > 1 {
			fmt.Printf("%s       (multiple parents: ", indent)
			for i, p := range currentParents {
				if i > 0 {
					fmt.Print(", ")
				}
				fmt.Printf("#%d", p)
			}
			fmt.Println(")")
			// For now, just follow the first parent
			fmt.Printf("%s       (following first parent #%d)\n", indent, currentParents[0])
		}

		current = currentParents[0]
		depth++

		// Safety limit
		if depth > 100 {
			fmt.Printf("%s[DEPTH LIMIT REACHED]\n", strings.Repeat("  ", depth))
			break
		}
	}

	fmt.Printf("\nTotal depth: %d\n", depth)
}
