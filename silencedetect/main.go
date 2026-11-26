package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

func main() {
	cfg := DefaultConfig()
	var (
		flagNoise   = flag.Float64("noise-db", cfg.NoiseDB, "noise-floor threshold in dBFS")
		flagPause   = flag.Float64("pause-db", cfg.PauseDB, "pause threshold in dBFS")
		flagWindow  = flag.Float64("window-ms", cfg.WindowMS, "analysis window size in ms")
		flagBridge  = flag.Float64("bridge-ms", cfg.BridgeMS, "max click/gap window bridged inside a quiet region")
		flagMinDur  = flag.Float64("min-ms", cfg.MinDurMS, "discard intervals shorter than this (ms)")
		flagMicro   = flag.Float64("micro-ms", cfg.MicroMS, "micro-gap upper bound (ms)")
		flagNatural = flag.Float64("natural-ms", cfg.NaturalMS, "acceptable natural-pause upper bound (ms)")
		flagLong    = flag.Float64("long-ms", cfg.LongMS, "long-pause upper bound; beyond it is suspect (ms)")
		flagRMS     = flag.Bool("rms", false, "gate on window RMS instead of peak")
		flagJSON    = flag.Bool("json", false, "emit machine-readable JSON")

		flagRate = flag.Int("raw-rate", 0, "raw PCM only: sample rate in Hz")
		flagCh   = flag.Int("raw-channels", 0, "raw PCM only: channel count")
		flagBits = flag.Int("raw-bits", 0, "raw PCM only: bits per sample (8/16/24/32)")
		flagFlt  = flag.Bool("raw-float", false, "raw PCM only: 32/64-bit IEEE float samples")
	)
	flag.Parse()

	cfg.NoiseDB = *flagNoise
	cfg.PauseDB = *flagPause
	cfg.WindowMS = *flagWindow
	cfg.BridgeMS = *flagBridge
	cfg.MinDurMS = *flagMinDur
	cfg.MicroMS = *flagMicro
	cfg.NaturalMS = *flagNatural
	cfg.LongMS = *flagLong
	cfg.UseRMS = *flagRMS

	if err := cfg.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, "invalid flags:", err)
		os.Exit(2)
	}

	args := flag.Args()
	if len(args) == 0 {
		usage()
		os.Exit(2)
	}

	reports := make([]fileReport, 0, len(args))

	for _, path := range args {
		a, err := load(path, *flagRate, *flagCh, *flagBits, *flagFlt)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
			os.Exit(1)
		}
		reports = append(reports, fileReport{
			File:      path,
			Audio:     a,
			Intervals: Detect(a, cfg),
		})
	}

	if *flagJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(reports); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	for i, r := range reports {
		if i > 0 {
			fmt.Println()
		}
		printReport(r.Audio, r.Intervals, cfg)
	}
	if len(reports) > 1 {
		printComparison(reports)
	}
}

func load(path string, rate, channels, bits int, isFloat bool) (*Audio, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if ext := filepath.Ext(path); ext == ".pcm" || ext == ".raw" {
		if rate <= 0 || channels <= 0 || bits <= 0 {
			return nil, fmt.Errorf("raw PCM requires -raw-rate, -raw-channels and -raw-bits")
		}
		return decodePCM(path, b, waveFmt{
			tag:      map[bool]uint16{true: 3, false: 1}[isFloat],
			channels: uint16(channels),
			rate:     uint32(rate),
			bits:     uint16(bits),
		})
	}
	return ParseWAV(path, b)
}

func usage() {
	fmt.Fprintf(os.Stderr, "usage: %s [flags] file.wav [file2.wav ...]\n", os.Args[0])
	fmt.Fprintln(os.Stderr, "Detects digital silence, low-level noise floor and pauses per channel.")
	flag.PrintDefaults()
}

func printReport(a *Audio, ivs []*Interval, cfg Config) {
	fmt.Printf("file: %s\n", a.Path)
	format := "PCM"
	if a.IsFloat {
		format = "IEEE float"
	}
	fmt.Printf("format: %d Hz, %d ch, %d-bit %s, %.3f s (%d frames)\n",
		a.SampleRate, a.Channels, a.Bits, format,
		float64(a.Frames)/float64(a.SampleRate), a.Frames)
	fmt.Printf("thresholds: digital=exact-zero, noise=%.0f dBFS, pause=%.0f dBFS; min=%.0f ms, window=%.0f ms, bridge<=%.0f ms\n",
		cfg.NoiseDB, cfg.PauseDB, cfg.MinDurMS, cfg.WindowMS, cfg.BridgeMS)
	fmt.Printf("pause policy: natural <= %.0f ms, review <= %.0f ms, suspect > %.0f ms\n\n",
		cfg.NaturalMS, cfg.LongMS, cfg.LongMS)

	hdr := "ch  level      start(s)    end(s)      dur(ms)   peak(dBFS)  rms(dBFS)  zero%   position    kind / verdict"
	fmt.Println(hdr)
	for _, iv := range ivs {
		fmt.Printf("%-2d  %-9s  %9.6f  %9.6f  %9.3f  %9s  %8s  %5.1f  %-10s %s / %s\n",
			iv.Channel, iv.Level, iv.StartSec, iv.EndSec, iv.DurationMS,
			dbLabel(float64(iv.PeakDB)), dbLabel(float64(iv.RMSDB)), iv.ZeroPct,
			iv.Position, iv.Kind, iv.Verdict)
	}
	printSummary(ivs)
}

func printSummary(ivs []*Interval) {
	counts := map[string]int{}
	var suspect int
	for _, iv := range ivs {
		if iv.Channel >= 0 {
			continue // summary only uses the program-level view
		}
		counts[iv.Level]++
		if iv.Verdict == VerdictSuspect {
			suspect++
		}
	}
	fmt.Printf("\nprogram-level summary: digital=%d, noise-floor=%d, pause=%d regions; suspect long pauses=%d\n",
		counts[LevelDigital], counts[LevelNoise], counts[LevelPause], suspect)
}

// printComparison aligns program-level intervals of the same level across
// files so a tens-of-ms length delta between two "identical" silences is
// immediately visible.
type fileReport struct {
	File      string      `json:"file"`
	Audio     *Audio      `json:"audio"`
	Intervals []*Interval `json:"intervals"`
}

func printComparison(reports []fileReport) {
	fmt.Println("\n=== cross-file comparison (program-level, by start order) ===")
	for _, level := range []string{LevelDigital, LevelNoise, LevelPause} {
		fmt.Printf("\n[%s]\n", level)
		maxN := 0
		perFile := make([][]*Interval, len(reports))
		for i, r := range reports {
			for _, iv := range r.Intervals {
				if iv.Channel == -1 && iv.Level == level {
					perFile[i] = append(perFile[i], iv)
				}
			}
			if len(perFile[i]) > maxN {
				maxN = len(perFile[i])
			}
		}
		for k := 0; k < maxN; k++ {
			fmt.Printf("region #%d\n", k+1)
			var base *Interval
			for i := range reports {
				if k >= len(perFile[i]) {
					fmt.Printf("  %s: <missing>\n", reports[i].File)
					continue
				}
				iv := perFile[i][k]
				delta := ""
				if base != nil {
					delta = fmt.Sprintf("  delta=%+9.3f ms vs %s",
						iv.DurationMS-base.DurationMS, base.Level)
				}
				fmt.Printf("  %s: start=%.6f s end=%.6f s dur=%9.3f ms%s\n",
					reports[i].File, iv.StartSec, iv.EndSec, iv.DurationMS, delta)
				if base == nil {
					base = iv
				}
			}
		}
	}
}

func dbLabel(v float64) string {
	if math.IsInf(v, -1) {
		return "-inf"
	}
	return fmt.Sprintf("%.1f", v)
}
