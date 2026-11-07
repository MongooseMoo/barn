package engine

import "testing"

func TestFinallyControlTransfer(t *testing.T) {
	for _, test := range []struct{ name, source, want string }{
		{"continue", `seen={}; for i in [1..3] try continue; finally seen={@seen,i}; endtry endfor return seen;`, "{1, {1, 2, 3}}"},
		{"break", `seen={}; for i in [1..3] try break; finally seen={@seen,i}; endtry endfor return seen;`, "{1, {1}}"},
		{"loop_inside_try", `seen={}; try for i in [1..3] seen={@seen,i}; continue; endfor finally seen={@seen,99}; endtry return seen;`, "{1, {1, 2, 3, 99}}"},
		{"nested_finally", `seen={}; for i in [1..2] try try continue; finally seen={@seen,i}; endtry finally seen={@seen,-i}; endtry endfor return seen;`, "{1, {1, -1, 2, -2}}"},
		{"return_overrides_continue", `for i in [1..2] try continue; finally return 73; endtry endfor return 0;`, "{1, 73}"},
		{"continue_overrides_return", `for i in [1..2] try return 99; finally continue; endtry endfor return 73;`, "{1, 73}"},
		{"cleanup_inner_loop", `seen={}; for i in [1..2] try continue; finally for j in [1..2] try continue; finally seen={@seen,{i,j}}; endtry endfor endtry endfor return seen;`, "{1, {{1, 1}, {1, 2}, {2, 1}, {2, 2}}}"},
		{"crossed_except_removed", `seen=0; for i in [1..2] try continue; except (E_INVARG) seen=99; endtry endfor try raise(E_INVARG); except (E_INVARG) seen=73; endtry return seen;`, "{1, 73}"},
		{"nested_cleanup_preserves_return", `seen={}; try return 73; finally try seen={@seen,1}; finally seen={@seen,2}; endtry seen={@seen,3}; return seen; endtry`, "{1, {1, 2, 3}}"},
		{"nested_cleanup_preserves_error", `seen={}; try try raise(E_INVARG); finally try seen={@seen,1}; finally seen={@seen,2}; endtry seen={@seen,3}; endtry except (E_INVARG) return seen; endtry`, "{1, {1, 2, 3}}"},
		{"sibling_except_not_active", `try try raise(E_TYPE); except (E_TYPE) raise(E_INVARG); except (E_INVARG) return 99; endtry except (E_INVARG) return 73; endtry return 0;`, "{1, 73}"},
		{"named_outer_continue", `seen={}; for i in [1..2] for j in [1..2] try continue i; finally seen={@seen,{i,j}}; endtry endfor endfor return seen;`, "{1, {{1, 1}, {2, 1}}}"},
		{"named_outer_break", `seen={}; for i in [1..2] for j in [1..2] try break i; finally seen={@seen,{i,j}}; endtry endfor endfor return seen;`, "{1, {{1, 1}}}"},
		{"error_overrides_continue", `try for i in [1..2] try continue; finally raise(E_INVARG); endtry endfor except (E_INVARG) return 73; endtry return 0;`, "{1, 73}"},
		{"inner_caught_error_preserves_outer_error", `try try raise(E_TYPE); finally try try raise(E_INVARG); finally x=1; endtry except (E_INVARG) x=2; endtry endtry except (E_TYPE) return 73; endtry return 99;`, "{1, 73}"},
		{"local_break_preserves_outer_return", `try return 73; finally for j in [1..1] try return 99; finally break; endtry endfor endtry return 42;`, "{1, 73}"},
		{"local_break_preserves_outer_continue", `seen={}; for i in [1..2] try continue; finally for j in [1..1] try return 99; finally break; endtry endfor endtry seen={@seen,"bad"}; endfor return seen;`, "{1, {}}"},
		{"jump_canceled_before_outer_cleanup_exit", `for i in [1..1] try return 73; finally for j in [1..1] try continue i; finally break j; endtry endfor endtry endfor return 42;`, "{1, 73}"},
	} {
		t.Run(test.name, func(t *testing.T) {
			runtime := newTestRuntimeWithBuiltins(t, newConflictTestStore(t))
			defer runtime.Stop()
			if got := runtime.EvalCommandOutput(0, test.source); got != test.want {
				t.Fatalf("got %s, want %s", got, test.want)
			}
		})
	}
}
