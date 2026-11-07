// moo-fuzz-source prints the MOO program that the structured fuzz targets
// generate from the choice bytes read on stdin.
//
//	moo-fuzz-source < choices.bin
//	head -c 200 /dev/urandom | moo-fuzz-source
//
// It prints one example program. It is not a grammar of the MOO language.
// It exits 1 when the generated program has no MOO spelling.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/MongooseMoo/barn/internal/fuzzseed"
)

func main() {
	choices, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	source, err := fuzzseed.Source(choices)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(source)
}
