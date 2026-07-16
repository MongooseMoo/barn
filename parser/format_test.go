package parser_test

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	dbformat "barn/db/format"
	"barn/parser"
	"barn/types"
	"barn/verb"
)

func TestFormatMOOPreservesSemanticIRAcrossParserCorpus(t *testing.T) {
	sources := []string{
		"x = 1; return x;",
		"; return;",
		"return -(2 ^ 3) + 4 * 5;",
		"return a ? b | (c ? d | e);",
		"return {1, @{2, 3}, {4..6}};",
		"return [\"key\" -> value, 2 -> other];",
		"object.name = object.(property);",
		"items[^] = value; items[2..$] = replacement;",
		"{required, ?missing, ?optional = 2, @rest} = values;",
		"if (a) return 1; elseif (b) return 2; else return 3; endif",
		"while named (condition) continue named; endwhile",
		"for value, key in (collection) break value; endfor",
		"for number in [start..finish] continue number; endfor",
		"try value = risky(); except detail (E_TYPE, E_DIV) return detail; finally cleanup(); endtry",
		"fork task_id (delay) notify(player, \"done\"); endfork",
		"return `value / divisor ! E_DIV => 0';",
		"return object:(verb_name)(arg, @args);",
	}

	for _, source := range sources {
		t.Run(source, func(t *testing.T) { assertCanonicalRoundTrip(t, source) })
	}
}

func TestFormatMOOPreservesRepresentativeDatabaseVerbs(t *testing.T) {
	database, err := dbformat.LoadDatabase("../db/format/testdata/toastcore.db")
	if err != nil {
		t.Fatalf("LoadDatabase() error = %v", err)
	}
	store := database.NewStoreFromDatabase()
	objects := store.All()
	sort.Slice(objects, func(i, j int) bool { return objects[i].ID < objects[j].ID })

	checked := 0
	for _, object := range objects {
		for index := 0; index < object.VerbCount && checked < 50; index++ {
			view, errCode := store.VerbByIndex(object.ID, index)
			if errCode != types.E_NONE || len(view.Code) == 0 {
				continue
			}
			source := strings.Join(view.Code, "\n")
			if _, err := parser.NewParser(source).ParseProgram(); err != nil {
				continue
			}
			assertCanonicalRoundTrip(t, source)
			checked++
		}
		if checked == 50 {
			break
		}
	}
	if checked < 50 {
		t.Fatalf("checked %d database verbs, want 50", checked)
	}
}

func assertCanonicalRoundTrip(t *testing.T, source string) {
	t.Helper()
	original, err := parser.NewParser(source).ParseProgram()
	if err != nil {
		t.Fatalf("initial parse error = %v\nsource:\n%s", err, source)
	}
	formatted := strings.Join(parser.FormatMOO(original), "\n")
	reparsed, err := parser.NewParser(formatted).ParseProgram()
	if err != nil {
		t.Fatalf("formatted parse error = %v\nformatted:\n%s", err, formatted)
	}
	if !reflect.DeepEqual(withoutPositions(reflect.ValueOf(original)).Interface(), withoutPositions(reflect.ValueOf(reparsed)).Interface()) {
		t.Fatalf("semantic IR changed\nsource:\n%s\nformatted:\n%s", source, formatted)
	}
	formattedAgain := strings.Join(parser.FormatMOO(reparsed), "\n")
	if formattedAgain != formatted {
		t.Fatalf("formatter is unstable\nfirst:\n%s\nsecond:\n%s", formatted, formattedAgain)
	}
}

var positionType = reflect.TypeOf(verb.Position{})

func withoutPositions(value reflect.Value) reflect.Value {
	if !value.IsValid() {
		return value
	}
	if value.Type() == positionType {
		return reflect.Zero(positionType)
	}
	switch value.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		copy := reflect.New(value.Type()).Elem()
		copy.Set(withoutPositions(value.Elem()).Convert(value.Elem().Type()))
		return copy
	case reflect.Pointer:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		copy := reflect.New(value.Type().Elem())
		copy.Elem().Set(withoutPositions(value.Elem()))
		return copy
	case reflect.Struct:
		copy := reflect.New(value.Type()).Elem()
		for i := 0; i < value.NumField(); i++ {
			copy.Field(i).Set(withoutPositions(value.Field(i)))
		}
		return copy
	case reflect.Slice:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		copy := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		for i := 0; i < value.Len(); i++ {
			copy.Index(i).Set(withoutPositions(value.Index(i)))
		}
		return copy
	default:
		return value
	}
}
