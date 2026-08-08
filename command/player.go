package command

import (
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// ParsePlayerCommand parses input exactly as a player's command line and
// resolves direct and indirect object strings against that player's context.
func ParsePlayerCommand(store *dbstore.Store, player types.ObjID, location types.ObjID, input string) *ParsedCommand {
	cmd := ParseCommand(input)
	resolveCommandObjects(store, player, location, cmd)
	return cmd
}

func resolveCommandObjects(store *dbstore.Store, player types.ObjID, location types.ObjID, cmd *ParsedCommand) {
	if cmd == nil {
		return
	}
	if cmd.Dobjstr != "" {
		cmd.Dobj = MatchObject(store, player, location, cmd.Dobjstr)
	}
	if cmd.Iobjstr != "" {
		cmd.Iobj = MatchObject(store, player, location, cmd.Iobjstr)
	}
}
