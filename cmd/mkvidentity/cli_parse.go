package main

import (
	"flag"
	"strings"
)

func parseInterspersed(fs *flag.FlagSet, args []string) error {
	var flagArgs []string
	var positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positional = append(positional, arg)
			continue
		}
		flagArgs = append(flagArgs, arg)
		if strings.Contains(arg, "=") {
			continue
		}
		name := strings.TrimLeft(arg, "-")
		field := fs.Lookup(name)
		if field == nil {
			continue
		}
		boolFlag, ok := field.Value.(interface{ IsBoolFlag() bool })
		if ok && boolFlag.IsBoolFlag() {
			continue
		}
		if i+1 < len(args) {
			i++
			flagArgs = append(flagArgs, args[i])
		}
	}
	return fs.Parse(append(flagArgs, positional...))
}
