package vm

import "testing"

// F21: labeled break/continue must execute correctly at runtime. `break ID;` /
// `continue ID;` target the enclosing loop whose (variable) name is ID
// (ToastStunt parser.y:241-264 + push_loop_name on the loop variable id).

func TestLabeledBreakTargetsOuterLoop(t *testing.T) {
	// `break i;` from the inner loop exits the OUTER loop entirely.
	// i=1: j=1 -> sum=1; j=2 -> break i (exits outer). Final: 1.
	code := `sum = 0;
for i in ({1, 2, 3})
  for j in ({1, 2, 3})
    if (j == 2)
      break i;
    endif
    sum = sum + 1;
  endfor
endfor
return sum;`
	requireInt(t, runBytecodeProgram(t, code, nil, nil), 1)
}

func TestLabeledContinueTargetsOuterLoop(t *testing.T) {
	// `continue i;` from the inner loop continues the OUTER loop.
	// Each i: j=1 -> sum+=1; j=2 -> continue i. 3 outer iterations => sum=3.
	code := `sum = 0;
for i in ({1, 2, 3})
  for j in ({1, 2, 3})
    if (j == 2)
      continue i;
    endif
    sum = sum + 1;
  endfor
endfor
return sum;`
	requireInt(t, runBytecodeProgram(t, code, nil, nil), 3)
}

func TestPlainBreakStillWorks(t *testing.T) {
	// i=1 sum=1, i=2 sum=2, i=3 break. Final: 2.
	code := `sum = 0;
for i in ({1, 2, 3, 4, 5})
  if (i == 3)
    break;
  endif
  sum = sum + 1;
endfor
return sum;`
	requireInt(t, runBytecodeProgram(t, code, nil, nil), 2)
}

func TestPlainContinueStillWorks(t *testing.T) {
	// i=3 skipped, rest counted. Final: 4.
	code := `sum = 0;
for i in ({1, 2, 3, 4, 5})
  if (i == 3)
    continue;
  endif
  sum = sum + 1;
endfor
return sum;`
	requireInt(t, runBytecodeProgram(t, code, nil, nil), 4)
}
