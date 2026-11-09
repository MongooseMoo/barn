package task

import (
	"reflect"
	"testing"
)

func TestContextDoesNotExposeRuntimeServices(t *testing.T) {
	typeOfContext := reflect.TypeOf(Context{})
	for _, name := range []string{"Task", "CallerVM", "Registry"} {
		if field, ok := typeOfContext.FieldByName(name); ok {
			t.Errorf("Context.%s has type %s; runtime services must be owned and supplied explicitly", name, field.Type)
		}
	}
}
