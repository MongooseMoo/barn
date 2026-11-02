package command_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/command"
	dbformat "github.com/MongooseMoo/barn/db/format"
	"github.com/MongooseMoo/barn/types"
)

// FuzzCommandLine feeds arbitrary input lines through the paths every
// connection reaches: word splitting (also used before login), command
// parsing with object matching, and verb lookup. Test.db player #3 stands in
// room #2. Matching is read-only, so one store serves every input.
func FuzzCommandLine(f *testing.F) {
	database, err := dbformat.LoadDatabaseReadOnly(filepath.Join("..", "Test.db"))
	if err != nil {
		f.Fatal(err)
	}
	store, err := database.NewStoreFromDatabase()
	if err != nil {
		f.Fatal(err)
	}
	const player, room = types.ObjID(3), types.ObjID(2)
	for _, line := range []string{
		"", " ", "look", "look at me", "l here", "put key in box", "get all from here",
		"give #3 to me", "examine #-1", "examine #99999999999999999999", "x $thing",
		"\"hello", ":waves", ";1 + 2", "say \"quoted \\\"word\\\"\" end", "l\\ook",
		"\"", "\\", "a \"b", "co-ordinate with the thing", "drop it on top of the table",
		"PREFIX yes", ".program #3:foo", "@create $thing named foo", "\xff\xfe\x00",
		"x " + strings.Repeat("a ", 1000), strings.Repeat("\"", 257),
	} {
		f.Add(line)
	}
	f.Fuzz(func(t *testing.T, line string) {
		command.CommandWordList(line)
		cmd := command.ParsePlayerCommand(store, player, room, line)
		command.HasVerbNameMatch(store, player, room, cmd)
		command.FindVerb(store, player, room, cmd)
	})
}
