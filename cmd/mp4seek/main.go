package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	mp4seek "github.com/lsafjasf/20261127"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "scan":
		err = runScan(os.Args[2:])
	case "repair":
		err = runRepair(os.Args[2:])
	case "seek":
		err = runSeek(os.Args[2:])
	case "-h", "--help", "help":
		usage()
		return
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "mp4seek:", err)
		os.Exit(1)
	}
}

func runScan(args []string) error {
	flags := flag.NewFlagSet("scan", flag.ContinueOnError)
	jsonOut := flags.Bool("json", false, "emit JSON")
	samples := flags.Bool("samples", false, "print every sample dependency")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return fmt.Errorf("usage: mp4seek scan [-json] [-samples] FILE")
	}
	report, err := mp4seek.ScanFile(flags.Arg(0))
	if err != nil {
		return err
	}
	if *jsonOut {
		return printJSON(report)
	}
	printReport(report, *samples)
	return nil
}

func runRepair(args []string) error {
	flags := flag.NewFlagSet("repair", flag.ContinueOnError)
	jsonOut := flags.Bool("json", false, "emit JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 2 {
		return fmt.Errorf("usage: mp4seek repair [-json] INPUT OUTPUT")
	}
	report, err := mp4seek.RepairFile(flags.Arg(0), flags.Arg(1))
	if err != nil {
		return err
	}
	if *jsonOut {
		return printJSON(report)
	}
	fmt.Printf("input:  %s (%d bytes)\n", report.Input, report.OriginalSize)
	fmt.Printf("output: %s (%d bytes)\n", report.Output, report.RepairedSize)
	fmt.Printf("changed=%t payload_verified=%t moov_delta=%d chunk_offset_delta=%d\n",
		report.Changed, report.PayloadOK, report.MoovSizeDelta, report.OffsetDelta)
	for _, track := range report.Tracks {
		fmt.Printf("track %d: added=%v removed=%v\n", track.ID, track.Added, track.Removed)
	}
	return nil
}

func runSeek(args []string) error {
	flags := flag.NewFlagSet("seek", flag.ContinueOnError)
	jsonOut := flags.Bool("json", false, "emit JSON")
	trackID := flags.Uint("track", 0, "track ID (0 means every video track)")
	randomCount := flags.Int("random", 0, "generate this many random seek targets")
	seed := flags.Int64("seed", 1, "random seed")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() < 1 {
		return fmt.Errorf("usage: mp4seek seek [-json] [-track ID] FILE [TIME ...]")
	}
	file, err := mp4seek.ParseFile(flags.Arg(0))
	if err != nil {
		return err
	}
	var targets []int64
	for _, raw := range flags.Args()[1:] {
		target, err := parseTime(raw)
		if err != nil {
			return fmt.Errorf("target %q: %w", raw, err)
		}
		targets = append(targets, target)
	}
	if *randomCount > 0 {
		maxUS := fileDurationUS(file)
		rng := rand.New(rand.NewSource(*seed))
		for i := 0; i < *randomCount && maxUS > 0; i++ {
			targets = append(targets, rng.Int63n(maxUS))
		}
	}
	if len(targets) == 0 {
		return fmt.Errorf("provide at least one TIME or -random N")
	}
	var results []mp4seek.SeekResult
	for _, target := range targets {
		found, err := file.SeekResults(target, uint32(*trackID))
		if err != nil {
			return err
		}
		results = append(results, found...)
	}
	if *jsonOut {
		return printJSON(results)
	}
	for _, result := range results {
		fmt.Printf("track=%d target=%s requested=#%d indexed=#%d@%d actual=#%d exact=%t false_success=%t\n",
			result.TrackID, formatUS(result.TargetUS), result.RequestedSample,
			result.IndexedSample, int64(result.IndexedOffset), result.ActualSample,
			result.Exact, result.FalseSuccess)
	}
	return nil
}

func fileDurationUS(file *mp4seek.File) int64 {
	var max int64
	for _, track := range file.Tracks {
		if track.Handler != "vide" || track.Timescale == 0 {
			continue
		}
		duration := int64(track.Duration) * 1_000_000 / int64(track.Timescale)
		if duration > max {
			max = duration
		}
	}
	return max
}

func printReport(report *mp4seek.Report, allSamples bool) {
	fmt.Printf("file: %s (%d bytes)\n", report.Path, report.Size)
	for _, track := range report.Tracks {
		fmt.Printf("track %d: handler=%s codec=%s samples=%d indexed_sync=%d actual_sync=%d scan_complete=%t\n",
			track.ID, track.Handler, track.Codec, track.SampleCount,
			len(track.IndexedSync), len(track.ActualSync), track.ScanComplete)
	}
	fmt.Println("GOP starts:")
	for _, gop := range report.GOPs {
		marker := "indexed"
		if !gop.Indexed {
			marker = "MISSING-FROM-INDEX"
		}
		fmt.Printf("  track=%d sample=%d pts=%s offset=%d size=%d kind=%s %s\n",
			gop.TrackID, gop.StartSample, formatUS(gop.StartPTSUS), gop.Offset, gop.Size, gop.Kind, marker)
	}
	fmt.Println("seek hit offsets:")
	for _, hit := range report.SeekHits {
		fmt.Printf("  track=%d source=%s sample=%d pts=%s offset=%d actual_rap=%t\n",
			hit.TrackID, hit.Source, hit.Sample, formatUS(hit.PTSUS), hit.Offset, hit.ActualRandomAccess)
	}
	fmt.Println("broken index ranges (old stss cannot land on the real GOP):")
	if len(report.BrokenRanges) == 0 {
		fmt.Println("  none")
	}
	for _, span := range report.BrokenRanges {
		fmt.Printf("  track=%d %s - %s reason=%s\n",
			span.TrackID, formatUS(span.StartUS), formatUS(span.EndUS), span.Reason)
	}
	fmt.Println("ranges that cannot land exactly on a random access point:")
	for _, span := range report.InexactRanges {
		fmt.Printf("  track=%d %s - %s reason=%s\n",
			span.TrackID, formatUS(span.StartUS), formatUS(span.EndUS), span.Reason)
	}
	if allSamples {
		fmt.Println("sample dependencies:")
		for _, track := range report.Tracks {
			if track.Handler != "vide" {
				continue
			}
			for _, sample := range track.Samples {
				fmt.Printf("  track=%d sample=%d pts=%s kind=%s dep=%s nals=%v\n",
					track.ID, sample.Number, formatUS(sample.PTSMicros(track.Timescale)+track.EditOffsetUS),
					sample.Kind, sample.Dependency, sample.NALTypes)
			}
		}
	}
	for _, warning := range report.Warnings {
		fmt.Fprintln(os.Stderr, "warning:", warning)
	}
}

func parseTime(raw string) (int64, error) {
	if strings.Contains(raw, ":") {
		parts := strings.Split(raw, ":")
		if len(parts) != 3 {
			return 0, fmt.Errorf("want HH:MM:SS.mmm")
		}
		hours, err := strconv.ParseFloat(parts[0], 64)
		if err != nil {
			return 0, err
		}
		minutes, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			return 0, err
		}
		seconds, err := strconv.ParseFloat(parts[2], 64)
		if err != nil {
			return 0, err
		}
		return int64((hours*3600 + minutes*60 + seconds) * 1_000_000), nil
	}
	if duration, err := time.ParseDuration(raw); err == nil {
		return duration.Microseconds(), nil
	}
	seconds, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, err
	}
	return int64(seconds * 1_000_000), nil
}

func formatUS(us int64) string {
	return (time.Duration(us) * time.Microsecond).String()
}

func printJSON(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  mp4seek scan [-json] [-samples] FILE
  mp4seek repair [-json] INPUT OUTPUT
  mp4seek seek [-json] [-track ID] [-random N] [-seed N] FILE [TIME ...]`)
}
