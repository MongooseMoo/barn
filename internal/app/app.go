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
	"sort"
	"strconv"
	"strings"

	"github.com/MongooseMoo/barn/config"
	dbformat "github.com/MongooseMoo/barn/db/format"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/internal/listener"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/logging"
	"github.com/MongooseMoo/barn/profile"
	"github.com/MongooseMoo/barn/server"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/trace"
	"github.com/MongooseMoo/barn/types"
	"github.com/MongooseMoo/barn/vm"
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
func dumpVerbCode(out, errOut io.Writer, store *dbstore.Store, spec string) error {
	objID, verbName, err := parseObjVerb(spec)
	if err != nil {
		fmt.Fprintf(errOut, "Error: %v\n", err)
		return errors.New("inspection failed")
	}

	verb, defObjID, err := store.DirectTxn().FindVerb(objID, verbName)
	if err != nil {
		fmt.Fprintf(errOut, "Error: %v\n", err)
		return errors.New("inspection failed")
	}

	fmt.Fprintf(out, "=== #%d:%s ===\n", defObjID, verbName)
	fmt.Fprintf(out, "Names: %s\n", strings.Join(verb.Names, " "))
	fmt.Fprintf(out, "Owner: #%d\n", verb.Owner)
	fmt.Fprintf(out, "Perms: %s\n", verb.Perms.String())
	fmt.Fprintf(out, "--- Code (%d lines) ---\n", len(verb.Code))
	for i, line := range verb.Code {
		fmt.Fprintf(out, "%4d: %s\n", i+1, line)
	}
	return nil
}

// dumpListVerbs lists all verbs on an object
func dumpListVerbs(out, errOut io.Writer, store *dbstore.Store, spec string) error {
	objID, err := parseObjID(spec)
	if err != nil {
		fmt.Fprintf(errOut, "Error: %v\n", err)
		return errors.New("inspection failed")
	}

	obj, ok := store.Get(objID)
	if !ok {
		fmt.Fprintf(errOut, "Error: object #%d not found\n", objID)
		return errors.New("inspection failed")
	}

	fmt.Fprintf(out, "=== Verbs on #%d (%s) ===\n", objID, obj.Name)
	fmt.Fprintf(out, "Count: %d\n\n", obj.VerbCount)

	for i := 0; i < obj.VerbCount; i++ {
		view, errCode := store.DirectTxn().VerbByIndex(objID, i)
		if errCode != types.E_NONE {
			continue
		}
		fmt.Fprintf(out, "%3d. %-30s owner=#%-6d perms=%-4s lines=%d\n",
			i, strings.Join(view.Names, " "), view.Owner, view.Perms.String(), len(view.Code))
	}
	return nil
}

// dumpObjInfo shows detailed object info
func dumpObjInfo(out, errOut io.Writer, store *dbstore.Store, spec string) error {
	objID, err := parseObjID(spec)
	if err != nil {
		fmt.Fprintf(errOut, "Error: %v\n", err)
		return errors.New("inspection failed")
	}

	obj, ok := store.Get(objID)
	if !ok {
		fmt.Fprintf(errOut, "Error: object #%d not found\n", objID)
		return errors.New("inspection failed")
	}

	fmt.Fprintf(out, "=== Object #%d ===\n", objID)
	fmt.Fprintf(out, "Name:     %s\n", obj.Name)
	fmt.Fprintf(out, "Owner:    #%d\n", obj.Owner)
	fmt.Fprintf(out, "Location: #%d\n", obj.Location)
	fmt.Fprintf(out, "Flags:    0x%x", obj.Flags)

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
		fmt.Fprintf(out, " (%s)", strings.Join(flagNames, ", "))
	}
	fmt.Fprintln(out)

	// Parents
	parents, _ := store.DirectTxn().Parents(objID)
	fmt.Fprintf(out, "Parents:  ")
	if len(parents) == 0 {
		fmt.Fprintln(out, "(none)")
	} else {
		for i, p := range parents {
			if i > 0 {
				fmt.Fprint(out, ", ")
			}
			fmt.Fprintf(out, "#%d", p)
		}
		fmt.Fprintln(out)
	}

	// Children
	children, _ := store.DirectTxn().Children(objID)
	fmt.Fprintf(out, "Children: ")
	if len(children) == 0 {
		fmt.Fprintln(out, "(none)")
	} else {
		for i, c := range children {
			if i > 0 {
				fmt.Fprint(out, ", ")
			}
			fmt.Fprintf(out, "#%d", c)
		}
		fmt.Fprintln(out)
	}

	// Properties
	propNames, _ := store.DirectTxn().DefinedPropertyNames(objID)
	fmt.Fprintf(out, "\n--- Properties (%d) ---\n", len(propNames))
	sort.Strings(propNames)
	for _, name := range propNames {
		prop, ok, _ := store.DirectTxn().LocalProperty(objID, name)
		if !ok {
			continue
		}
		valStr := fmt.Sprintf("%v", prop.Value)
		if len(valStr) > 60 {
			valStr = valStr[:57] + "..."
		}
		fmt.Fprintf(out, "  %-25s = %-60s  owner=#%-6d perms=%s\n",
			name, valStr, prop.Owner, prop.Perms.String())
	}

	// Verbs
	fmt.Fprintf(out, "\n--- Verbs (%d) ---\n", obj.VerbCount)
	for i := 0; i < obj.VerbCount; i++ {
		view, errCode := store.DirectTxn().VerbByIndex(objID, i)
		if errCode != types.E_NONE {
			continue
		}
		fmt.Fprintf(out, "  %3d. %-30s owner=#%-6d perms=%-4s lines=%d\n",
			i, strings.Join(view.Names, " "), view.Owner, view.Perms.String(), len(view.Code))
	}
	return nil
}

// evalExpression parses and evaluates a MOO expression
func evalExpression(out, errOut io.Writer, store *dbstore.Store, expr string, options config.Options) error {
	registry := vm.BuildVMRegistry()
	registry.SetTaskManager(task.NewManager())
	prog, diagnostics := registry.Compiler().CompileMOO([]string{"return " + expr + ";"})
	if len(diagnostics) > 0 {
		fmt.Fprintf(errOut, "Compile error: %s\n", diagnostics[0].Error())
		return errors.New("inspection failed")
	}

	ctx := kernel.NewTaskContext()
	ctx.Store = store
	ctx.StoreTxn = store.DirectTxn()
	ctx.RuntimeOptions = options

	machine := vm.NewVM(store, registry)
	machine.Context = ctx
	result := machine.Run(prog)

	if result.Flow == types.FlowReturn || result.Flow == types.FlowNormal {
		if result.Val.IsNone() {
			result.Val = types.NewInt(0)
		}
		fmt.Fprintf(out, "=> %s\n", result.Val.String())
	} else {
		fmt.Fprintf(out, "Error: %s\n", result.Error.String())
	}
	return nil
}

// dumpObjRawCommand dumps raw database fields for debugging
func dumpObjRawCommand(out, errOut io.Writer, store *dbstore.Store, spec string) error {
	objID, err := parseObjID(spec)
	if err != nil {
		fmt.Fprintf(errOut, "Error: %v\n", err)
		return errors.New("inspection failed")
	}

	obj, ok := store.Get(objID)
	if !ok {
		fmt.Fprintf(errOut, "Error: object #%d not found\n", objID)
		return errors.New("inspection failed")
	}

	parents, _ := store.DirectTxn().Parents(objID)
	children, _ := store.DirectTxn().Children(objID)
	contents, _ := store.DirectTxn().Contents(objID)

	fmt.Fprintf(out, "=== Raw Object Data #%d ===\n", objID)
	fmt.Fprintf(out, "ID:         %d\n", obj.ID)
	fmt.Fprintf(out, "Name:       %q\n", obj.Name)
	fmt.Fprintf(out, "Owner:      #%d\n", obj.Owner)
	fmt.Fprintf(out, "Location:   #%d\n", obj.Location)
	fmt.Fprintf(out, "Flags:      0x%x (%d)\n", obj.Flags, obj.Flags)
	fmt.Fprintf(out, "Anonymous:  %v\n", obj.Anonymous)

	fmt.Fprintf(out, "\nParents:    [")
	for i, p := range parents {
		if i > 0 {
			fmt.Fprint(out, ", ")
		}
		fmt.Fprintf(out, "#%d", p)
	}
	fmt.Fprintf(out, "] (count=%d)\n", len(parents))

	fmt.Fprintf(out, "Children:   [")
	for i, c := range children {
		if i > 0 {
			fmt.Fprint(out, ", ")
		}
		fmt.Fprintf(out, "#%d", c)
	}
	fmt.Fprintf(out, "] (count=%d)\n", len(children))

	fmt.Fprintf(out, "Contents:   [")
	for i, c := range contents {
		if i > 0 {
			fmt.Fprint(out, ", ")
		}
		fmt.Fprintf(out, "#%d", c)
	}
	fmt.Fprintf(out, "] (count=%d)\n", len(contents))

	fmt.Fprintf(out, "\nVerbList:   %d verbs\n", obj.VerbCount)
	for i := 0; i < obj.VerbCount; i++ {
		view, errCode := store.DirectTxn().VerbByIndex(objID, i)
		if errCode != types.E_NONE {
			continue
		}
		fmt.Fprintf(out, "  [%d] %q (names=%d, owner=#%d, code=%d lines)\n",
			i, view.Name, len(view.Names), view.Owner, len(view.Code))
	}

	verbNames, _ := store.DirectTxn().VerbNames(objID)
	fmt.Fprintf(out, "\nVerbs map:  %d entries\n", len(verbNames))

	propNames, _ := store.DirectTxn().DefinedPropertyNames(objID)
	fmt.Fprintf(out, "\nProperties: %d entries\n", len(propNames))
	for _, name := range propNames {
		prop, ok, _ := store.DirectTxn().LocalProperty(objID, name)
		if !ok {
			continue
		}
		fmt.Fprintf(out, "  %q: owner=#%d perms=%s type=%T\n",
			name, prop.Owner, prop.Perms.String(), prop.Value)
	}
	return nil
}

// verbLookupCommand shows where a verb would be found (which parent)
func verbLookupCommand(out, errOut io.Writer, store *dbstore.Store, spec string) error {
	objID, verbName, err := parseObjVerb(spec)
	if err != nil {
		fmt.Fprintf(errOut, "Error: %v\n", err)
		return errors.New("inspection failed")
	}

	fmt.Fprintf(out, "=== Verb Lookup: #%d:%s ===\n\n", objID, verbName)

	// Check if object exists
	obj, ok := store.Get(objID)
	if !ok {
		fmt.Fprintf(errOut, "Error: object #%d not found\n", objID)
		return errors.New("inspection failed")
	}

	fmt.Fprintf(out, "Starting object: #%d (%s)\n", objID, obj.Name)

	// Try to find the verb
	verb, defObjID, err := store.DirectTxn().FindVerb(objID, verbName)
	if err != nil {
		fmt.Fprintf(out, "\nResult: NOT FOUND\n")
		fmt.Fprintf(out, "Error: %v\n", err)

		// Show the search path
		fmt.Fprintf(out, "\nSearch path:\n")
		current := objID
		visited := make(map[types.ObjID]bool)
		depth := 0
		for {
			if visited[current] {
				fmt.Fprintf(out, "  [cycle detected at #%d]\n", current)
				break
			}
			visited[current] = true

			currentObj, ok := store.Get(current)
			if !ok {
				fmt.Fprintf(out, "  #%d (NOT FOUND)\n", current)
				break
			}

			indent := strings.Repeat("  ", depth)
			fmt.Fprintf(out, "%s#%d (%s) - %d verbs\n", indent, current, currentObj.Name, currentObj.VerbCount)

			currentParents, _ := store.DirectTxn().Parents(current)
			if len(currentParents) == 0 {
				break
			}
			current = currentParents[0]
			depth++
		}
		return errors.New("inspection failed")
	}

	fmt.Fprintf(out, "\nResult: FOUND on #%d\n", defObjID)

	if defObjID == objID {
		fmt.Fprintf(out, "  (defined directly on this object)\n")
	} else {
		fmt.Fprintf(out, "  (inherited from parent)\n")

		// Show the inheritance chain to the definition
		fmt.Fprintf(out, "\nInheritance chain:\n")
		current := objID
		visited := make(map[types.ObjID]bool)
		depth := 0
		for current != defObjID {
			if visited[current] {
				fmt.Fprintf(out, "  [cycle detected]\n")
				break
			}
			visited[current] = true

			currentObj, ok := store.Get(current)
			if !ok {
				fmt.Fprintf(out, "  #%d (NOT FOUND)\n", current)
				break
			}

			indent := strings.Repeat("  ", depth)
			fmt.Fprintf(out, "%s#%d (%s)\n", indent, current, currentObj.Name)

			currentParents, _ := store.DirectTxn().Parents(current)
			if len(currentParents) == 0 {
				fmt.Fprintf(out, "  [no parent, but verb is on #%d?]\n", defObjID)
				break
			}
			current = currentParents[0]
			depth++
		}

		// Print the defining object
		defObj, _ := store.Get(defObjID)
		indent := strings.Repeat("  ", depth)
		fmt.Fprintf(out, "%s#%d (%s) *** VERB DEFINED HERE ***\n", indent, defObjID, defObj.Name)
	}

	fmt.Fprintf(out, "\nVerb details:\n")
	fmt.Fprintf(out, "  Name:    %s\n", verb.Name)
	fmt.Fprintf(out, "  Names:   %s\n", strings.Join(verb.Names, " "))
	fmt.Fprintf(out, "  Owner:   #%d\n", verb.Owner)
	fmt.Fprintf(out, "  Perms:   %s\n", verb.Perms.String())
	fmt.Fprintf(out, "  ArgSpec: %s %s %s\n", verb.ArgSpec.This, verb.ArgSpec.Prep, verb.ArgSpec.That)
	fmt.Fprintf(out, "  Code:    %d lines\n", len(verb.Code))
	return nil
}

// ancestryCommand shows the full parent chain
func ancestryCommand(out, errOut io.Writer, store *dbstore.Store, spec string) error {
	objID, err := parseObjID(spec)
	if err != nil {
		fmt.Fprintf(errOut, "Error: %v\n", err)
		return errors.New("inspection failed")
	}

	obj, ok := store.Get(objID)
	if !ok {
		fmt.Fprintf(errOut, "Error: object #%d not found\n", objID)
		return errors.New("inspection failed")
	}

	fmt.Fprintf(out, "=== Ancestry for #%d (%s) ===\n\n", objID, obj.Name)

	current := objID
	visited := make(map[types.ObjID]bool)
	depth := 0

	for {
		if visited[current] {
			fmt.Fprintf(out, "%s[CYCLE DETECTED: #%d already visited]\n", strings.Repeat("  ", depth), current)
			break
		}
		visited[current] = true

		currentObj, ok := store.Get(current)
		if !ok {
			fmt.Fprintf(out, "%s#%d (NOT FOUND)\n", strings.Repeat("  ", depth), current)
			break
		}

		indent := strings.Repeat("  ", depth)
		fmt.Fprintf(out, "%s#%d - %s\n", indent, current, currentObj.Name)
		fmt.Fprintf(out, "%s       owner=#%d, verbs=%d, props=%d\n",
			indent, currentObj.Owner, currentObj.VerbCount, currentObj.PropertyCount)

		currentParents, _ := store.DirectTxn().Parents(current)
		if len(currentParents) == 0 {
			fmt.Fprintf(out, "%s       (root object - no parent)\n", indent)
			break
		}

		if len(currentParents) > 1 {
			fmt.Fprintf(out, "%s       (multiple parents: ", indent)
			for i, p := range currentParents {
				if i > 0 {
					fmt.Fprint(out, ", ")
				}
				fmt.Fprintf(out, "#%d", p)
			}
			fmt.Fprintln(out, ")")
			// For now, just follow the first parent
			fmt.Fprintf(out, "%s       (following first parent #%d)\n", indent, currentParents[0])
		}

		current = currentParents[0]
		depth++

		// Safety limit
		if depth > 100 {
			fmt.Fprintf(out, "%s[DEPTH LIMIT REACHED]\n", strings.Repeat("  ", depth))
			break
		}
	}

	fmt.Fprintf(out, "\nTotal depth: %d\n", depth)
	return nil
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
	DumpPath                                                                   string
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
		return dumpDatabase(cfg.DatabasePath, cfg.DumpPath)
	}
	if cfg.VerbCode != "" || cfg.ListVerbs != "" || cfg.ObjectInfo != "" || cfg.Eval != "" || cfg.DumpObjectRaw != "" || cfg.VerbLookup != "" || cfg.Ancestry != "" {
		database, err := dbformat.LoadDatabase(cfg.DatabasePath)
		if err != nil {
			return fmt.Errorf("load database: %w", err)
		}
		store, err := database.NewStoreFromDatabase()
		if err != nil {
			return fmt.Errorf("construct store from database: %w", err)
		}
		// Inspection helpers retain the established textual contracts.
		if cfg.VerbCode != "" {
			err := dumpVerbCode(out, errOut, store, cfg.VerbCode)
			if err != nil {
				return err
			}
		}
		if cfg.ListVerbs != "" {
			err := dumpListVerbs(out, errOut, store, cfg.ListVerbs)
			if err != nil {
				return err
			}
		}
		if cfg.ObjectInfo != "" {
			err := dumpObjInfo(out, errOut, store, cfg.ObjectInfo)
			if err != nil {
				return err
			}
		}
		if cfg.Eval != "" {
			err := evalExpression(out, errOut, store, cfg.Eval, options)
			if err != nil {
				return err
			}
		}
		if cfg.DumpObjectRaw != "" {
			err := dumpObjRawCommand(out, errOut, store, cfg.DumpObjectRaw)
			if err != nil {
				return err
			}
		}
		if cfg.VerbLookup != "" {
			err := verbLookupCommand(out, errOut, store, cfg.VerbLookup)
			if err != nil {
				return err
			}
		}
		if cfg.Ancestry != "" {
			err := ancestryCommand(out, errOut, store, cfg.Ancestry)
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
	if cfg.DebugAddr != "off" && cfg.DebugAddr != "" {
		debugSrv, err := startHTTPEndpoint("debug", cfg.DebugAddr, debugMux())
		if err != nil {
			slog.Warn("debug endpoint unavailable", slog.Any("err", err))
		} else {
			defer debugSrv.shutdown()
		}
	}
	state := newLifecycle()
	if cfg.OperatorAddr != "off" && cfg.OperatorAddr != "" {
		operatorSrv, err := startHTTPEndpoint("operator", cfg.OperatorAddr, operatorMux(state))
		if err != nil {
			slog.Warn("operator endpoint unavailable", slog.Any("err", err))
		} else {
			defer operatorSrv.shutdown()
		}
	}
	srv, err := server.NewServerWithOptions(cfg.DatabasePath, specs, cfg.CheckpointInterval, options)
	if err != nil {
		state.Failed()
		return fmt.Errorf("create server: %w", err)
	}
	srv.SetLifecycleObserver(state)
	if cfg.ProfileManifest != "" {
		manifest, err := profile.BuildManifest(profile.BuildInput{ProfileID: cfg.ProfileID, ImplementationRef: gitImplementationRef(), DatabasePath: cfg.DatabasePath, ConfigPath: cfg.ConfigPath, Options: options})
		if err != nil {
			return fmt.Errorf("build profile manifest: %w", err)
		}
		if err := profile.WriteManifest(cfg.ProfileManifest, manifest); err != nil {
			return fmt.Errorf("write profile manifest: %w", err)
		}
	}
	if err := srv.LoadDatabase(); err != nil {
		state.Failed()
		return fmt.Errorf("load database: %w", err)
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

func dumpDatabase(source, target string) error {
	database, err := dbformat.LoadDatabase(source)
	if err != nil {
		return fmt.Errorf("load database: %w", err)
	}
	f, err := os.Create(target)
	if err != nil {
		return fmt.Errorf("create dump file: %w", err)
	}
	defer f.Close()
	store, err := database.NewStoreFromDatabase()
	if err != nil {
		return fmt.Errorf("construct store from database: %w", err)
	}
	if err := dbformat.NewWriter(f, store.Snapshot()).WriteDatabase(); err != nil {
		return fmt.Errorf("write database: %w", err)
	}
	return nil
}
