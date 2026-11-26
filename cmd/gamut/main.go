package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"gamutcompare"
)

func main() {
	inputPath := flag.String("input", "", "输入 JSON 路径；使用 - 从 stdin 读取")
	format := flag.String("format", "text", "输出格式：text 或 json")
	center := flag.String("center", "", "方向中心：display_white（默认）、target_white、xy_origin")
	demo := flag.Bool("demo", false, "运行内置示例")
	flag.Parse()

	data, err := readInput(*demo, *inputPath)
	if err != nil {
		fatal(err)
	}

	displays, target, options, err := gamutcompare.ParseInput(data)
	if err != nil {
		fatal(err)
	}
	if *center != "" {
		options.CenterMode = *center
	}

	report, err := gamutcompare.Analyze(displays, target, options)
	if err != nil {
		fatal(err)
	}

	switch *format {
	case "json":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			fatal(err)
		}
	case "text":
		fmt.Print(gamutcompare.FormatText(report))
	default:
		fatal(fmt.Errorf("unknown format %q", *format))
	}
}

func readInput(demo bool, path string) ([]byte, error) {
	if demo {
		return gamutcompare.SampleInputJSON, nil
	}
	if path == "" {
		flag.Usage()
		return nil, fmt.Errorf("请使用 -input 指定 JSON，或运行 -demo")
	}
	if path == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(path)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "gamut:", err)
	os.Exit(1)
}
