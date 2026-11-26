// Package click detects edit-point clicks in PCM audio while suppressing
// ordinary percussive attacks.
package click

// Options controls detection and report size.
type Options struct {
	WaveformBefore int
	WaveformAfter  int
	MinConfidence  float64
}

// DefaultOptions returns conservative defaults suitable for music material.
func DefaultOptions() Options {
	return Options{
		WaveformBefore: 12,
		WaveformAfter:  12,
		MinConfidence:  0.50,
	}
}

// Features contains the three requested click indicators plus the transient
// shape used to distinguish cuts from drum attacks. Scores marked Z are
// relative to a robust local baseline; raw values are normalized full-scale
// sample amplitudes.
type Features struct {
	JumpValue       float64 `json:"jump_value"`
	JumpRatio       float64 `json:"jump_vs_local_p99"`
	JumpZ           float64 `json:"jump_robust_z"`
	SlopeJumpZ      float64 `json:"slope_jump_robust_z"`
	PhaseJumpRad    float64 `json:"phase_jump_rad"`
	PhaseCoherence  float64 `json:"phase_coherence"`
	LowFreqStep     float64 `json:"low_freq_step"`
	LowFreqStepZ    float64 `json:"low_freq_step_robust_z"`
	LowFreqImpulse  float64 `json:"low_freq_impulse"`
	LowFreqImpulseZ float64 `json:"low_freq_impulse_robust_z"`
	BurstSamples    int     `json:"burst_samples"`
	PercussiveOnset bool    `json:"percussive_onset"`
}

// Waveform holds raw normalized samples around a detected click. Values are
// inclusive on both sides; ClickSample is the first sample after the splice.
type Waveform struct {
	FirstSample int         `json:"first_sample"`
	ClickSample int         `json:"click_sample"`
	Samples     [][]float64 `json:"samples"`
}

// Detection is one click event. Channels are zero-based.
type Detection struct {
	Sample     int      `json:"sample"`
	TimeSec    float64  `json:"time_sec"`
	Channels   []int    `json:"channels"`
	Confidence float64  `json:"confidence"`
	Flags      []string `json:"flags"`
	Features   Features `json:"features"`
	Waveform   Waveform `json:"waveform"`
}

// Suppressed records a transient rejected by the drum/percussion guard.
type Suppressed struct {
	Sample   int      `json:"sample"`
	TimeSec  float64  `json:"time_sec"`
	Channels []int    `json:"channels"`
	Reason   string   `json:"reason"`
	Features Features `json:"features"`
}

// Analysis is the complete detector result.
type Analysis struct {
	File            string       `json:"file,omitempty"`
	SampleRate      int          `json:"sample_rate"`
	Channels        int          `json:"channels"`
	Frames          int          `json:"frames"`
	DurationSec     float64      `json:"duration_sec"`
	Detections      []Detection  `json:"detections"`
	Suppressed      []Suppressed `json:"suppressed_transients"`
	SuppressedTotal int          `json:"suppressed_total"`
}

type featureSet struct {
	sample     int
	features   Features
	confidence float64
	suppressed bool
	reason     string
}
