package store

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func TestObjectAllows(t *testing.T) {
	tests := []struct {
		name       string
		owner      types.ObjID
		flags      ObjectFlags
		programmer types.ObjID
		isWizard   bool
		required   ObjectFlags
		want       bool
	}{
		{name: "wizard", owner: 1, programmer: 2, isWizard: true, required: FlagWrite, want: true},
		{name: "owner", owner: 1, programmer: 1, required: FlagWrite, want: true},
		{name: "read flag", owner: 1, flags: FlagRead, programmer: 2, required: FlagRead, want: true},
		{name: "write flag", owner: 1, flags: FlagWrite, programmer: 2, required: FlagWrite, want: true},
		{name: "wrong flag", owner: 1, flags: FlagRead, programmer: 2, required: FlagWrite, want: false},
		{name: "no authority", owner: 1, programmer: 2, required: FlagRead, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ObjectAllows(test.owner, test.flags, test.programmer, test.isWizard, test.required); got != test.want {
				t.Errorf("ObjectAllows(%d, %v, %d, %v, %v) = %v, want %v", test.owner, test.flags, test.programmer, test.isWizard, test.required, got, test.want)
			}
		})
	}
}
