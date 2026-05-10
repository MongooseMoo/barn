package main

import (
	"barn/builtins"
	"barn/db"
	"barn/types"
	"barn/vm"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func main() {
	var dbPath string
	var objectID int64
	var verbName string
	var fromLine int
	var toLine int
	var compile bool

	flag.StringVar(&dbPath, "db", "", "MOO database path")
	flag.Int64Var(&objectID, "object", 0, "object id")
	flag.StringVar(&verbName, "verb", "", "verb name")
	flag.IntVar(&fromLine, "from", 1, "first source line to print")
	flag.IntVar(&toLine, "to", 0, "last source line to print; 0 prints all")
	flag.BoolVar(&compile, "compile", false, "parse and bytecode-compile the verb")
	flag.Parse()

	if dbPath == "" || objectID == 0 || verbName == "" {
		fmt.Fprintln(os.Stderr, "usage: mongoose_inspect_load -db path -object 9501 -verb cluster_by_proximity [-from 17 -to 32] [-compile]")
		os.Exit(2)
	}

	loaded, err := db.LoadDatabase(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load database: %v\n", err)
		os.Exit(1)
	}

	obj := loaded.Objects[types.ObjID(objectID)]
	if obj == nil {
		fmt.Fprintf(os.Stderr, "object #%d not found\n", objectID)
		os.Exit(1)
	}

	verb, index := findVerb(obj, verbName)
	if verb == nil {
		fmt.Fprintf(os.Stderr, "verb %q not found on #%d\n", verbName, objectID)
		os.Exit(1)
	}

	fmt.Printf("#%d:%d %q code_lines=%d owner=#%d perms=%s args={%s,%s,%s}\n",
		objectID, index, verb.Name, len(verb.Code), verb.Owner, verb.Perms.String(),
		verb.ArgSpec.This, verb.ArgSpec.Prep, verb.ArgSpec.That)

	if toLine == 0 || toLine > len(verb.Code) {
		toLine = len(verb.Code)
	}
	if fromLine < 1 {
		fromLine = 1
	}
	for line := fromLine; line <= toLine; line++ {
		fmt.Printf("%s %q\n", strconv.Itoa(line), verb.Code[line-1])
	}

	if !compile {
		return
	}

	program, errors := db.CompileVerb(verb.Code)
	fmt.Printf("parse_errors=%v statements=%d\n", errors, len(program.Statements))
	if len(errors) > 0 {
		return
	}

	verb.Program = program
	bytecode, err := vm.CompileVerbBytecode(verb, builtins.NewRegistry())
	if err != nil {
		fmt.Printf("bytecode_error=%v\n", err)
		return
	}
	fmt.Printf("bytecode_bytes=%d constants=%d locals=%d vars=%s\n",
		len(bytecode.Code), len(bytecode.Constants), bytecode.NumLocals, strings.Join(bytecode.VarNames, ","))
}

func findVerb(obj *db.Object, name string) (*db.Verb, int) {
	search := strings.ToLower(name)
	for i, verb := range obj.VerbList {
		for _, alias := range verb.Names {
			if strings.ToLower(alias) == search {
				return verb, i
			}
		}
	}
	return nil, -1
}
