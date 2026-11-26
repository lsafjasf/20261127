package main

import (
	"errors"
	"fmt"
	"os"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		var silent silentExit
		if errors.As(err, &silent) {
			os.Exit(silent.code)
		}
		fmt.Fprintf(os.Stderr, "renderctl: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	command := "run"
	if len(args) > 0 && !isFlag(args[0]) {
		command = args[0]
		args = args[1:]
	}

	switch command {
	case "run":
		return runCommand(args)
	case "status":
		return statusCommand(args)
	case "recover":
		return recoverCommand(args)
	case "quarantine":
		return quarantineCommand(args)
	case "help", "-h", "--help":
		printUsage()
		return nil
	default:
		printUsage()
		return fmt.Errorf("unknown command %q", command)
	}
}

func isFlag(value string) bool {
	return len(value) > 0 && value[0] == '-'
}

func printUsage() {
	fmt.Print(`renderctl - checkpointed chapter renderer

Usage:
  renderctl [run] [flags]
  renderctl status --root DIR --job ID
  renderctl recover --root DIR --job ID
  renderctl quarantine --root DIR

Run flags:
  --root DIR          workspace directory (default ".render-workspace")
  --job ID            checkpoint job id (default "demo-book")
  --chapters N        generated demo chapters (default 4)
  --delay DURATION    simulated stage duration (default 25ms)
  --concurrency N     parallel nodes (default 2)
  --cancel-after DUR  self-cancel for demonstration, e.g. 120ms
  --no-publish        do not copy final files into output/

Signals:
  Ctrl-C / SIGTERM cancels the current run. Completed nodes and their
  artifacts remain reusable; run the same command again to resume.
`)
}
