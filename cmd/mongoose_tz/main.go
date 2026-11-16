// mongoose_tz supplies Mongoose's external executables/tz helper on Windows.
// The live Linux helper prints TZ="$1" date +%z without a trailing newline.
//
//	tz America/Denver    # prints -0700 (or -0600 in summer)
//	tz --version
package main

import (
	"fmt"
	"os"
	"time"
	_ "time/tzdata" // Mongoose uses IANA names even on Windows without zoneinfo.

	"github.com/spf13/cobra"

	"github.com/MongooseMoo/barn/internal/cli"
)

func offset(now time.Time, name string) (string, error) {
	location, err := time.LoadLocation(name)
	if err != nil {
		return "", err
	}
	return now.In(location).Format("-0700"), nil
}

func main() {
	os.Exit(cli.Execute(newRootCommand(), os.Args[1:], os.Stdout, os.Stderr))
}

func newRootCommand() *cobra.Command {
	root := cli.NewRoot("mongoose_tz", "Print a time zone's current UTC offset, as Mongoose's tz helper does")
	root.Use = "mongoose_tz <IANA zone>"
	root.Long = `Print the current UTC offset of an IANA time zone as +HHMM with no trailing
newline, as the Linux helper TZ="$1" date +%z does for Mongoose's executables/tz.`
	root.Example = "  mongoose_tz America/Denver"
	root.Args = cobra.ArbitraryArgs
	root.RunE = cli.RunE(func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			// The live helper's exact text and status.
			fmt.Fprint(cmd.OutOrStdout(), "No timezone provided. Please provide a timezone as an argument.")
			return nil
		}
		value, err := offset(time.Now(), args[0])
		if err != nil {
			return err
		}
		fmt.Fprint(cmd.OutOrStdout(), value)
		return nil
	})
	return root
}
