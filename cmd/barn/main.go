package main

import (
	"barn/db"
	"barn/parser"
	"barn/server"
	"barn/trace"
	"barn/types"
	"barn/vm"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
)

func main() {
	dbPath := flag.String("db", "Test.db", "Database file path")
	port := flag.Int("port", 7777, "Listen port")

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

	flag.Parse()

	// Check if any inspection flag is set
	isInspection := *verbCode != "" || *listVerbs != "" || *objInfo != "" || *evalExpr != "" ||
		*dumpObjRaw != "" || *verbLookup != "" || *ancestry != ""

	if isInspection {
		// Load database for inspection
		database, err := db.LoadDatabase(*dbPath)
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
	log.Printf("Port: %d", *port)

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

	srv, err := server.NewServer(*dbPath, *port)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	if err := srv.LoadDatabase(); err != nil {
		log.Fatalf("Failed to load database: %v", err)
	}

	log.Printf("Starting server on port %d...", *port)
	if err := srv.Start(); err != nil {
		log.Fatalf("Server error: %v", err)
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
func dumpVerbCode(store *db.Store, spec string) {
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
func dumpListVerbs(store *db.Store, spec string) {
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
func dumpObjInfo(store *db.Store, spec string) {
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
	if obj.Flags.Has(db.FlagUser) {
		flagNames = append(flagNames, "player")
	}
	if obj.Flags.Has(db.FlagProgrammer) {
		flagNames = append(flagNames, "programmer")
	}
	if obj.Flags.Has(db.FlagWizard) {
		flagNames = append(flagNames, "wizard")
	}
	if obj.Flags.Has(db.FlagRead) {
		flagNames = append(flagNames, "r")
	}
	if obj.Flags.Has(db.FlagWrite) {
		flagNames = append(flagNames, "w")
	}
	if obj.Flags.Has(db.FlagFertile) {
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
		prop := obj.Properties[name]
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
func evalExpression(store *db.Store, expr string) {
	// Parse the expression
	p := parser.NewParser(expr)
	node, err := p.ParseExpression(0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Parse error: %v\n", err)
		os.Exit(1)
	}

	// Create an evaluator with the store
	evaluator := vm.NewEvaluatorWithStore(store)

	// Create a task context
	ctx := types.NewTaskContext()

	// Evaluate the expression
	result := evaluator.Eval(node, ctx)

	// Print the result
	if result.IsNormal() {
		// Success - print the value in MOO literal format
		fmt.Printf("=> %s\n", result.Val.String())
	} else {
		// Error - print the error code
		fmt.Printf("Error: %s\n", result.Error.String())
	}
}

// dumpObjRawCommand dumps raw database fields for debugging
func dumpObjRawCommand(store *db.Store, spec string) {
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
	for name, prop := range obj.Properties {
		fmt.Printf("  %q: owner=#%d perms=%s type=%T\n",
			name, prop.Owner, prop.Perms.String(), prop.Value)
	}
}

// verbLookupCommand shows where a verb would be found (which parent)
func verbLookupCommand(store *db.Store, spec string) {
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
func ancestryCommand(store *db.Store, spec string) {
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
