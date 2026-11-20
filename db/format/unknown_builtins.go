package format

import (
	"log/slog"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/MongooseMoo/barn/compiler"
	"github.com/MongooseMoo/barn/types"
)

// LowerUnknownBuiltinCalls compiles the loaded verbs the way Toast's loader does
// (db_io.cc dbio_read_program, whose parser client has a warning callback): a
// call to a builtin the runtime lacks is rewritten to call_function("name",
// @args) and logged as a parser warning, instead of leaving a verb that cannot
// compile. set_verb_code still rejects such a call. c must be the compiler of
// the registry the runtime will use, and the call must precede
// NewStoreFromDatabase.
func (database *Database) LowerUnknownBuiltinCalls(c *compiler.Compiler) {
	type program struct {
		object   types.ObjID
		index    int
		source   []string
		lowered  []string
		warnings []compiler.Diagnostic
	}
	ids := make([]types.ObjID, 0, len(database.Objects))
	for id := range database.Objects {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var programs []*program
	for _, id := range ids {
		builder := database.Objects[id]
		for index := 0; index < builder.VerbCount(); index++ {
			if source, ok := builder.VerbCodeByIndex(index); ok && len(source) > 0 {
				programs = append(programs, &program{object: id, index: index, source: source})
			}
		}
	}

	// Every verb is parsed once; 24k Mongoose verbs take about 1.6 s on one core.
	work := make(chan *program, 256)
	var wg sync.WaitGroup
	for range runtime.GOMAXPROCS(0) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range work {
				p.lowered, p.warnings = c.LowerUnknownBuiltinCalls(p.source)
			}
		}()
	}
	for _, p := range programs {
		work <- p
	}
	close(work)
	wg.Wait()

	for _, p := range programs {
		if p.lowered == nil {
			continue
		}
		builder := database.Objects[p.object]
		builder.SetVerbCodeByIndex(p.index, p.lowered)
		// Toast names the program by definer and verb names (db_file.cc
		// fmt_verb_name) and logs each warning on its own line.
		name := types.NewObj(p.object).String() + ":" + strings.Join(builder.VerbNamesAt(p.index), " ")
		slog.Info("PARSER: Warning in "+name+":", slog.String("src", "startup_parser"))
		for _, warning := range p.warnings {
			slog.Info("           "+warning.Error(), slog.String("src", "startup_parser"))
		}
	}
}
