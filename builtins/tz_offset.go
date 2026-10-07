package builtins

import (
	"time"
	_ "time/tzdata" // Supply IANA zones on hosts without a zoneinfo database.

	"github.com/MongooseMoo/barn/types"
)

// builtinTZOffset implements tz_offset(zone [, time]) with the same numeric
// offset format as Mongoose's executables/tz helper.
func builtinTZOffset(_ *Execution, args []types.Value) types.Result {
	location, err := time.LoadLocation(args[0].Str())
	if err != nil {
		return types.Err(types.E_INVARG)
	}
	var stamp time.Time
	if len(args) == 2 {
		stamp = time.Unix(args[1].Int(), 0)
	} else {
		stamp = time.Now()
	}
	return types.Ok(types.NewStr(stamp.In(location).Format("-0700")))
}
