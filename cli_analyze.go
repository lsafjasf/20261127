package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"
)

func runAnalyze(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("analyze", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var cfg analyzeConfig
	bindAnalyzeFlags(fs, &cfg)
	pos, err := parseInterspersed(fs, args)
	if err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	cfg.positional = pos

	windowSec, err := parseDurationArg(cfg.window)
	if err != nil || windowSec <= 0 {
		fmt.Fprintf(stderr, "无效 --window: %q\n", cfg.window)
		return 2
	}
	opt := DefaultOptions()
	opt.FPS = cfg.fps
	opt.WindowSec = windowSec
	opt.Clock = cfg.clock
	opt.DupBits = cfg.dupBits
	opt.JumpRatio = cfg.jumpRatio

	src, notes, err := loadSources(cfg)
	if err != nil {
		fmt.Fprintln(stderr, "加载输入失败:", err)
		return 2
	}
	rep, err := Analyze(src, opt)
	if err != nil {
		fmt.Fprintln(stderr, "分析失败:", err)
		return 2
	}
	rep.Notes = append(rep.Notes, notes...)

	var out io.Writer = stdout
	if cfg.out != "" {
		f, err := os.Create(cfg.out)
		if err != nil {
			fmt.Fprintln(stderr, "无法写报告文件:", err)
			return 2
		}
		defer f.Close()
		out = f
	}
	if cfg.asJSON || cfg.format == "json" {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		if err := enc.Encode(rep); err != nil {
			fmt.Fprintln(stderr, "JSON 输出失败:", err)
			return 2
		}
	} else {
		fmt.Fprint(out, RenderText(rep, cfg.allWindows))
	}
	if cfg.strict && rep.First != nil {
		return 1
	}
	return 0
}

func loadSources(cfg analyzeConfig) (Sources, []string, error) {
	src := Sources{}
	var notes []string
	load := func(p string, st Stage) error {
		tr, err := LoadTraceFile(p, st)
		if err != nil {
			return err
		}
		if parts := splitByStage(tr); parts != nil {
			mergeSources(&src, *parts)
			return nil
		}
		tr.Stage = st
		assignTrace(&src, tr)
		return nil
	}
	if cfg.capture != "" {
		if err := load(cfg.capture, StageCapture); err != nil {
			return src, notes, err
		}
	}
	if cfg.record != "" {
		if err := load(cfg.record, StageRecord); err != nil {
			return src, notes, err
		}
	}
	if cfg.player != "" {
		if err := load(cfg.player, StagePlayer); err != nil {
			return src, notes, err
		}
	}
	for _, p := range cfg.positional {
		st, err := os.Stat(p)
		if err != nil {
			return src, notes, err
		}
		if st.IsDir() {
			ds, warns, err := LoadDir(p)
			if err != nil {
				return src, notes, err
			}
			notes = append(notes, warns...)
			mergeSources(&src, ds)
			continue
		}
		stage := InferStage(p)
		if stage == StageUnknown {
			stage = StageRecord
		}
		if err := load(p, stage); err != nil {
			return src, notes, err
		}
	}
	if !src.any() {
		return src, notes, fmt.Errorf("未提供任何轨迹文件")
	}
	return src, notes, nil
}

func mergeSources(dst *Sources, add Sources) {
	if dst.Capture == nil {
		dst.Capture = add.Capture
	}
	if dst.Record == nil {
		dst.Record = add.Record
	}
	if dst.Player == nil {
		dst.Player = add.Player
	}
}

func parseDurationArg(s string) (float64, error) {
	var f float64
	if _, err := fmt.Sscanf(s, "%g", &f); err == nil && !looksLikeDuration(s) {
		return f, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, err
	}
	return d.Seconds(), nil
}

func looksLikeDuration(s string) bool {
	for _, u := range []string{"ns", "us", "µs", "ms", "s", "m", "h"} {
		if len(s) > len(u) && s[len(s)-len(u):] == u {
			return true
		}
	}
	return false
}
