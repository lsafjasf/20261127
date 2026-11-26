package main

import (
	"os"

	"proalpha/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
