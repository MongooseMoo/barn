package vm

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// Toast's except and catch-expression codes are an argument list evaluated
// when the try is entered (parser.y `codes: tANY | ne_arglist`; code_gen.cc
// generate_codes). A raised code matches an arm when ismember(code, codes, 0)
// holds (execute.cc find_handler_activ), so codes compare case-insensitively
// and need not be errors.
func TestExpressionCodesMatchAsToast(t *testing.T) {
	cases := []struct {
		name string
		code string
		want string
	}{
		{"string_code_with_variable", "try raise(\"E_CUSTOM\"); except e (\"E_CUSTOM\") return e[1]; endtry", `"E_CUSTOM"`},
		{"string_code_without_variable", "try raise(\"E_FUSE\"); except (\"E_FUSE\") return \"caught\"; endtry", `"caught"`},
		{"string_code_is_case_insensitive", "try raise(\"E_CUSTOM\"); except (\"e_custom\") return \"caught\"; endtry", `"caught"`},
		{"variable_code", "c = \"E_X\"; try raise(\"E_X\"); except (c) return \"caught\"; endtry", `"caught"`},
		{"spliced_codes", "cs = {\"A\", \"B\"}; try raise(\"B\"); except (E_DIV, @cs) return \"caught\"; endtry", `"caught"`},
		{"error_code_in_expression_list", "c = 1; try 1 / 0; except (c, E_DIV) return \"caught\"; endtry", `"caught"`},
		{"later_literal_arm_after_expression_arm", "try 1 / 0; except (\"x\") return \"wrong\"; except (E_DIV) return \"literal\"; endtry", `"literal"`},
		{"earlier_expression_arm_wins", "try raise(\"x\"); except (\"x\") return \"expression\"; except (ANY) return \"any\"; endtry", `"expression"`},
		{"unmatched_expression_arm_propagates", "try try raise(\"x\"); except (\"y\") return \"wrong\"; endtry except (ANY) return \"outer\"; endtry", `"outer"`},
		{"codes_evaluate_before_body", "n = 0; try n = n * 10; except (n = n + 1) endtry return n;", `10`},
		{"error_in_codes_is_not_caught_by_its_own_try", "try try x = 1; except (1 / 0) return \"inner\"; endtry except (E_DIV) return \"outer\"; endtry", `"outer"`},
		{"bool_code_matches_int", "try raise(1); except (true) return \"caught\"; endtry", `"caught"`},
		{"e_none_does_not_catch_string_raise", "try try raise(\"x\"); except (E_NONE) return \"wrong\"; endtry except (ANY) return \"outer\"; endtry", `"outer"`},
		{"catch_non_error_code", "return `raise(1) ! 1 => \"caught\"';", `"caught"`},
		{"catch_unmatched_code_propagates", "return ``raise(\"x\") ! \"y\" => \"wrong\"' ! ANY => \"outer\"';", `"outer"`},
		{"catch_default_assignment_assigns", "x = 0; `1 / 0 ! ANY => x = 5'; return x;", `5`},
		{"catch_default_assignment_is_value", "return `1 / 0 ! ANY => x = 5';", `5`},
		{"splice_of_assignment_in_list", "x = {@ret = {1, 2}}; return {x, ret};", `{{1, 2}, {1, 2}}`},
		{"splice_of_assignment_in_call", "return {length({@ret = {1, 2}}), max(@ret = {3, 4}), ret};", `{2, 4, {3, 4}}`},
		{"true_is_assignable_variable", "x = true = {}; return {x, true};", `{{}, {}}`},
		{"false_is_assignable_variable", "x = false = 7; return {x, false};", `{7, 7}`},
		{"true_and_false_start_as_booleans", "return {true, false, TRUE, False, typeof(true) == BOOL};", `{true, false, true, false, 1}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := runBytecodeProgram(t, tc.code, nil, nil)
			if result.Flow != types.FlowReturn || result.Val.String() != tc.want {
				t.Fatalf("result = flow %v, value %v, error %v; want %s", result.Flow, result.Val, result.Error, tc.want)
			}
		})
	}
}
