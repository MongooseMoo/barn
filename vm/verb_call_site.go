package vm

import (
	"sync/atomic"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/compiler"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/sourcekey"
)

// verbCallSite is what a static OP_CALL_VERB keeps in its program's call-site
// slot for the verb name it calls. Programs are shared by every task running
// them, so it is made once per slot and never changed; the program it last
// compiled is swapped atomically as a whole.
//
// The verb lookup is the store's (dbstore.VerbCallSite): each transaction
// keeps its own record per site, so nothing per-transaction lives here. The
// program is reused only for the same compiler and the same content key as the
// verb being called, which is the compiler cache's own identity for it, so a
// code edit (committed, or staged by this very transaction) misses and
// compiles the new source.
type verbCallSite struct {
	lookup  *dbstore.VerbCallSite
	program atomic.Pointer[siteProgram]
}

type siteProgram struct {
	compiler *compiler.Compiler
	key      sourcekey.Key
	program  *bytecode.Program
}

// callSiteAt returns the call site for program's constant index, making it
// on first use, or nil when the program has no call-site table.
func callSiteAt(program *bytecode.Program, index int) *verbCallSite {
	slot := program.CallSites.Slot(index)
	if slot == nil {
		return nil
	}
	if site, ok := slot.Load().(*verbCallSite); ok {
		return site
	}
	fresh := &verbCallSite{lookup: dbstore.NewVerbCallSite()}
	if slot.CompareAndSwap(nil, fresh) {
		return fresh
	}
	return slot.Load().(*verbCallSite)
}

// compile returns the program for code with content key key, from this site
// when it compiled the same source with the same compiler last, otherwise from
// the compiler, which this site then remembers.
func (site *verbCallSite) compile(c *compiler.Compiler, code []string, key sourcekey.Key) (*bytecode.Program, []compiler.Diagnostic) {
	if key.IsSet() {
		if last := site.program.Load(); last != nil && last.compiler == c && last.key == key {
			return last.program, nil
		}
	}
	program, diagnostics := c.CompileMOOWithKey(code, key)
	if len(diagnostics) == 0 && key.IsSet() {
		site.program.Store(&siteProgram{compiler: c, key: key, program: program})
	}
	return program, diagnostics
}
