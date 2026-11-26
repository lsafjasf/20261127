package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "lfecheck:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		usage()
		return errors.New("missing command")
	}
	switch args[0] {
	case "analyze":
		return runAnalyze(args[1:])
	case "repair":
		return runRepair(args[1:])
	case "demo":
		return runDemo(args[1:])
	case "help", "-h", "--help":
		usage()
		return nil
	default:
		usage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  lfecheck analyze --in program.wav [--dialogue FC] [--source auto] [--report out.json]
  lfecheck repair  --in program.wav --out repaired.wav [--source auto] [--report out.json]
  lfecheck demo    [--out-dir artifacts/demo] [--rate 48000]

analysis:
  --low 20 --high 120 --window 100ms --hop 20ms --max-lag 20ms
  --min-corr 0.55 --min-duration 120ms --merge-gap 80ms
  --low-floor -60 --dialogue-gate -60 --leak-margin 3 --leak-below-dialogue 30`)
}

func defaultReport(input, suffix string) string {
	base := strings.TrimSuffix(input, filepath.Ext(input))
	return base + suffix
}
