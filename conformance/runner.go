package conformance

import (
	"fmt"

	"barn/builtins"
	dbformat "barn/db/format"
	dbstore "barn/db/store"
	"barn/kernel"
	"barn/parser"
	"barn/server"
	"barn/types"
	"barn/vm"
)

// Default database path (local copy)
const DefaultDBPath = "C:/Users/Q/code/barn/Test.db"

// TestResult represents the outcome of running a single test
type TestResult struct {
	Test       LoadedTest
	Passed     bool
	Skipped    bool
	SkipReason string
	Error      error
}

// Runner executes conformance tests
type Runner struct {
	store       *dbstore.Store
	registry    *builtins.Registry
	setupSuites map[string]bool // Track which suites have had setup run
}

// NewRunner creates a new test runner with the default database
func NewRunner() *Runner {
	return NewRunnerWithDB(DefaultDBPath)
}

// NewRunnerWithDB creates a test runner with a specific database file
func NewRunnerWithDB(dbPath string) *Runner {
	// Load the database
	database, err := dbformat.LoadDatabase(dbPath)
	if err != nil {
		// Fall back to empty store if database can't be loaded
		store := dbstore.NewStore()
		return &Runner{
			store:       store,
			registry:    vm.BuildVMRegistry(),
			setupSuites: make(map[string]bool),
		}
	}

	// Create store from loaded database
	store := database.NewStoreFromDatabase()

	// Apply test-required properties
	setupStoreForTests(store)

	return &Runner{
		store:       store,
		registry:    vm.BuildVMRegistry(),
		setupSuites: make(map[string]bool),
	}
}

// NewRunnerWithServer creates a test runner using a server's store.
func NewRunnerWithServer(srv *server.Server) *Runner {
	// Apply the same store setup that NewRunnerWithDB does
	store := srv.GetStore()
	setupStoreForTests(store)

	return &Runner{
		store:       store,
		registry:    vm.BuildVMRegistry(),
		setupSuites: make(map[string]bool),
	}
}

// Run executes a single test case
func (r *Runner) Run(test LoadedTest) TestResult {
	// Check if test should be skipped
	if skipped, reason := test.Test.IsSkipped(); skipped {
		return TestResult{
			Test:       test,
			Skipped:    true,
			SkipReason: reason,
		}
	}

	// Create task context
	ctx := kernel.NewTaskContext()
	ctx.Store = r.store
	ctx.Registry = r.registry

	// Set up player and programmer for tests
	// Tests expect player to be #1 (matches environment.go default)
	ctx.Player = types.ObjID(1)
	ctx.Programmer = types.ObjID(1)

	// Set permissions based on test's permission field
	if test.Test.Permission == "wizard" {
		ctx.IsWizard = true
	}

	// Run suite setup if not already done
	if test.Suite.Setup != nil && !r.setupSuites[test.File] {
		if err := r.runSetupBlock(test.Suite.Setup, ctx); err != nil {
			return TestResult{
				Test:   test,
				Passed: false,
				Error:  fmt.Errorf("suite setup failed: %w", err),
			}
		}
		r.setupSuites[test.File] = true
	}

	// Run test-specific setup
	if err := r.runSetupBlock(test.Test.Setup, ctx); err != nil {
		return TestResult{
			Test:   test,
			Passed: false,
			Error:  fmt.Errorf("test setup failed: %w", err),
		}
	}

	// Determine what code to execute and how to parse it
	var result types.Result

	if test.Test.Statement != "" {
		// Statement-based test: parse as full program with statements
		p := parser.NewParser(test.Test.Statement)
		stmts, err := p.ParseProgram()
		if err != nil {
			return TestResult{
				Test:   test,
				Passed: false,
				Error:  fmt.Errorf("parse error: %w", err),
			}
		}
		result = r.executeStatements(stmts, ctx)
		// Handle FlowReturn - extract the value
		if result.Flow == types.FlowReturn {
			result = types.Ok(result.Val)
		}
	} else if test.Test.Code != "" {
		// Expression-based test: parse as expression
		p := parser.NewParser(test.Test.Code)
		expr, err := p.ParseExpression(0)
		if err != nil {
			return TestResult{
				Test:   test,
				Passed: false,
				Error:  fmt.Errorf("parse error: %w", err),
			}
		}
		result = r.executeStatements([]parser.Stmt{&parser.ReturnStmt{Value: expr}}, ctx)
		if result.Flow == types.FlowReturn {
			result = types.Ok(result.Val)
		}
	} else {
		// No code to execute
		return TestResult{
			Test:       test,
			Skipped:    true,
			SkipReason: "no code/statement",
		}
	}

	// Check expectation
	passed, err := r.checkExpectation(test.Test, result)
	return TestResult{
		Test:   test,
		Passed: passed,
		Error:  err,
	}
}

func (r *Runner) executeStatements(stmts []parser.Stmt, ctx *kernel.TaskContext) types.Result {
	compiler := vm.NewCompilerWithRegistry(r.registry)
	prog, err := compiler.CompileStatements(stmts)
	if err != nil {
		return types.Result{Flow: types.FlowParseError, Val: types.NewStr(err.Error())}
	}

	machine := vm.NewVM(r.store, r.registry)
	machine.Context = ctx
	result := machine.Run(prog)
	for result.Flow == types.FlowSuspend || result.Flow == types.FlowFork {
		if result.Flow == types.FlowFork && result.ForkInfo != nil && result.ForkInfo.VarName != "" {
			machine.SetForkResult(0)
		}
		result = machine.Resume()
	}
	return result
}

// RunAll executes all loaded tests
func (r *Runner) RunAll(tests []LoadedTest) []TestResult {
	results := make([]TestResult, len(tests))
	for i, test := range tests {
		results[i] = r.Run(test)
	}
	return results
}
