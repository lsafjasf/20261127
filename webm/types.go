// Package webm inspects WebM/Matroska alpha, crop, and timing metadata.
package webm

import "alphacheck/preview"

// Report is the structured inspection result for one file.
type Report struct {
	Path                string          `json:"path"`
	FileSize            int64           `json:"file_size"`
	TimecodeScaleNS     uint64          `json:"timecode_scale_ns"`
	MuxingApp           string          `json:"muxing_app,omitempty"`
	WritingApp          string          `json:"writing_app,omitempty"`
	ContainerDurationNS int64           `json:"container_duration_ns,omitempty"`
	Tracks              []TrackReport   `json:"tracks"`
	Frames              []FrameReport   `json:"frames,omitempty"`
	Timing              TimingReport    `json:"timing"`
	Findings            []Finding       `json:"findings"`
	Verdict             Verdict         `json:"verdict"`
	Preview             *preview.Result `json:"preview,omitempty"`
}

// TrackReport contains track-level metadata and crop validation.
type TrackReport struct {
	TrackNumber       uint64      `json:"track_number"`
	TrackType         uint64      `json:"track_type"`
	CodecID           string      `json:"codec_id"`
	CodecName         string      `json:"codec_name"`
	AlphaMode         int         `json:"alpha_mode"`
	AlphaModePresent  bool        `json:"alpha_mode_present"`
	DefaultDurationNS uint64      `json:"default_duration_ns,omitempty"`
	PixelWidth        uint64      `json:"pixel_width,omitempty"`
	PixelHeight       uint64      `json:"pixel_height,omitempty"`
	DisplayWidth      uint64      `json:"display_width,omitempty"`
	DisplayHeight     uint64      `json:"display_height,omitempty"`
	DisplayUnit       uint64      `json:"display_unit,omitempty"`
	Crop              Crop        `json:"crop"`
	VisibleWidth      uint64      `json:"visible_width,omitempty"`
	VisibleHeight     uint64      `json:"visible_height,omitempty"`
	Bitstream         *Bitstream  `json:"bitstream,omitempty"`
	Alpha             AlphaReport `json:"alpha"`
	FrameCount        int         `json:"frame_count"`
}

// Crop stores Matroska PixelCrop edge values in encoded pixels.
type Crop struct {
	Left   uint64 `json:"left"`
	Right  uint64 `json:"right"`
	Top    uint64 `json:"top"`
	Bottom uint64 `json:"bottom"`
}

// Bitstream contains dimensions parsed from a codec keyframe header.
type Bitstream struct {
	Codec        string `json:"codec"`
	Profile      int    `json:"profile,omitempty"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	RenderWidth  int    `json:"render_width,omitempty"`
	RenderHeight int    `json:"render_height,omitempty"`
	Keyframe     bool   `json:"keyframe"`
}

// AlphaReport summarizes BlockAdditional alpha payloads.
type AlphaReport struct {
	FramesWithAdditional int      `json:"frames_with_block_additional"`
	AlphaAddIDOne        int      `json:"alpha_add_id_one"`
	OtherAddIDs          []uint64 `json:"other_add_ids,omitempty"`
	EmptyPayloads        int      `json:"empty_payloads"`
	ParsedHeaders        int      `json:"parsed_headers"`
	ConstantByte         *int     `json:"constant_byte,omitempty"`
}

// FrameReport is timing and alpha evidence for one block.
type FrameReport struct {
	Index           int     `json:"index"`
	TrackNumber     uint64  `json:"track_number"`
	PTSNS           int64   `json:"pts_ns"`
	OffsetMS        float64 `json:"offset_ms"`
	DurationNS      uint64  `json:"duration_ns,omitempty"`
	Keyframe        bool    `json:"keyframe"`
	PayloadBytes    int     `json:"payload_bytes"`
	AlphaPresent    bool    `json:"alpha_present"`
	AlphaAddID      uint64  `json:"alpha_add_id,omitempty"`
	AlphaBytes      int     `json:"alpha_bytes,omitempty"`
	BitstreamWidth  int     `json:"bitstream_width,omitempty"`
	BitstreamHeight int     `json:"bitstream_height,omitempty"`
	HeaderError     string  `json:"header_error,omitempty"`
}

// TimingReport summarizes PTS intervals and frame duration.
type TimingReport struct {
	FirstPTSNS         int64   `json:"first_pts_ns"`
	LastPTSNS          int64   `json:"last_pts_ns"`
	FrameSpanNS        int64   `json:"frame_span_ns"`
	MeanIntervalNS     float64 `json:"mean_interval_ns,omitempty"`
	MinIntervalNS      int64   `json:"min_interval_ns,omitempty"`
	MaxIntervalNS      int64   `json:"max_interval_ns,omitempty"`
	DefaultFrameRate   float64 `json:"default_frame_rate,omitempty"`
	IntervalCV         float64 `json:"interval_cv,omitempty"`
	NonMonotonic       int     `json:"non_monotonic"`
	Gaps               int     `json:"gaps"`
	Duplicates         int     `json:"duplicates"`
	IrregularIntervals int     `json:"irregular_intervals"`
}

// Finding is a classified, actionable inspection result.
type Finding struct {
	Severity string `json:"severity"`
	Layer    string `json:"layer"`
	Class    string `json:"class"`
	Code     string `json:"code"`
	Track    uint64 `json:"track,omitempty"`
	Frame    int    `json:"frame,omitempty"`
	Message  string `json:"message"`
	Evidence string `json:"evidence,omitempty"`
}

// Verdict gives the primary responsibility category.
type Verdict struct {
	Status           string         `json:"status"`
	PrimaryClass     string         `json:"primary_class,omitempty"`
	ResponsibilityZH string         `json:"responsibility_zh,omitempty"`
	ErrorCounts      map[string]int `json:"error_counts"`
}
