package dbtool

import (
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"sort"
	"strings"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/ir"
	"github.com/MongooseMoo/barn/parser"
	"github.com/MongooseMoo/barn/types"
	"github.com/MongooseMoo/barn/vm"
)

// ToastCheck names every verb in store whose code a Toast cannot run as
// written, and prints a count line last. builtinsPath lists the builtin names
// that Toast registers, one per line (its function_info() names).
//
// Toast loads a verb that calls a builtin it does not register: its loader
// compiles the call as call_function("name", ...) with a warning (parser.y,
// db_io.cc my_warning), the call raises E_INVARG when it runs (execute.cc
// bf_call_function), and set_verb_code refuses the verb's code as it stands.
// A verb whose code does not parse makes Toast refuse the whole database
// (db_file.cc "Unparsable program"); here that is code Barn's parser rejects.
// call_function with a literal name calls that name.
//
// The check fails on a verb that calls a builtin Barn registers and Toast does
// not, or that does not parse. A call to a name neither registers is listed
// but does not fail it: it fails on Barn as on Toast, so going back to Toast
// loses nothing there. (Barn's compiler refuses a direct call to such a name,
// compiler.compileUncached; the loader lowers one read from a database to
// call_function, db/format.LowerUnknownBuiltinCalls.) A call
// through eval, or call_function with a computed name, cannot be checked here
// and is listed as not checkable; it does not fail the check either.
func ToastCheck(out, errOut io.Writer, store *dbstore.Store, builtinsPath string) error {
	known, err := readBuiltinNames(builtinsPath)
	if err != nil {
		fmt.Fprintf(errOut, "Error: %v\n", err)
		return errors.New("inspection failed")
	}
	barn := map[string]struct{}{}
	for _, d := range vm.Descriptors() {
		barn[strings.ToLower(d.Name)] = struct{}{}
	}
	objects := store.All()
	sort.Slice(objects, func(i, j int) bool { return objects[i].ID < objects[j].ID })
	txn := store.DirectTxn()
	var verbs, barnOnly, neither, unparsable, unchecked int
	for _, obj := range objects {
		for i := 0; i < obj.VerbCount; i++ {
			view, errCode := txn.VerbByIndex(obj.ID, i)
			if errCode != types.E_NONE || len(view.Code) == 0 {
				continue
			}
			verbs++
			name := fmt.Sprintf("#%d:%d", obj.ID, i)
			if len(view.Names) != 0 {
				name = fmt.Sprintf("#%d:%s", obj.ID, view.Names[0])
			}
			program, diagnostics := parser.NewParser(strings.Join(view.Code, "\n")).ParseProgramWithDiagnostics(nil)
			if len(diagnostics) != 0 {
				unparsable++
				fmt.Fprintf(out, "%s: line %d: does not parse on Barn: %s\n", name, diagnostics[0].Position.Line, diagnostics[0].Msg)
				continue
			}
			var barnCall, neitherCall, dynamic bool
			for _, call := range builtinCalls(program) {
				callee := strings.ToLower(call.Name)
				if callee == "call_function" && len(call.Args) != 0 {
					if literal, ok := call.Args[0].(*ir.LiteralExpr); ok && literal.Kind == ir.LiteralString {
						callee = strings.ToLower(literal.StringValue)
					} else {
						dynamic = true
						fmt.Fprintf(out, "%s: line %d: not checkable: call_function with a computed name\n", name, call.Pos.Line)
					}
				}
				if callee == "eval" {
					dynamic = true
					fmt.Fprintf(out, "%s: line %d: not checkable: eval\n", name, call.Pos.Line)
				}
				if _, ok := known[callee]; ok {
					continue
				}
				if _, ok := barn[callee]; ok {
					barnCall = true
					fmt.Fprintf(out, "%s: line %d: calls %s, which Barn registers and Toast does not\n", name, call.Pos.Line, callee)
				} else {
					neitherCall = true
					fmt.Fprintf(out, "%s: line %d: calls %s, which neither Toast nor Barn registers\n", name, call.Pos.Line, callee)
				}
			}
			if barnCall {
				barnOnly++
			}
			if neitherCall {
				neither++
			}
			if dynamic {
				unchecked++
			}
		}
	}
	fmt.Fprintf(out, "toast-check: verbs=%d calls_barn_only_builtin=%d unparsable=%d calls_unregistered_builtin=%d not_checkable=%d\n",
		verbs, barnOnly, unparsable, neither, unchecked)
	if barnOnly+unparsable != 0 {
		return fmt.Errorf("%d verbs Toast cannot run as written", barnOnly+unparsable)
	}
	return nil
}

// readBuiltinNames reads one builtin name per line, ignoring blank lines.
// Toast matches builtin names in any letter case (functions.cc
// number_func_by_name uses strcasecmp).
func readBuiltinNames(path string) (map[string]struct{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	known := map[string]struct{}{}
	for _, line := range strings.Split(string(data), "\n") {
		if name := strings.TrimSpace(line); name != "" {
			known[strings.ToLower(name)] = struct{}{}
		}
	}
	if len(known) == 0 {
		return nil, fmt.Errorf("no builtin names in %s", path)
	}
	return known, nil
}

// builtinCalls returns every builtin call in program, in source order.
func builtinCalls(program *ir.Program) []*ir.BuiltinCallExpr {
	var calls []*ir.BuiltinCallExpr
	var walk func(reflect.Value)
	walk = func(v reflect.Value) {
		switch v.Kind() {
		case reflect.Interface, reflect.Pointer:
			if v.IsNil() {
				return
			}
			if call, ok := v.Interface().(*ir.BuiltinCallExpr); ok && v.Kind() == reflect.Pointer {
				calls = append(calls, call)
			}
			walk(v.Elem())
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				if v.Type().Field(i).IsExported() {
					walk(v.Field(i))
				}
			}
		case reflect.Slice:
			for i := 0; i < v.Len(); i++ {
				walk(v.Index(i))
			}
		}
	}
	walk(reflect.ValueOf(program))
	return calls
}
