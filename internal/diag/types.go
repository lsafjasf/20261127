package diag

import (
	"seekdiag/internal/clock"
	"seekdiag/internal/seek"
)

const (
	VerdictContainer    = "container"
	VerdictDecoder      = "decoder"
	VerdictClock        = "playback_clock"
	VerdictOK           = "no_fault_detected"
	VerdictInconclusive = "inconclusive"
	VerdictFileError    = "container"
)

type Options struct {
	MinDriftMS    float64 `json:"min_drift_ms"`
	MinDriftMSps  float64 `json:"min_drift_ms_per_s"`
	BoundaryTolMS float64 `json:"boundary_tolerance_ms"`
}

func DefaultOptions() Options {
	return Options{MinDriftMS: 10, MinDriftMSps: 0.1, BoundaryTolMS: 2}
}

type Observation struct {
	Label             string   `json:"label,omitempty"`
	WallMS            float64  `json:"wall_ms"`
	AudioPTSMS        *float64 `json:"audio_pts_ms,omitempty"`
	DecodedAudioPTSMS *float64 `json:"decoded_audio_pts_ms,omitempty"`
	AudioSampleIndex  *int64   `json:"audio_sample_index,omitempty"`
	AudioByteOffset   *int64   `json:"audio_byte_offset,omitempty"`
	AudioPCMOffset    *int64   `json:"audio_pcm_offset,omitempty"`
	VideoPTSMS        *float64 `json:"video_pts_ms,omitempty"`
	PlaybackClockMS   *float64 `json:"playback_clock_ms,omitempty"`
}

type ObservationSet struct {
	SeekMS float64       `json:"seek_ms,omitempty"`
	Before []Observation `json:"before,omitempty"`
	After  []Observation `json:"after"`
}

type Finding struct {
	Layer    string  `json:"layer"`
	Code     string  `json:"code"`
	Message  string  `json:"message"`
	ValueMS  float64 `json:"value_ms,omitempty"`
	Severity string  `json:"severity"`
}

type EditView struct {
	Index           int     `json:"index"`
	SegmentDuration uint64  `json:"segment_duration"`
	MediaTime       int64   `json:"media_time"`
	MediaRate       float64 `json:"media_rate"`
	Empty           bool    `json:"empty"`
	MovieStartMS    float64 `json:"movie_start_ms"`
	MovieEndMS      float64 `json:"movie_end_ms"`
	MediaStartMS    float64 `json:"media_start_ms,omitempty"`
	MediaEndMS      float64 `json:"media_end_ms,omitempty"`
}

type TrackView struct {
	TrackID       uint32         `json:"track_id"`
	Kind          string         `json:"kind"`
	Codec         string         `json:"codec"`
	Timescale     uint32         `json:"timescale"`
	DurationMS    float64        `json:"duration_ms"`
	SampleRate    uint32         `json:"sample_rate,omitempty"`
	Channels      uint16         `json:"channels,omitempty"`
	SampleCount   int            `json:"sample_count"`
	EditListFound bool           `json:"edit_list_found"`
	Edits         []EditView     `json:"edit_list"`
	FirstSample   *seek.Position `json:"first_sample,omitempty"`
}

type ProbeView struct {
	Observation
	ExpectedAudioPTSMS *float64 `json:"expected_audio_pts_ms,omitempty"`
	ContainerErrorMS   *float64 `json:"container_error_ms,omitempty"`
	DecoderErrorMS     *float64 `json:"decoder_error_ms,omitempty"`
	ClockErrorMS       *float64 `json:"clock_error_ms,omitempty"`
}

type DriftView struct {
	Layer     string  `json:"layer"`
	StartMS   float64 `json:"start_ms"`
	EndMS     float64 `json:"end_ms"`
	DeltaMS   float64 `json:"delta_ms"`
	SlopeMSps float64 `json:"slope_ms_per_s"`
	SpanMS    float64 `json:"span_ms"`
	Samples   int     `json:"samples"`
}

type ObservationReport struct {
	Before []ProbeView `json:"before,omitempty"`
	After  []ProbeView `json:"after,omitempty"`
	Drift  []DriftView `json:"drift,omitempty"`
}

type Result struct {
	File          string             `json:"file"`
	SeekMS        float64            `json:"seek_ms"`
	Verdict       string             `json:"verdict"`
	Confidence    string             `json:"confidence"`
	Summary       string             `json:"summary"`
	Findings      []Finding          `json:"findings"`
	Tracks        []TrackView        `json:"tracks"`
	SeekPlan      *seek.Plan         `json:"seek_plan"`
	SeekAnchor    *clock.SeekAnchor  `json:"seek_anchor,omitempty"`
	Observations  *ObservationReport `json:"observations,omitempty"`
	ResyncWarning string             `json:"resync_warning"`
}
