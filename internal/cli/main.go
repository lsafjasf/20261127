package cli

import (
	"fmt"
	"os"
)

func Run(arguments []string) int {
	if len(arguments) == 0 {
		usage()
		return 2
	}
	var err error
	switch arguments[0] {
	case "check":
		err = runCheck(arguments[1:])
	case "fixtures":
		err = runFixtures(arguments[1:])
	case "demo":
		err = runDemo(arguments[1:])
	case "-h", "--help", "help":
		usage()
		return 0
	default:
		err = fmt.Errorf("unknown command %q", arguments[0])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "proalpha:", err)
		return 1
	}
	return 0
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  proalpha check --source SOURCE.mov [--variant CONVERTED.mov]... --out DIR [--max-frames N] [--no-exit-code]
  proalpha fixtures --dir testdata/fixtures [--frames 4] [--width 96] [--height 54]
  proalpha demo [--dir examples/demo]

Checks MOV/MP4 color atoms, ProRes alpha_channel_type, ffprobe tags, and decoded RGBA channels.
Writes report.json, wrong-params.json, report.txt, and repaired PNG frames into --out.`)
}
