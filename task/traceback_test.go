package task

import (
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func TestFormatTracebackJoinsStoredVerbNamesLazily(t *testing.T) {
	frame := ActivationFrame{
		This:            1,
		Verb:            "evaluate",
		StoredVerbNames: []string{"eval*-d", "evaluate"},
		VerbLoc:         2,
		LineNumber:      3,
	}

	traceback := FormatTraceback([]ActivationFrame{frame}, types.E_INVARG)
	if got := traceback[0]; !strings.Contains(got, "#2:eval*-d evaluate ") {
		t.Fatalf("traceback did not use joined stored verb names: %q", got)
	}
}
