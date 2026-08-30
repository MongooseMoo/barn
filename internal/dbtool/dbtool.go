package dbtool

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/config"
	dbformat "github.com/MongooseMoo/barn/db/format"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
	"github.com/MongooseMoo/barn/vm"
)

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

// DumpVerbCode dumps verb source code
func DumpVerbCode(out, errOut io.Writer, store *dbstore.Store, spec string) error {
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

// DumpListVerbs lists all verbs on an object
func DumpListVerbs(out, errOut io.Writer, store *dbstore.Store, spec string) error {
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

// DumpObjInfo shows detailed object info
func DumpObjInfo(out, errOut io.Writer, store *dbstore.Store, spec string) error {
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

// EvalExpression parses and evaluates a MOO expression
func EvalExpression(out, errOut io.Writer, store *dbstore.Store, expr string, options config.Options) error {
	registry := vm.BuildVMRegistry()
	session := builtins.NewSession(registry, builtins.Host{TaskManager: task.NewManager()})
	prog, diagnostics := registry.Compiler().CompileMOO([]string{"return " + expr + ";"})
	if len(diagnostics) > 0 {
		fmt.Fprintf(errOut, "Compile error: %s\n", diagnostics[0].Error())
		return errors.New("inspection failed")
	}

	ctx := kernel.NewTaskContext()
	ctx.Store = store
	ctx.StoreTxn = store.DirectTxn()
	ctx.RuntimeOptions = options

	machine := vm.NewVM(store, session)
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

// EvalFile evaluates one MOO expression per line of the named file, printing
// one result line ("=> VALUE" or "Error: CODE") per input line, in order.
// Blank lines and lines starting with "#" are echoed as "-- skipped" so line
// counts stay aligned with the input for differential drivers.
func EvalFile(out, errOut io.Writer, store *dbstore.Store, path string, options config.Options) error {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(errOut, "Error: %v\n", err)
		return errors.New("inspection failed")
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	registry := vm.BuildVMRegistry()
	for _, line := range lines {
		expr := strings.TrimSpace(line)
		if expr == "" || strings.HasPrefix(expr, "##") {
			fmt.Fprintln(out, "-- skipped")
			continue
		}
		session := builtins.NewSession(registry, builtins.Host{TaskManager: task.NewManager()})
		prog, diagnostics := registry.Compiler().CompileMOO([]string{"return " + expr + ";"})
		if len(diagnostics) > 0 {
			fmt.Fprintf(out, "Compile error: %s\n", diagnostics[0].Error())
			continue
		}
		ctx := kernel.NewTaskContext()
		ctx.Store = store
		ctx.StoreTxn = store.DirectTxn()
		ctx.RuntimeOptions = options
		// Match Toast's emergency-wizard eval context so permission-gated
		// builtins behave comparably in differential runs.
		ctx.Player = types.ObjID(3)
		ctx.Programmer = types.ObjID(3)
		machine := vm.NewVM(store, session)
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
	}
	return nil
}

// DumpObjRawCommand dumps raw database fields for debugging
func DumpObjRawCommand(out, errOut io.Writer, store *dbstore.Store, spec string) error {
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

// VerbLookupCommand shows where a verb would be found (which parent)
func VerbLookupCommand(out, errOut io.Writer, store *dbstore.Store, spec string) error {
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

// AncestryCommand shows the full parent chain
func AncestryCommand(out, errOut io.Writer, store *dbstore.Store, spec string) error {
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

func DumpDatabase(source, target string) error {
	return dumpDatabase(source, target, dbformat.LoadDatabase)
}

type databaseLoader func(string) (*dbformat.Database, error)

func dumpDatabase(source, target string, load databaseLoader) error {
	database, err := load(source)
	if err != nil {
		return fmt.Errorf("load database: %w", err)
	}
	store, err := database.NewStoreFromDatabase()
	if err != nil {
		return fmt.Errorf("construct store from database: %w", err)
	}
	f, err := os.Create(target)
	if err != nil {
		return fmt.Errorf("create dump file: %w", err)
	}
	if err := dbformat.NewWriter(f, store.Snapshot()).WriteDatabase(); err != nil {
		f.Close()
		return fmt.Errorf("write database: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close dump file: %w", err)
	}

	reloaded, err := load(target)
	if err != nil {
		return fmt.Errorf("reload database: %w", err)
	}
	reloadedStore, err := reloaded.NewStoreFromDatabase()
	if err != nil {
		return fmt.Errorf("construct reloaded store: %w", err)
	}
	return compareRoundTripStores(store, reloadedStore)
}

func compareRoundTripStores(original, reloaded *dbstore.Store) error {
	originalMax := original.DirectTxn().MaxObject()
	reloadedMax := reloaded.DirectTxn().MaxObject()
	mismatches := make([]string, 0)
	if originalMax != reloadedMax {
		mismatches = append(mismatches, fmt.Sprintf("max object #%d != #%d", originalMax, reloadedMax))
	}
	if originalPlayers, reloadedPlayers := len(original.Players()), len(reloaded.Players()); originalPlayers != reloadedPlayers {
		mismatches = append(mismatches, fmt.Sprintf("players %d != %d", originalPlayers, reloadedPlayers))
	}
	if originalObjects, reloadedObjects := len(original.All()), len(reloaded.All()); originalObjects != reloadedObjects {
		mismatches = append(mismatches, fmt.Sprintf("objects %d != %d", originalObjects, reloadedObjects))
	}

	for id := types.ObjID(0); id <= originalMax; id++ {
		originalObject, originalOK := original.GetUnsafe(id)
		reloadedObject, reloadedOK := reloaded.GetUnsafe(id)
		if originalOK != reloadedOK {
			mismatches = append(mismatches, fmt.Sprintf("object #%d existence differs", id))
			continue
		}
		if !originalOK {
			continue
		}
		if originalObject.Name != reloadedObject.Name {
			mismatches = append(mismatches, fmt.Sprintf("object #%d name %q != %q", id, originalObject.Name, reloadedObject.Name))
		}
		if originalObject.Flags != reloadedObject.Flags {
			mismatches = append(mismatches, fmt.Sprintf("object #%d flags %v != %v", id, originalObject.Flags, reloadedObject.Flags))
		}
		if originalObject.Owner != reloadedObject.Owner {
			mismatches = append(mismatches, fmt.Sprintf("object #%d owner #%d != #%d", id, originalObject.Owner, reloadedObject.Owner))
		}
		if originalObject.VerbCount != reloadedObject.VerbCount {
			mismatches = append(mismatches, fmt.Sprintf("object #%d verbs %d != %d", id, originalObject.VerbCount, reloadedObject.VerbCount))
		}
		if originalObject.PropertyCount != reloadedObject.PropertyCount {
			mismatches = append(mismatches, fmt.Sprintf("object #%d properties %d != %d", id, originalObject.PropertyCount, reloadedObject.PropertyCount))
		}
	}

	if len(mismatches) != 0 {
		return fmt.Errorf("round-trip verification failed: %s", strings.Join(mismatches, "; "))
	}
	return nil
}
