package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"ripplecheck/ripple"
)

type report struct {
	Seed       int64             `json:"seed"`
	Iterations int               `json:"iterations"`
	Insert     *ripple.RunResult `json:"insert"`
	Overwrite  *ripple.RunResult `json:"overwrite"`
}

func main() {
	seed := flag.Int64("seed", 20261127, "deterministic random seed")
	count := flag.Int("n", 20, "number of random drag operations")
	out := flag.String("out", "artifacts/ripple-audit.json", "JSON trace output path")
	flag.Parse()

	insertRun, err := ripple.RunRandom(int64(*seed), *count, ripple.ModeInsert, ripple.MarkerPolicyRipple)
	if err != nil {
		fatalf("insert run failed: %v", err)
	}
	overwriteRun, err := ripple.RunRandom(int64(*seed), *count, ripple.ModeOverwrite, ripple.MarkerPolicyRipple)
	if err != nil {
		fatalf("overwrite run failed: %v", err)
	}

	combined := report{Seed: int64(*seed), Iterations: *count, Insert: insertRun, Overwrite: overwriteRun}
	if *out != "" {
		if err := writeJSON(*out, combined); err != nil {
			fatalf("write trace: %v", err)
		}
	}

	printOperationTable(insertRun, overwriteRun)
	printMarkerSummary("INSERT", insertRun)
	printMarkerSummary("OVERWRITE", overwriteRun)
	printVerification(insertRun, overwriteRun, *out)

	if !insertRun.Verification.OK || !overwriteRun.Verification.OK {
		os.Exit(1)
	}
}

func writeJSON(path string, value any) error {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}

func printOperationTable(insertRun, overwriteRun *ripple.RunResult) {
	fmt.Printf("固定种子 %d；同一组 %d 次随机拖放分别走 insert / overwrite\n\n", insertRun.Seed, len(insertRun.Drags))
	writer := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(writer, "#\t拖入位置\t素材时长\t轨道\tinsert 后总时长\toverwrite 后总时长")
	for i, drag := range insertRun.Drags {
		fmt.Fprintf(writer, "%d\t%s\t%s\t%s\t%s\t%s\n",
			i+1,
			drag.At,
			drag.Duration,
			drag.TargetTrackID,
			insertRun.Audit.Records[i].AfterDuration,
			overwriteRun.Audit.Records[i].AfterDuration,
		)
	}
	writer.Flush()
	fmt.Println()
}

func printMarkerSummary(label string, run *ripple.RunResult) {
	fmt.Printf("[%s] 初始时长=%s 最终时长=%s 台账=%s\n",
		label, run.Audit.InitialDuration, run.Timeline.Duration(), run.Verification.DurationFromRecords)

	writer := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(writer, "标记\t初始\t最终\t轨迹点\t末次原因")
	for _, marker := range run.Timeline.Markers {
		trace := run.Audit.MarkerTraces[marker.ID]
		initial := trace.Points[0].NewTime
		last := trace.Points[len(trace.Points)-1]
		fmt.Fprintf(writer, "%s\t%s\t%s\t%d\t%s\n",
			marker.ID, initial, marker.Time, len(trace.Points), last.Reason)
	}
	writer.Flush()

	if len(run.SyncIssues) > 0 {
		fmt.Printf("联动检查：%d 条声画组存在片段级差异（单轨 overwrite/编辑可能是有意 split-edit）\n", len(run.SyncIssues))
	} else {
		fmt.Println("联动检查：所有 linked group 的轨道片段签名一致")
	}
	fmt.Println()
}

func printVerification(insertRun, overwriteRun *ripple.RunResult, out string) {
	runs := []struct {
		label string
		run   *ripple.RunResult
	}{
		{label: "insert", run: insertRun},
		{label: "overwrite", run: overwriteRun},
	}
	for _, item := range runs {
		label, run := item.label, item.run
		verification := run.Verification
		fmt.Printf("校验 %-9s ok=%v duration=%s markers=%d historyPoints=%d errors=%d\n",
			label, verification.OK, verification.DurationActual,
			verification.MarkersChecked, verification.MarkerHistoryPoints, len(verification.Errors))
		for _, issue := range verification.Errors {
			fmt.Printf("  - %s\n", issue)
		}
	}
	if out != "" {
		fmt.Printf("\n完整逐项轨迹已写入 %s\n", out)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
