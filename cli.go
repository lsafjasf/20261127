package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

const helpText = `framedoctor — 直播录制掉帧/重复/跳帧审计工具

用法:
  framedoctor analyze [选项] <轨迹文件或目录...>
  framedoctor gen --out <目录> [--fps 30] [--seconds 12]
  framedoctor extract <视频文件> [--out trace.jsonl]
  framedoctor demo

analyze 选项:
  --capture <file>   采集端轨迹 (CSV/TSV/JSON/JSONL)
  --record  <file>    录制/编码端轨迹
  --player  <file>    播放/渲染端轨迹
  --fps <n>           标称帧率，缺省按时间戳中位数估计
  --window <dur>      时间窗，如 1s / 500ms / 0.5 (默认 1s)
  --clock <axis>      auto|pts|wall (默认 auto)
  --dup-bits <n>      感知哈希 <=n bit 视为同一画面 (默认 2)
  --jump-ratio <x>    跳变判定倍数 (默认 3)
  --all-windows       文本报告输出全部时间窗
  --format <fmt>      text|json (默认 text)
  --json              等价于 --format json
  --out <file>        报告输出到文件 (默认 stdout)
  --strict            发现异常时退出码为 1

轨迹字段 (CSV 表头或 JSON 键，别名宽松匹配):
  seq/frame_no, pts/pts_time, pts_ms, wall_ms, hash/phash/dhash/md5,
  diff/advance/motion, hamming, dropped

示例:
  framedoctor gen --out ./examples/scenario
  framedoctor analyze ./examples/scenario
  framedoctor analyze -r record.jsonl -c capture.csv -p player.csv --json
  framedoctor extract live.mp4 --out live.trace.jsonl
`

// RunCLI executes the command line and returns the process exit code.
func RunCLI(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stdout, helpText)
		return 0
	}
	cmd := args[0]
	if cmd == "help" || cmd == "-h" || cmd == "--help" {
		fmt.Fprint(stdout, helpText)
		return 0
	}
	switch cmd {
	case "analyze":
		return runAnalyze(args[1:], stdout, stderr)
	case "gen":
		return runGen(args[1:], stdout, stderr)
	case "extract":
		return runExtract(args[1:], stdout, stderr)
	case "demo":
		return runDemo(stdout, stderr)
	case "version", "--version":
		fmt.Fprintln(stdout, "framedoctor v1.0.0")
		return 0
	default:
		if strings.HasPrefix(cmd, "-") || pathExists(cmd) {
			return runAnalyze(args, stdout, stderr)
		}
		fmt.Fprintf(stderr, "未知命令: %s\n\n%s", cmd, helpText)
		return 2
	}
}

func pathExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// parseInterspersed parses flags that may appear both before and after
// positional arguments, returning the collected positionals.
func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for len(args) > 0 {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		rest := fs.Args()
		if len(rest) == 0 {
			break
		}
		if rest[0] == "--" {
			pos = append(pos, rest[1:]...)
			break
		}
		pos = append(pos, rest[0])
		args = rest[1:]
	}
	return pos, nil
}

type analyzeConfig struct {
	capture    string
	record     string
	player     string
	fps        float64
	window     string
	clock      string
	dupBits    int
	jumpRatio  float64
	allWindows bool
	format     string
	asJSON     bool
	out        string
	strict     bool
	positional []string
}

func bindAnalyzeFlags(fs *flag.FlagSet, cfg *analyzeConfig) {
	fs.StringVar(&cfg.capture, "capture", "", "capture trace file")
	fs.StringVar(&cfg.capture, "c", "", "capture trace file (shorthand)")
	fs.StringVar(&cfg.record, "record", "", "record trace file")
	fs.StringVar(&cfg.record, "r", "", "record trace file (shorthand)")
	fs.StringVar(&cfg.player, "player", "", "player trace file")
	fs.StringVar(&cfg.player, "p", "", "player trace file (shorthand)")
	fs.Float64Var(&cfg.fps, "fps", 0, "nominal fps (0=estimate)")
	fs.StringVar(&cfg.window, "window", "1s", "window duration")
	fs.StringVar(&cfg.clock, "clock", "auto", "time axis: auto|pts|wall")
	fs.IntVar(&cfg.dupBits, "dup-bits", 2, "phash hamming bits treated as duplicate")
	fs.Float64Var(&cfg.jumpRatio, "jump-ratio", 3.0, "jump edge ratio threshold")
	fs.BoolVar(&cfg.allWindows, "all-windows", false, "print every window in text report")
	fs.StringVar(&cfg.format, "format", "text", "output format text|json")
	fs.BoolVar(&cfg.asJSON, "json", false, "shorthand for --format json")
	fs.StringVar(&cfg.out, "out", "", "output file")
	fs.BoolVar(&cfg.strict, "strict", false, "exit 1 when anomalies found")
}
