package format

import (
	"strings"

	"github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/internal/isolationprobe"
	"github.com/MongooseMoo/barn/types"
)

// checkpointRowFault applies the deliberate checkpoint-writer defects of the
// elle build's Jepsen negative controls to a Jepsen property row (a list in a
// property named jp*, scripts/jepsen_checkpoint.py props_code). Normal builds
// return view unchanged.
func checkpointRowFault(name string, view store.PropertyView) store.PropertyView {
	if !isolationprobe.Enabled || view.Clear || !strings.HasPrefix(name, "jp") ||
		view.Value.Type() != types.TYPE_LIST || view.Value.Len() == 0 {
		return view
	}
	last := view.Value.Len()
	switch {
	case isolationprobe.Fault("checkpoint-drop-append"):
		// A lost checkpointed update: the row's last token is not written.
		view.Value = view.Value.DeleteAt(last)
	case isolationprobe.Fault("checkpoint-duplicate-append"):
		// A duplicated token: the row's last token is written twice.
		view.Value = view.Value.Append(view.Value.Get(last))
	}
	return view
}
