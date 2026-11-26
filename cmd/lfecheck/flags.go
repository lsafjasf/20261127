package main

import (
	"flag"
	"fmt"
	"strings"
	"time"

	"lfecheck/internal/audio"
)

type commonOptions struct {
	dialogue          string
	source            string
	low               float64
	high              float64
	window            time.Duration
	hop               time.Duration
	maxLag            time.Duration
	minCorrelation    float64
	minDuration       time.Duration
	mergeGap          time.Duration
	lowFloor          float64
	dialogueGate      float64
	leakBelowDialogue float64
	leakMargin        float64
}

func addCommonFlags(fs *flag.FlagSet, opts *commonOptions) {
	fs.StringVar(&opts.dialogue, "dialogue", "FC", "dialogue channel name or zero-based index")
	fs.StringVar(&opts.source, "source", "auto", "source channel name/index, or auto")
	fs.Float64Var(&opts.low, "low", 20, "low-band lower cutoff in Hz")
	fs.Float64Var(&opts.high, "high", 120, "low-band upper cutoff in Hz")
	fs.DurationVar(&opts.window, "window", 100*time.Millisecond, "analysis window")
	fs.DurationVar(&opts.hop, "hop", 20*time.Millisecond, "analysis hop")
	fs.DurationVar(&opts.maxLag, "max-lag", 20*time.Millisecond, "maximum source/dialogue lag")
	fs.Float64Var(&opts.minCorrelation, "min-corr", 0.55, "minimum absolute frame correlation")
	fs.DurationVar(&opts.minDuration, "min-duration", 120*time.Millisecond, "minimum leak interval")
	fs.DurationVar(&opts.mergeGap, "merge-gap", 80*time.Millisecond, "merge intervals separated by less than this")
	fs.Float64Var(&opts.lowFloor, "low-floor", -60, "low-band frame floor in dBFS")
	fs.Float64Var(&opts.dialogueGate, "dialogue-gate", -60, "center dialogue gate in dBFS")
	fs.Float64Var(&opts.leakMargin, "leak-margin", 3, "required low-band margin above the visibility floor")
	fs.Float64Var(&opts.leakBelowDialogue, "leak-below-dialogue", 30,
		"maximum leak level below center dialogue in dB")
}

func (opts commonOptions) config(b *audio.Buffer) (audio.AnalysisConfig, error) {
	cfg := audio.DefaultAnalysisConfig()
	dialogue, err := audio.ChannelIndex(b.Format, opts.dialogue)
	if err != nil {
		return cfg, fmt.Errorf("dialogue: %w", err)
	}
	cfg.DialogueChannel = dialogue
	if strings.EqualFold(opts.source, "auto") || opts.source == "" {
		cfg.SourceChannel = -1
	} else {
		source, err := audio.ChannelIndex(b.Format, opts.source)
		if err != nil {
			return cfg, fmt.Errorf("source: %w", err)
		}
		cfg.SourceChannel = source
	}
	cfg.LowCutoffHz = opts.low
	cfg.HighCutoffHz = opts.high
	cfg.WindowSec = opts.window.Seconds()
	cfg.HopSec = opts.hop.Seconds()
	cfg.MaxLagSec = opts.maxLag.Seconds()
	cfg.MinCorrelation = opts.minCorrelation
	cfg.MinDurationSec = opts.minDuration.Seconds()
	cfg.MergeGapSec = opts.mergeGap.Seconds()
	cfg.LowFloorDBFS = opts.lowFloor
	cfg.DialogueGateDB = opts.dialogueGate
	cfg.LeakMarginDB = opts.leakMargin
	cfg.MaxLeakBelowDialogueDB = opts.leakBelowDialogue
	if cfg.HighCutoffHz <= cfg.LowCutoffHz {
		return cfg, fmt.Errorf("--high must exceed --low")
	}
	return cfg, nil
}
