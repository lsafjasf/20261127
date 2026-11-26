package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"avsync"
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "avsync:", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("avsync", flag.ContinueOnError)
	fs.SetOutput(stderr)
	inPath := fs.String("in", "-", "input JSON path, or - for stdin")
	jsonOut := fs.Bool("json", false, "write machine-readable JSON")
	matchWindow := fs.Float64("match-window-ms", 0, "maximum pulse/event pairing error in ms")
	offsetTolerance := fs.Float64("offset-tolerance-ms", 0, "per-segment sync tolerance in ms")
	driftTolerance := fs.Float64("drift-tolerance-ppm", 0, "per-segment drift tolerance in ppm")
	minPairs := fs.Int("min-pairs", 0, "minimum matched pairs per segment")
	minDriftSpan := fs.Float64("min-drift-span-ms", 0, "minimum pair span for drift estimation")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: avsync [flags] [input.json]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 1 {
		return errors.New("only one input path is allowed")
	}
	if fs.NArg() == 1 {
		if *inPath != "-" {
			return errors.New("input path supplied both positionally and with -in")
		}
		*inPath = fs.Arg(0)
	}

	data, err := readInput(*inPath, stdin)
	if err != nil {
		return err
	}
	in, err := avsync.Unmarshal(data)
	if err != nil {
		return fmt.Errorf("decode input: %w", err)
	}
	if *matchWindow > 0 {
		in.Options.MatchWindowMS = *matchWindow
	}
	if *offsetTolerance > 0 {
		in.Options.OffsetToleranceMS = *offsetTolerance
	}
	if *driftTolerance > 0 {
		in.Options.DriftTolerancePPM = *driftTolerance
	}
	if *minPairs > 0 {
		in.Options.MinPairs = *minPairs
	}
	if *minDriftSpan > 0 {
		in.Options.MinDriftSpanMS = *minDriftSpan
	}

	res, err := avsync.Analyze(in)
	if err != nil {
		return err
	}
	if *jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(res)
	}
	writeReport(stdout, res)
	return nil
}

func readInput(path string, stdin io.Reader) ([]byte, error) {
	if path == "-" {
		data, err := io.ReadAll(stdin)
		if err != nil {
			return nil, fmt.Errorf("read stdin: %w", err)
		}
		if len(data) == 0 {
			return nil, errors.New("empty input")
		}
		return data, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return data, nil
}

func writeReport(w io.Writer, r avsync.Result) {
	fmt.Fprintln(w, "分段音画同步分析（时间单位毫秒）")
	fmt.Fprintf(w, "配对 %d；未配对脉冲 %d；未配对画面事件 %d\n\n",
		r.MatchedPairs, r.UnmatchedPulse, r.UnmatchedEvent)

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "段\t来源\t配对\t起点偏移\t末端偏移\t漂移(ppm)\t修复后RMS\t修复后MAX\t状态")
	for _, s := range r.Segments {
		status := "正常"
		if s.Anomalous {
			status = "异常:" + strings.Join(s.AnomalyWhy, ",")
		}
		fmt.Fprintf(tw, "%s\t%s\t%d\t%.3f\t%.3f\t%.3f\t%.3f\t%.3f\t%s\n",
			s.ID, s.Source, s.PairCount, s.OffsetMS, s.OffsetEndMS,
			s.DriftPPM, s.RMSAfterMS, s.MaxAfterMS, status)
	}
	tw.Flush()

	fmt.Fprintf(w, "\n修复前误差: RMS %.3f ms, MAX %.3f ms\n", r.Before.RMSMS, r.Before.MaxMS)
	fmt.Fprintf(w, "逐段修复后误差: RMS %.3f ms, MAX %.3f ms\n", r.After.RMSMS, r.After.MaxMS)
	fmt.Fprintf(w, "单一全局偏移/漂移对照: RMS %.3f ms, MAX %.3f ms（仅用于验证不能使用全局常数）\n",
		r.GlobalAfter.RMSMS, r.GlobalAfter.MaxMS)

	if r.FirstAnomaly == nil {
		fmt.Fprintln(w, "首个异常源: 无（所有段均在容差内）")
		return
	}
	a := r.FirstAnomaly
	fmt.Fprintf(w, "首个异常源: %s（段 %s，起点 %.3f ms）\n",
		a.Source, a.SegmentID, a.SegmentStartMS)
	if a.FirstBadMS != nil {
		fmt.Fprintf(w, "首次超差样本: %.3f ms\n", *a.FirstBadMS)
	}
	fmt.Fprintf(w, "原因: %s\n", strings.Join(a.Reasons, ", "))
}
