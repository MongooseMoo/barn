package kernel

import (
	"reflect"
	"testing"
)

func TestTaskContextDoesNotExposeRuntimeServices(t *testing.T) {
	typeOfContext := reflect.TypeOf(TaskContext{})
	for _, name := range []string{"Task", "CallerVM", "Registry"} {
		if field, ok := typeOfContext.FieldByName(name); ok {
			t.Errorf("TaskContext.%s has type %s; runtime services must be owned and supplied explicitly", name, field.Type)
		}
	}
}
