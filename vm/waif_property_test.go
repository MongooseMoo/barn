package vm

import (
	"testing"

	"barn/types"
)

func TestWaifWizardAndProgrammerIntrinsicsAreFalse(t *testing.T) {
	waif := types.NewWaif(10, 1)
	for _, property := range []string{"wizard", "programmer"} {
		t.Run(property, func(t *testing.T) {
			machine := NewVM(nil, nil)
			if err := machine.getWaifProp(waif, property); err != nil {
				t.Fatalf("getWaifProp(%q) failed: %v", property, err)
			}
			value := machine.Pop()
			if value.Type() != types.TYPE_INT || value.Int() != 0 {
				t.Fatalf("waif.%s = %v, want integer 0", property, value)
			}
		})
	}
}
