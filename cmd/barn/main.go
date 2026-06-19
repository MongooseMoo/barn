package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"

	"barn/builtins"
	"barn/bytecode"
	dbformat "barn/db/format"
	dbstore "barn/db/store"
	"barn/kernel"
	"barn/parser"
	"barn/server"
	"barn/trace"
	"barn/types"
	"barn/vm"
)

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
	var listenFlags stringListFlag
	flag.Var(&listenFlags, "listen", "Listener URL; repeatable, e.g. tcp://:7777")

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

	flag.Parse()

	// Handle -dump flag: dump database and exit
	if *dumpPath != "" {
		database, err := dbformat.LoadDatabase(*dbPath)
		if err != nil {
			log.Fatalf("Failed to load database: %v", err)
		}
		store := database.NewStoreFromDatabase()

		f, err := os.Create(*dumpPath)
		if err != nil {
			log.Fatalf("Failed to create dump file: %v", err)
		}

		writer := dbformat.NewWriter(f, store.Snapshot())
		writer.SetPendingFinalizations(database.PendingFinalizations)
		if err := writer.WriteDatabase(); err != nil {
			f.Close()
			log.Fatalf("Failed to write database: %v", err)
		}
		f.Close()

		log.Printf("Database dumped to %s", *dumpPath)
		return
	}

	// Check if any inspection flag is set
	isInspection := *verbCode != "" || *listVerbs != "" || *objInfo != "" || *evalExpr != "" ||
		*dumpObjRaw != "" || *verbLookup != "" || *ancestry != ""

	if isInspection {
		// Load database for inspection
		database, err := dbformat.LoadDatabase(*dbPath)
		if err != nil {
			log.Fatalf("Failed to load database: %v", err)
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
			evalExpression(store, *evalExpr)
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
	log.Printf("Barn MOO Server")
	log.Printf("Database: %s", *dbPath)
	listenerSpecs, err := buildListenerSpecs(*port, listenFlags, flagWasProvided("port"))
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Listeners: %s", formatListenerSpecs(listenerSpecs))

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
		log.Printf("Tracing enabled (filters: %v)", filters)
	} else {
		trace.Init(false, nil, nil)
	}

	srv, err := server.NewServer(*dbPath, listenerSpecs, *checkpointInterval)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	if err := srv.LoadDatabase(); err != nil {
		log.Fatalf("Failed to load database: %v", err)
	}

	log.Printf("Starting server...")
	if err := srv.Start(); err != nil {
		log.Fatalf("Server error: %v", err)
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

	obj := store.Get(objID)
	if obj == nil {
		fmt.Fprintf(os.Stderr, "Error: object #%d not found\n", objID)
		os.Exit(1)
	}

	fmt.Printf("=== Verbs on #%d (%s) ===\n", objID, obj.Name)
	fmt.Printf("Count: %d\n\n", len(obj.VerbList))

	for i, verb := range obj.VerbList {
		fmt.Printf("%3d. %-30s owner=#%-6d perms=%-4s lines=%d\n",
			i, strings.Join(verb.Names, " "), verb.Owner, verb.Perms.String(), len(verb.Code))
	}
}

// dumpObjInfo shows detailed object info
func dumpObjInfo(store *dbstore.Store, spec string) {
	objID, err := parseObjID(spec)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	obj := store.Get(objID)
	if obj == nil {
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
	fmt.Printf("Parents:  ")
	if len(obj.Parents) == 0 {
		fmt.Println("(none)")
	} else {
		for i, p := range obj.Parents {
			if i > 0 {
				fmt.Print(", ")
			}
			fmt.Printf("#%d", p)
		}
		fmt.Println()
	}

	// Children
	fmt.Printf("Children: ")
	if len(obj.Children) == 0 {
		fmt.Println("(none)")
	} else {
		for i, c := range obj.Children {
			if i > 0 {
				fmt.Print(", ")
			}
			fmt.Printf("#%d", c)
		}
		fmt.Println()
	}

	// Properties
	fmt.Printf("\n--- Properties (%d) ---\n", len(obj.Properties))
	propNames := make([]string, 0, len(obj.Properties))
	for name := range obj.Properties {
		propNames = append(propNames, name)
	}
	sort.Strings(propNames)
	for _, name := range propNames {
		prop := obj.Properties[name].View()
		valStr := fmt.Sprintf("%v", prop.Value)
		if len(valStr) > 60 {
			valStr = valStr[:57] + "..."
		}
		fmt.Printf("  %-25s = %-60s  owner=#%-6d perms=%s\n",
			name, valStr, prop.Owner, prop.Perms.String())
	}

	// Verbs
	fmt.Printf("\n--- Verbs (%d) ---\n", len(obj.VerbList))
	for i, verb := range obj.VerbList {
		fmt.Printf("  %3d. %-30s owner=#%-6d perms=%-4s lines=%d\n",
			i, strings.Join(verb.Names, " "), verb.Owner, verb.Perms.String(), len(verb.Code))
	}
}

// evalExpression parses and evaluates a MOO expression
func evalExpression(store *dbstore.Store, expr string) {
	p := parser.NewParser(expr)
	node, err := p.ParseExpression(0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Parse error: %v\n", err)
		os.Exit(1)
	}

	registry := vm.BuildVMRegistry()
	compiler := bytecode.NewCompilerWithRegistry(registry)
	prog, err := compiler.Compile(&parser.ReturnStmt{Value: node})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Compile error: %v\n", err)
		os.Exit(1)
	}

	ctx := kernel.NewTaskContext()
	ctx.Store = store
	ctx.Registry = registry

	machine := vm.NewVM(store, registry)
	machine.Context = ctx
	result := machine.Run(prog)

	if result.Flow == types.FlowReturn || result.Flow == types.FlowNormal {
		if result.Val == nil {
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

	obj := store.Get(objID)
	if obj == nil {
		fmt.Fprintf(os.Stderr, "Error: object #%d not found\n", objID)
		os.Exit(1)
	}

	fmt.Printf("=== Raw Object Data #%d ===\n", objID)
	fmt.Printf("ID:         %d\n", obj.ID)
	fmt.Printf("Name:       %q\n", obj.Name)
	fmt.Printf("Owner:      #%d\n", obj.Owner)
	fmt.Printf("Location:   #%d\n", obj.Location)
	fmt.Printf("Flags:      0x%x (%d)\n", obj.Flags, obj.Flags)
	fmt.Printf("Anonymous:  %v\n", obj.Anonymous)

	fmt.Printf("\nParents:    [")
	for i, p := range obj.Parents {
		if i > 0 {
			fmt.Print(", ")
		}
		fmt.Printf("#%d", p)
	}
	fmt.Printf("] (count=%d)\n", len(obj.Parents))

	fmt.Printf("Children:   [")
	for i, c := range obj.Children {
		if i > 0 {
			fmt.Print(", ")
		}
		fmt.Printf("#%d", c)
	}
	fmt.Printf("] (count=%d)\n", len(obj.Children))

	fmt.Printf("Contents:   [")
	for i, c := range obj.Contents {
		if i > 0 {
			fmt.Print(", ")
		}
		fmt.Printf("#%d", c)
	}
	fmt.Printf("] (count=%d)\n", len(obj.Contents))

	fmt.Printf("\nVerbList:   %d verbs\n", len(obj.VerbList))
	for i, v := range obj.VerbList {
		fmt.Printf("  [%d] %q (names=%d, owner=#%d, code=%d lines)\n",
			i, v.Name, len(v.Names), v.Owner, len(v.Code))
	}

	fmt.Printf("\nVerbs map:  %d entries\n", len(obj.Verbs))

	fmt.Printf("\nProperties: %d entries\n", len(obj.Properties))
	for name, rawProp := range obj.Properties {
		prop := rawProp.View()
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
	obj := store.Get(objID)
	if obj == nil {
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

			currentObj := store.Get(current)
			if currentObj == nil {
				fmt.Printf("  #%d (NOT FOUND)\n", current)
				break
			}

			indent := strings.Repeat("  ", depth)
			fmt.Printf("%s#%d (%s) - %d verbs\n", indent, current, currentObj.Name, len(currentObj.VerbList))

			if len(currentObj.Parents) == 0 {
				break
			}
			current = currentObj.Parents[0]
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

			currentObj := store.Get(current)
			if currentObj == nil {
				fmt.Printf("  #%d (NOT FOUND)\n", current)
				break
			}

			indent := strings.Repeat("  ", depth)
			fmt.Printf("%s#%d (%s)\n", indent, current, currentObj.Name)

			if len(currentObj.Parents) == 0 {
				fmt.Printf("  [no parent, but verb is on #%d?]\n", defObjID)
				break
			}
			current = currentObj.Parents[0]
			depth++
		}

		// Print the defining object
		defObj := store.Get(defObjID)
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

	obj := store.Get(objID)
	if obj == nil {
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

		currentObj := store.Get(current)
		if currentObj == nil {
			fmt.Printf("%s#%d (NOT FOUND)\n", strings.Repeat("  ", depth), current)
			break
		}

		indent := strings.Repeat("  ", depth)
		fmt.Printf("%s#%d - %s\n", indent, current, currentObj.Name)
		fmt.Printf("%s       owner=#%d, verbs=%d, props=%d\n",
			indent, currentObj.Owner, len(currentObj.VerbList), len(currentObj.Properties))

		if len(currentObj.Parents) == 0 {
			fmt.Printf("%s       (root object - no parent)\n", indent)
			break
		}

		if len(currentObj.Parents) > 1 {
			fmt.Printf("%s       (multiple parents: ", indent)
			for i, p := range currentObj.Parents {
				if i > 0 {
					fmt.Print(", ")
				}
				fmt.Printf("#%d", p)
			}
			fmt.Println(")")
			// For now, just follow the first parent
			fmt.Printf("%s       (following first parent #%d)\n", indent, currentObj.Parents[0])
		}

		current = currentObj.Parents[0]
		depth++

		// Safety limit
		if depth > 100 {
			fmt.Printf("%s[DEPTH LIMIT REACHED]\n", strings.Repeat("  ", depth))
			break
		}
	}

	fmt.Printf("\nTotal depth: %d\n", depth)
}
