package conformance

import (
	"barn/db"
	"barn/eval"
	"barn/parser"
	"barn/types"
	"fmt"
	"strings"
)

// Default database path (local copy)
const DefaultDBPath = "C:/Users/Q/code/barn/toastcore.db"

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
	evaluator    *eval.Evaluator
	setupSuites  map[string]bool // Track which suites have had setup run
}

// NewRunner creates a new test runner with the default database
func NewRunner() *Runner {
	return NewRunnerWithDB(DefaultDBPath)
}

// NewRunnerWithDB creates a test runner with a specific database file
func NewRunnerWithDB(dbPath string) *Runner {
	// Load the database
	database, err := db.LoadDatabase(dbPath)
	if err != nil {
		// Fall back to empty store if database can't be loaded
		return &Runner{
			evaluator:   eval.NewEvaluator(),
			setupSuites: make(map[string]bool),
		}
	}

	// Create store from loaded database
	store := database.NewStoreFromDatabase()

	// Ensure $object exists (alias for $nothing or first available object)
	// Many tests expect $object to be defined
	if obj := store.Get(0); obj != nil {
		if _, ok := obj.Properties["object"]; !ok {
			// Create $object as alias for $nothing (which is #-1)
			obj.Properties["object"] = &db.Property{
				Name:  "object",
				Value: types.NewObj(-1),
				Owner: 0,
				Perms: db.PropRead,
			}
		}
	}

	return &Runner{
		evaluator:   eval.NewEvaluatorWithStore(store),
		setupSuites: make(map[string]bool),
	}
}

// runSetupBlock executes a setup or teardown block
func (r *Runner) runSetupBlock(block *SetupBlock, ctx *types.TaskContext) error {
	if block == nil {
		return nil
	}

	// Save original wizard state and apply setup's permissions
	origWizard := ctx.IsWizard
	if block.Permission == "wizard" {
		ctx.IsWizard = true
	}
	defer func() { ctx.IsWizard = origWizard }()

	var code string
	if block.Statement != "" {
		code = block.Statement
	} else if block.Code != "" {
		code = block.Code
	} else {
		return nil
	}

	p := parser.NewParser(code)
	stmts, err := p.ParseProgram()
	if err != nil {
		return fmt.Errorf("setup parse error: %w", err)
	}

	result := r.evaluator.EvalStatements(stmts, ctx)
	if result.Flow == types.FlowException {
		return fmt.Errorf("setup error: %s", errorCodeToName(result.Error))
	}

	return nil
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
	ctx := types.NewTaskContext()

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
		result = r.evaluator.EvalStatements(stmts, ctx)
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
		result = r.evaluator.Eval(expr, ctx)
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

// RunAll executes all loaded tests
func (r *Runner) RunAll(tests []LoadedTest) []TestResult {
	results := make([]TestResult, len(tests))
	for i, test := range tests {
		results[i] = r.Run(test)
	}
	return results
}

// SummaryStats computes statistics from test results
type SummaryStats struct {
	Total   int
	Passed  int
	Failed  int
	Skipped int
}

// ComputeStats generates statistics from test results
func ComputeStats(results []TestResult) SummaryStats {
	stats := SummaryStats{Total: len(results)}
	for _, r := range results {
		if r.Skipped {
			stats.Skipped++
		} else if r.Passed {
			stats.Passed++
		} else {
			stats.Failed++
		}
	}
	return stats
}

// FormatStats returns a human-readable summary
func FormatStats(stats SummaryStats) string {
	return fmt.Sprintf("%d passed, %d failed, %d skipped (%d total)",
		stats.Passed, stats.Failed, stats.Skipped, stats.Total)
}

// checkExpectation checks if the result matches the expected outcome
func (r *Runner) checkExpectation(test TestCase, result types.Result) (bool, error) {
	expect := test.Expect

	// Check for expected error
	if expect.Error != "" {
		// Convert error name to ErrorCode
		expectedErr, ok := errorNameToCode(expect.Error)
		if !ok {
			return false, fmt.Errorf("unknown error code: %s", expect.Error)
		}

		if result.Flow != types.FlowException {
			return false, fmt.Errorf("expected error %s, got value: %v", expect.Error, result.Val)
		}

		if result.Error != expectedErr {
			return false, fmt.Errorf("expected error %s, got %s", expect.Error, errorCodeToName(result.Error))
		}

		return true, nil
	}

	// Check for normal result
	if result.Flow == types.FlowException {
		return false, fmt.Errorf("unexpected error: %s", errorCodeToName(result.Error))
	}

	// Check expected value
	if expect.Value != nil {
		expectedVal, err := convertYAMLValue(expect.Value)
		if err != nil {
			return false, fmt.Errorf("failed to convert expected value: %w", err)
		}

		// Handle nil result value
		if result.Val == nil {
			return false, fmt.Errorf("expected %v, got nil", expectedVal)
		}

		if !result.Val.Equal(expectedVal) {
			return false, fmt.Errorf("expected %v, got %v", expectedVal, result.Val)
		}

		return true, nil
	}

	// Check expected type
	if expect.Type != "" {
		expectedType, ok := typeNameToCode(expect.Type)
		if !ok {
			return false, fmt.Errorf("unknown type: %s", expect.Type)
		}

		if result.Val.Type() != expectedType {
			return false, fmt.Errorf("expected type %s, got %s", expect.Type, typeCodeToName(result.Val.Type()))
		}

		return true, nil
	}

	// No expectation specified
	return false, fmt.Errorf("no expectation specified")
}

// convertYAMLValue converts a YAML value to a MOO Value
func convertYAMLValue(v interface{}) (types.Value, error) {
	switch val := v.(type) {
	case int:
		return types.NewInt(int64(val)), nil
	case int64:
		return types.NewInt(val), nil
	case float64:
		return types.NewFloat(val), nil
	case string:
		// Check if string represents an object reference like "#2" or "#-1"
		if len(val) > 0 && val[0] == '#' {
			var id int64
			if _, err := fmt.Sscanf(val, "#%d", &id); err == nil {
				return types.NewObj(types.ObjID(id)), nil
			}
		}
		return types.NewStr(val), nil
	case bool:
		return types.NewBool(val), nil
	case []interface{}:
		elements := make([]types.Value, len(val))
		for i, elem := range val {
			v, err := convertYAMLValue(elem)
			if err != nil {
				return nil, err
			}
			elements[i] = v
		}
		return types.NewList(elements), nil
	case map[string]interface{}:
		// Convert string-keyed map to MOO map
		pairs := make([][2]types.Value, 0, len(val))
		for k, v := range val {
			keyVal := types.NewStr(k)
			valVal, err := convertYAMLValue(v)
			if err != nil {
				return nil, err
			}
			pairs = append(pairs, [2]types.Value{keyVal, valVal})
		}
		return types.NewMap(pairs), nil
	case map[interface{}]interface{}:
		// Handle YAML's default map type (interface{} keys)
		pairs := make([][2]types.Value, 0, len(val))
		for k, v := range val {
			keyVal, err := convertYAMLValue(k)
			if err != nil {
				return nil, err
			}
			valVal, err := convertYAMLValue(v)
			if err != nil {
				return nil, err
			}
			pairs = append(pairs, [2]types.Value{keyVal, valVal})
		}
		return types.NewMap(pairs), nil
	default:
		return nil, fmt.Errorf("unsupported YAML type: %T", v)
	}
}

// errorNameToCode converts error name to ErrorCode
func errorNameToCode(name string) (types.ErrorCode, bool) {
	switch strings.ToUpper(name) {
	case "E_NONE":
		return types.E_NONE, true
	case "E_TYPE":
		return types.E_TYPE, true
	case "E_DIV":
		return types.E_DIV, true
	case "E_PERM":
		return types.E_PERM, true
	case "E_PROPNF":
		return types.E_PROPNF, true
	case "E_VERBNF":
		return types.E_VERBNF, true
	case "E_VARNF":
		return types.E_VARNF, true
	case "E_INVIND":
		return types.E_INVIND, true
	case "E_RECMOVE":
		return types.E_RECMOVE, true
	case "E_MAXREC":
		return types.E_MAXREC, true
	case "E_RANGE":
		return types.E_RANGE, true
	case "E_ARGS":
		return types.E_ARGS, true
	case "E_NACC":
		return types.E_NACC, true
	case "E_INVARG":
		return types.E_INVARG, true
	case "E_QUOTA":
		return types.E_QUOTA, true
	case "E_FLOAT":
		return types.E_FLOAT, true
	case "E_FILE":
		return types.E_FILE, true
	case "E_EXEC":
		return types.E_EXEC, true
	default:
		return 0, false
	}
}

// errorCodeToName converts ErrorCode to name
func errorCodeToName(code types.ErrorCode) string {
	switch code {
	case types.E_NONE:
		return "E_NONE"
	case types.E_TYPE:
		return "E_TYPE"
	case types.E_DIV:
		return "E_DIV"
	case types.E_PERM:
		return "E_PERM"
	case types.E_PROPNF:
		return "E_PROPNF"
	case types.E_VERBNF:
		return "E_VERBNF"
	case types.E_VARNF:
		return "E_VARNF"
	case types.E_INVIND:
		return "E_INVIND"
	case types.E_RECMOVE:
		return "E_RECMOVE"
	case types.E_MAXREC:
		return "E_MAXREC"
	case types.E_RANGE:
		return "E_RANGE"
	case types.E_ARGS:
		return "E_ARGS"
	case types.E_NACC:
		return "E_NACC"
	case types.E_INVARG:
		return "E_INVARG"
	case types.E_QUOTA:
		return "E_QUOTA"
	case types.E_FLOAT:
		return "E_FLOAT"
	case types.E_FILE:
		return "E_FILE"
	case types.E_EXEC:
		return "E_EXEC"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", code)
	}
}

// typeNameToCode converts type name to TypeCode
func typeNameToCode(name string) (types.TypeCode, bool) {
	switch strings.ToLower(name) {
	case "int":
		return types.TYPE_INT, true
	case "obj":
		return types.TYPE_OBJ, true
	case "str":
		return types.TYPE_STR, true
	case "err":
		return types.TYPE_ERR, true
	case "list":
		return types.TYPE_LIST, true
	case "float":
		return types.TYPE_FLOAT, true
	case "map":
		return types.TYPE_MAP, true
	case "anon":
		return types.TYPE_ANON, true
	case "waif":
		return types.TYPE_WAIF, true
	case "bool":
		return types.TYPE_BOOL, true
	default:
		return 0, false
	}
}

// typeCodeToName converts TypeCode to name
func typeCodeToName(code types.TypeCode) string {
	switch code {
	case types.TYPE_INT:
		return "int"
	case types.TYPE_OBJ:
		return "obj"
	case types.TYPE_STR:
		return "str"
	case types.TYPE_ERR:
		return "err"
	case types.TYPE_LIST:
		return "list"
	case types.TYPE_FLOAT:
		return "float"
	case types.TYPE_MAP:
		return "map"
	case types.TYPE_ANON:
		return "anon"
	case types.TYPE_WAIF:
		return "waif"
	case types.TYPE_BOOL:
		return "bool"
	default:
		return fmt.Sprintf("unknown(%d)", code)
	}
}
