// Command imfplaylist merges IMF main + supplemental packages by resolving
// CPL/OPL resource inheritance and reports missing/duplicate assets.
//
// `go run .` is equivalent to `go run ./cmd/imfplaylist`.
package main

import (
	"os"

	"imfplaylist/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
