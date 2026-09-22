// mongoose_tz supplies Mongoose's external executables/tz helper on Windows.
// The live Linux helper prints TZ="$1" date +%z without a trailing newline.
package main

import (
	"fmt"
	"os"
	"time"
	_ "time/tzdata" // Mongoose uses IANA names even on Windows without zoneinfo.
)

func offset(now time.Time, name string) (string, error) {
	location, err := time.LoadLocation(name)
	if err != nil {
		return "", err
	}
	return now.In(location).Format("-0700"), nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Print("No timezone provided. Please provide a timezone as an argument.")
		return
	}
	value, err := offset(time.Now(), os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Print(value)
}
