package vm

import (
	"math"
	"testing"

	"barn/config"
	"barn/kernel"
	"barn/types"
)

// requirePromoteFloat asserts the result is a FloatValue with the given value.
func requirePromoteFloat(t *testing.T, result types.Result, want float64) {
	t.Helper()
	if result.Flow != types.FlowReturn && result.Flow != types.FlowNormal {
		t.Fatalf("flow = %v, want float %v (error %s, val %v)", result.Flow, want, result.Error, result.Val)
	}
	got, ok := result.Val.(types.FloatValue)
	if !ok {
		t.Fatalf("value = %T %v, want float %v", result.Val, result.Val, want)
	}
	if got.Val != want {
		t.Fatalf("value = %v, want %v", got.Val, want)
	}
}

// strictCtx returns a default (strict) task context.
func strictCtx() *kernel.TaskContext {
	return kernel.NewTaskContext()
}

// promoteCtx returns a task context with PromoteNumbers enabled.
func promoteCtx() *kernel.TaskContext {
	ctx := kernel.NewTaskContext()
	ctx.RuntimeOptions = config.Options{OutboundNetwork: true, PromoteNumbers: true}
	return ctx
}

// --- STRICT MODE: mixed int/float must behave exactly as today ---

func TestPromoteNumbers_StrictArithmeticIsETYPE(t *testing.T) {
	exprs := []string{
		"1 + 2.0",
		"3.0 + 1",
		"2.0 - 1",
		"1 - 2.0",
		"2 * 3.0",
		"3.0 * 2",
		"7 / 2.0",
		"7.0 / 2",
		"5 % 2.0",
		"5.0 % 2",
		"2 ^ 1.5",
	}
	for _, expr := range exprs {
		t.Run(expr, func(t *testing.T) {
			result := runBytecodeProgram(t, "return "+expr+";", nil, strictCtx())
			requireError(t, result, types.E_TYPE)
		})
	}
}

func TestPromoteNumbers_StrictCompareIsETYPE(t *testing.T) {
	exprs := []string{
		"1.0 < 2",
		"2 < 1.0",
		"1 <= 1.0",
		"1.0 > 0",
		"1 >= 1.0",
	}
	for _, expr := range exprs {
		t.Run(expr, func(t *testing.T) {
			result := runBytecodeProgram(t, "return "+expr+";", nil, strictCtx())
			requireError(t, result, types.E_TYPE)
		})
	}
}

func TestPromoteNumbers_StrictEqualityIsZero(t *testing.T) {
	// Under strict, 1 == 1.0 is false (different types), 1 != 1.0 is true.
	result := runBytecodeProgram(t, "return 1 == 1.0;", nil, strictCtx())
	requireInt(t, result, 0)

	result = runBytecodeProgram(t, "return 1 != 1.0;", nil, strictCtx())
	requireInt(t, result, 1)
}

func TestPromoteNumbers_StrictPureIntFloatUnchanged(t *testing.T) {
	// Pure int and pure float ops are unaffected by strict mode.
	requireInt(t, runBytecodeProgram(t, "return 1 + 2;", nil, strictCtx()), 3)
	requirePromoteFloat(t, runBytecodeProgram(t, "return 1.0 + 2.0;", nil, strictCtx()), 3.0)
	requireInt(t, runBytecodeProgram(t, "return 7 / 2;", nil, strictCtx()), 3)
}

// --- PROMOTE MODE: mixed int/float auto-promote to float ---

func TestPromoteNumbers_PromoteAdd(t *testing.T) {
	requirePromoteFloat(t, runBytecodeProgram(t, "return 1 + 2.0;", nil, promoteCtx()), 3.0)
	requirePromoteFloat(t, runBytecodeProgram(t, "return 3.0 + 1;", nil, promoteCtx()), 4.0)
}

func TestPromoteNumbers_PromoteSub(t *testing.T) {
	requirePromoteFloat(t, runBytecodeProgram(t, "return 2.0 - 1;", nil, promoteCtx()), 1.0)
	requirePromoteFloat(t, runBytecodeProgram(t, "return 5 - 1.5;", nil, promoteCtx()), 3.5)
}

func TestPromoteNumbers_PromoteMul(t *testing.T) {
	requirePromoteFloat(t, runBytecodeProgram(t, "return 2 * 3.0;", nil, promoteCtx()), 6.0)
	requirePromoteFloat(t, runBytecodeProgram(t, "return 3.0 * 2;", nil, promoteCtx()), 6.0)
}

func TestPromoteNumbers_PromoteDiv(t *testing.T) {
	// Pure int/int stays int even in promote mode.
	requireInt(t, runBytecodeProgram(t, "return 7 / 2;", nil, promoteCtx()), 3)
	requirePromoteFloat(t, runBytecodeProgram(t, "return 7 / 2.0;", nil, promoteCtx()), 3.5)
	requirePromoteFloat(t, runBytecodeProgram(t, "return 7.0 / 2;", nil, promoteCtx()), 3.5)
}

func TestPromoteNumbers_PromoteMod(t *testing.T) {
	requirePromoteFloat(t, runBytecodeProgram(t, "return 5 % 2.0;", nil, promoteCtx()), 1.0)
	requirePromoteFloat(t, runBytecodeProgram(t, "return 5.0 % 2;", nil, promoteCtx()), 1.0)
}

func TestPromoteNumbers_PromotePow(t *testing.T) {
	// 2 ^ 1.5 = sqrt(8) ~ 2.8284...
	requirePromoteFloat(t, runBytecodeProgram(t, "return 2 ^ 1.5;", nil, promoteCtx()), math.Pow(2, 1.5))
	// int ^ float becomes legal under promote.
	requirePromoteFloat(t, runBytecodeProgram(t, "return 4 ^ 0.5;", nil, promoteCtx()), 2.0)
	// float ^ int.
	requirePromoteFloat(t, runBytecodeProgram(t, "return 2.0 ^ 3;", nil, promoteCtx()), 8.0)
	// int ^ int unchanged (stays int).
	requireInt(t, runBytecodeProgram(t, "return 2 ^ 3;", nil, promoteCtx()), 8)
}

func TestPromoteNumbers_PromoteCompare(t *testing.T) {
	requireInt(t, runBytecodeProgram(t, "return 1.0 < 2;", nil, promoteCtx()), 1)
	requireInt(t, runBytecodeProgram(t, "return 2 < 1.0;", nil, promoteCtx()), 0)
	requireInt(t, runBytecodeProgram(t, "return 1 <= 1.0;", nil, promoteCtx()), 1)
	requireInt(t, runBytecodeProgram(t, "return 1.0 > 0;", nil, promoteCtx()), 1)
	requireInt(t, runBytecodeProgram(t, "return 1 >= 1.0;", nil, promoteCtx()), 1)
}

func TestPromoteNumbers_PromoteEquality(t *testing.T) {
	requireInt(t, runBytecodeProgram(t, "return 1 == 1.0;", nil, promoteCtx()), 1)
	requireInt(t, runBytecodeProgram(t, "return 1.0 == 1;", nil, promoteCtx()), 1)
	requireInt(t, runBytecodeProgram(t, "return 1 != 1.0;", nil, promoteCtx()), 0)
	requireInt(t, runBytecodeProgram(t, "return 1 == 2.0;", nil, promoteCtx()), 0)
	requireInt(t, runBytecodeProgram(t, "return 1 != 2.0;", nil, promoteCtx()), 1)
}

// TestPromoteNumbers_PromotePowNegativeIntExponent verifies the mongoose PROMOTE
// do_power semantics for int ^ (negative int): a raw float pow with NO E_DIV
// special-case and NO IS_REAL/E_FLOAT rejection. Non-negative exponents stay int.
// Verified against Toast mongoose numbers.cc do_power (PROMOTE_NUMBERS branch).
func TestPromoteNumbers_PromotePowNegativeIntExponent(t *testing.T) {
	requirePromoteFloat(t, runBytecodeProgram(t, "return 2 ^ -1;", nil, promoteCtx()), 0.5)
	requirePromoteFloat(t, runBytecodeProgram(t, "return 10 ^ -2;", nil, promoteCtx()), 0.01)
	// (-1) ^ -1 -> -1.0 float (parens needed: unary minus binds looser than ^).
	requirePromoteFloat(t, runBytecodeProgram(t, "return (-1) ^ -1;", nil, promoteCtx()), -1.0)
	// 0 ^ -1 -> +Inf float, NOT E_DIV.
	res := runBytecodeProgram(t, "return 0 ^ -1;", nil, promoteCtx())
	if res.Flow != types.FlowReturn && res.Flow != types.FlowNormal {
		t.Fatalf("0 ^ -1 promote: flow = %v (err %s), want +Inf float", res.Flow, res.Error)
	}
	f, ok := res.Val.(types.FloatValue)
	if !ok || !math.IsInf(f.Val, 1) {
		t.Fatalf("0 ^ -1 promote: val = %T %v, want +Inf float", res.Val, res.Val)
	}
	// Non-negative exponent stays int.
	requireInt(t, runBytecodeProgram(t, "return 2 ^ 3;", nil, promoteCtx()), 8)
}

// TestPromoteNumbers_StrictPowUnchanged locks current strict-mode pow behavior
// (must remain byte-identical when the flag is OFF).
func TestPromoteNumbers_StrictPowUnchanged(t *testing.T) {
	requireInt(t, runBytecodeProgram(t, "return 2 ^ -1;", nil, strictCtx()), 0)
	requireInt(t, runBytecodeProgram(t, "return 10 ^ -2;", nil, strictCtx()), 0)
	requireInt(t, runBytecodeProgram(t, "return (-1) ^ -1;", nil, strictCtx()), -1)
	requireInt(t, runBytecodeProgram(t, "return 1 ^ -1;", nil, strictCtx()), 1)
	requireInt(t, runBytecodeProgram(t, "return 2 ^ 3;", nil, strictCtx()), 8)
	// 0 ^ -1 -> E_DIV (strict).
	requireError(t, runBytecodeProgram(t, "return 0 ^ -1;", nil, strictCtx()), types.E_DIV)
	// int ^ float -> E_TYPE (strict).
	requireError(t, runBytecodeProgram(t, "return 2 ^ 1.5;", nil, strictCtx()), types.E_TYPE)
}

// --- PROMOTE MODE edge cases (numbers.cc) ---

func TestPromoteNumbers_PromoteDivByZeroIsEDIV(t *testing.T) {
	// Mixed promoted divide-by-zero -> E_DIV (not E_FLOAT).
	requireError(t, runBytecodeProgram(t, "return 1 / 0.0;", nil, promoteCtx()), types.E_DIV)
	requireError(t, runBytecodeProgram(t, "return 5.0 / 0;", nil, promoteCtx()), types.E_DIV)
}

func TestPromoteNumbers_PromoteModByZeroIsEDIV(t *testing.T) {
	requireError(t, runBytecodeProgram(t, "return 5 % 0.0;", nil, promoteCtx()), types.E_DIV)
	requireError(t, runBytecodeProgram(t, "return 5.0 % 0;", nil, promoteCtx()), types.E_DIV)
}

func TestPromoteNumbers_PromoteOverflowIsEFLOAT(t *testing.T) {
	// 1.0e308 * 10 (mixed int->float) overflows to Inf -> E_FLOAT.
	requireError(t, runBytecodeProgram(t, "return 1.0e308 * 10;", nil, promoteCtx()), types.E_FLOAT)
}

func TestPromoteNumbers_PromoteMinIntDivStillInt(t *testing.T) {
	// MININT / -1 on the pure int/int branch -> MININT (unchanged by promote).
	// MININT has no positive magnitude, so build it as (MININT+1) - 1 to avoid
	// a lexer/parse issue with the bare literal.
	code := "x = " + intToStr(MININT+1) + " - 1; return x / -1;"
	result := runBytecodeProgram(t, code, nil, promoteCtx())
	requireInt(t, result, MININT)
}

func intToStr(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	var digits []byte
	// Handle MININT safely using uint64.
	var u uint64
	if neg {
		u = uint64(-(v + 1)) + 1
	} else {
		u = uint64(v)
	}
	for u > 0 {
		digits = append([]byte{byte('0' + u%10)}, digits...)
		u /= 10
	}
	if neg {
		return "-" + string(digits)
	}
	return string(digits)
}

// --- cluster_by_proximity reduction (proving fixture) ---

func TestPromoteNumbers_ClusterByProximityReduction(t *testing.T) {
	code := `
		threshold = 5.0;
		best_dist = threshold + 1;
		dist = 2.5;
		return dist < best_dist;
	`
	// Strict: threshold + 1 (float + int) -> E_TYPE.
	strictResult := runBytecodeProgram(t, code, nil, strictCtx())
	requireError(t, strictResult, types.E_TYPE)

	// Promote: best_dist = 6.0, dist < best_dist -> 1.
	promoteResult := runBytecodeProgram(t, code, nil, promoteCtx())
	requireInt(t, promoteResult, 1)
}
