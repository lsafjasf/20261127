package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
)

func runGen(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("gen", flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("out", "./framedoctor-scenario", "output directory")
	fps := fs.Float64("fps", 30, "frames per second")
	seconds := fs.Int("seconds", 12, "duration seconds")
	if _, err := parseInterspersed(fs, args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	gt, err := GenerateScenario(*out, *fps, *seconds)
	if err != nil {
		fmt.Fprintln(stderr, "生成场景失败:", err)
		return 2
	}
	fmt.Fprintf(stdout, "已生成合成场景到 %s (fps=%.0f)\n", *out, gt.FPS)
	fmt.Fprintf(stdout, "植入异常: 采集丢帧@%.1fs 编码重复@%.3fs 播放跳帧@%.1fs\n",
		gt.Planted[0].AtS, gt.Planted[1].AtS, gt.Planted[2].AtS)
	fmt.Fprintf(stdout, "运行: framedoctor analyze --all-windows %s\n", *out)
	return 0
}

func runExtract(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("extract", flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("out", "", "output JSONL file (default stdout)")
	pos, err := parseInterspersed(fs, args)
	if err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if len(pos) < 1 {
		fmt.Fprintln(stderr, "用法: framedoctor extract <video> [--out trace.jsonl]")
		return 2
	}
	video := pos[0]
	var w io.Writer = stdout
	if *out != "" {
		f, err := os.Create(*out)
		if err != nil {
			fmt.Fprintln(stderr, "无法创建输出:", err)
			return 2
		}
		defer f.Close()
		w = f
	}
	n, err := ExtractVideo(context.Background(), video, w)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	fmt.Fprintf(stderr, "已提取 %d 帧轨迹\n", n)
	return 0
}

func runDemo(stdout, stderr io.Writer) int {
	dir, err := os.MkdirTemp("", "framedoctor-demo-*")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if _, err := GenerateScenario(dir, 30, 12); err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	return runAnalyze([]string{"--all-windows", dir}, stdout, stderr)
}
