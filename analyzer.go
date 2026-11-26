package main

import (
	"math"
	"sort"
	"time"
)

const (
	defaultWindowSize = 4096
	minWindowSize     = 1024
	hopDivisor        = 2
	silenceFloorDBFS  = -70.0
	noiseFloorDBFS    = -80.0
)

type FrameMetrics struct {
	Index             int                `json:"index"`
	StartSample       int                `json:"start_sample"`
	SampleRate        int                `json:"-"`
	WindowSize        int                `json:"window_size"`
	StartTime         time.Duration      `json:"-"`
	StartTimeSeconds  float64            `json:"start_seconds"`
	LeftRMSDB         float64            `json:"left_rms_dbfs"`
	RightRMSDB        float64            `json:"right_rms_dbfs"`
	Correlation       float64            `json:"correlation"`
	MonoRMSDB         float64            `json:"mono_rms_dbfs"`
	CancellationDB    float64            `json:"cancellation_db"`
	SideMidRatioDB    float64            `json:"side_mid_ratio_db"`
	VocalPhaseRadians float64            `json:"vocal_phase_radians"`
	VocalCoherence    float64            `json:"vocal_coherence"`
	VocalCancellation float64            `json:"vocal_cancellation"`
	Coherence         map[string]float64 `json:"-"`
	VocalWeight       float64            `json:"-"`
	Bands             []BandMetrics      `json:"bands"`
}

type BandMetrics struct {
	Name             string  `json:"name"`
	LowHz            float64 `json:"low_hz"`
	HighHz           float64 `json:"high_hz"`
	Coherence        float64 `json:"coherence"`
	WeightedPhase    float64 `json:"weighted_phase_radians"`
	Cancellation     float64 `json:"cancellation"`
	MagnitudeBalance float64 `json:"magnitude_balance"`
	VocalBand        bool    `json:"vocal_band"`
}

type Segment struct {
	Start            time.Duration `json:"-"`
	End              time.Duration `json:"-"`
	Frames           int           `json:"frames"`
	Severity         float64       `json:"severity_score"`
	WorstFrame       FrameMetrics  `json:"worst_frame"`
	MeanCancellation float64
	MeanCorrelation  float64
	MeanPhase        float64
	MeanSideMid      float64
	Evidence         Evidence
}

type Evidence struct {
	LeftDBFS         float64            `json:"left_dbfs"`
	RightDBFS        float64            `json:"right_dbfs"`
	BalanceDB        float64            `json:"balance_db"`
	MonoDBFS         float64            `json:"mono_dbfs"`
	CancellationDB   float64            `json:"cancellation_db"`
	Correlation      float64            `json:"correlation"`
	SideMidRatioDB   float64            `json:"side_mid_ratio_db"`
	VocalPhaseDeg    float64            `json:"vocal_phase_degrees"`
	VocalCoherence   float64            `json:"vocal_coherence"`
	BandCancellation map[string]float64 `json:"band_cancellation"`
}

type Report struct {
	SampleRate   int            `json:"sample_rate"`
	Channels     int            `json:"channels"`
	TotalSamples int            `json:"total_samples_per_channel"`
	Duration     time.Duration  `json:"-"`
	WindowSize   int            `json:"window_size"`
	HopSize      int            `json:"hop_size"`
	Frames       int            `json:"analyzed_frames"`
	Severity     string         `json:"severity"`
	Summary      string         `json:"summary"`
	Segments     []Segment      `json:"segments"`
	FrameMetrics []FrameMetrics `json:"-"`
}

type options struct {
	windowSize int
	hopSize    int
}

func analyze(left, right []float64, sampleRate int, opts options) Report {
	if sampleRate <= 0 || len(left) != len(right) || len(left) == 0 {
		return Report{SampleRate: sampleRate, Channels: 2, TotalSamples: len(left), Severity: "unknown", Summary: "无效或非等长立体声音频"}
	}
	if opts.windowSize <= 0 {
		opts.windowSize = chooseWindowSize(sampleRate)
	}
	if opts.hopSize <= 0 {
		opts.hopSize = opts.windowSize / hopDivisor
	}
	if opts.windowSize < minWindowSize || opts.windowSize&(opts.windowSize-1) != 0 || opts.hopSize <= 0 || opts.hopSize > opts.windowSize {
		return Report{SampleRate: sampleRate, Channels: 2, TotalSamples: len(left), Severity: "unknown", Summary: "FFT 窗长必须是不小于 1024 的 2 的幂，hop 必须位于 1 到窗长之间"}
	}

	frames := collectFrames(left, right, sampleRate, opts)
	segments := findSegments(frames)
	severity, summary := classify(segments, frames)

	return Report{
		SampleRate:   sampleRate,
		Channels:     2,
		TotalSamples: len(left),
		Duration:     time.Duration(float64(len(left)) / float64(sampleRate) * float64(time.Second)),
		WindowSize:   opts.windowSize,
		HopSize:      opts.hopSize,
		Frames:       len(frames),
		Severity:     severity,
		Summary:      summary,
		Segments:     segments,
		FrameMetrics: frames,
	}
}

func chooseWindowSize(sampleRate int) int {
	target := sampleRate / 10
	if target < defaultWindowSize {
		target = defaultWindowSize
	}
	size := minWindowSize
	for size < target {
		size *= 2
	}
	return size
}

func collectFrames(left, right []float64, sampleRate int, opts options) []FrameMetrics {
	var frames []FrameMetrics
	for start, index := 0, 0; start+opts.windowSize <= len(left); start, index = start+opts.hopSize, index+1 {
		frame := measureFrame(left[start:start+opts.windowSize], right[start:start+opts.windowSize], sampleRate, index, start)
		if frame.LeftRMSDB > silenceFloorDBFS && frame.RightRMSDB > silenceFloorDBFS {
			frames = append(frames, frame)
		}
	}
	if len(frames) == 0 && len(left) >= minWindowSize/2 {
		frame := measurePaddedFrame(left, right, sampleRate, opts.windowSize)
		if frame.LeftRMSDB > silenceFloorDBFS && frame.RightRMSDB > silenceFloorDBFS {
			frames = append(frames, frame)
		}
	}
	return frames
}

func findSegments(frames []FrameMetrics) []Segment {
	var segments []Segment
	var active []FrameMetrics

	flush := func() {
		if len(active) == 0 {
			return
		}
		segments = append(segments, buildSegment(active))
		active = nil
	}

	for _, frame := range frames {
		if isCritical(frame) {
			if len(active) > 0 && frame.Index-active[len(active)-1].Index > 2 {
				flush()
			}
			active = append(active, frame)
			continue
		}
		flush()
	}
	flush()

	sort.Slice(segments, func(i, j int) bool {
		return segments[i].Severity > segments[j].Severity
	})
	if len(segments) > 5 {
		segments = segments[:5]
	}
	return segments
}

func isCritical(frame FrameMetrics) bool {
	return frame.VocalWeight > 0 &&
		frame.CancellationDB <= -12 &&
		frame.VocalCancellation >= 0.65 &&
		frame.Correlation <= -0.25 &&
		frame.VocalCoherence >= 0.45 &&
		math.Abs(wrapPhase(frame.VocalPhaseRadians)) >= 2.35 &&
		frame.SideMidRatioDB >= 4
}

func classify(segments []Segment, frames []FrameMetrics) (string, string) {
	if len(frames) == 0 {
		return "unknown", "没有足够的非静音立体声帧可供判断"
	}
	if len(segments) == 0 {
		return "ok", "未发现会导致单声道人声/中置内容消失的反相抵消；宽声场差异未判为故障"
	}
	worst := segments[0]
	return worst.SeverityLevel(), "最严重时段左右声道仍有电平，但单声道和人声频段能量大幅抵消，具备反相证据"
}

func (s Segment) SeverityLevel() string {
	switch {
	case s.Severity >= 0.55:
		return "critical"
	case s.Severity >= 0.40:
		return "warning"
	default:
		return "notice"
	}
}
