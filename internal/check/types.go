package check

import (
	"proalpha/internal/imagex"
	"proalpha/internal/mediameta"
)

type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

type WrongParam struct {
	File            string   `json:"file"`
	Frame           *int     `json:"frame,omitempty"`
	Code            string   `json:"code"`
	Severity        Severity `json:"severity"`
	Parameter       string   `json:"parameter"`
	Expected        string   `json:"expected,omitempty"`
	Actual          string   `json:"actual,omitempty"`
	Message         string   `json:"message"`
	FFmpegArguments []string `json:"ffmpeg_arguments,omitempty"`
	RepairAction    string   `json:"repair_action,omitempty"`
}

type FrameMetrics struct {
	Pixels          int     `json:"pixels"`
	RMeanAbsError   float64 `json:"r_mean_abs_error"`
	GMeanAbsError   float64 `json:"g_mean_abs_error"`
	BMeanAbsError   float64 `json:"b_mean_abs_error"`
	AMeanAbsError   float64 `json:"a_mean_abs_error"`
	RGBMeanAbsError float64 `json:"rgb_mean_abs_error"`
}

type ModelComparison struct {
	StraightRGBError   float64 `json:"straight_rgb_error"`
	PremultipliedError float64 `json:"premultiplied_error"`
	AlphaError         float64 `json:"alpha_error"`
	Winner             string  `json:"winner"`
}

type FrameReport struct {
	Index             int                `json:"index"`
	Width             int                `json:"width"`
	Height            int                `json:"height"`
	SourceAlpha       *imagex.AlphaStats `json:"source_alpha,omitempty"`
	VariantAlpha      imagex.AlphaStats  `json:"variant_alpha"`
	VariantModel      imagex.AlphaModel  `json:"variant_model"`
	Metrics           *FrameMetrics      `json:"metrics,omitempty"`
	Models            *ModelComparison   `json:"models,omitempty"`
	AttachSourceAlpha bool               `json:"attach_source_alpha"`
	Unpremultiply     bool               `json:"unpremultiply"`
	RepairConfidence  string             `json:"repair_confidence"`
	Outputs           map[string]string  `json:"outputs,omitempty"`
	WrongParameters   []WrongParam       `json:"wrong_parameters,omitempty"`
}

type FileReport struct {
	Path            string             `json:"path"`
	Codec           string             `json:"codec,omitempty"`
	Profile         string             `json:"profile,omitempty"`
	PixelFormat     string             `json:"pixel_format,omitempty"`
	Width           int                `json:"width,omitempty"`
	Height          int                `json:"height,omitempty"`
	ColorPrimaries  string             `json:"color_primaries,omitempty"`
	ColorTransfer   string             `json:"color_transfer,omitempty"`
	ColorMatrix     string             `json:"color_matrix,omitempty"`
	ColorRange      string             `json:"color_range,omitempty"`
	ProResAlphaType *int               `json:"prores_alpha_type,omitempty"`
	AlphaStats      *imagex.AlphaStats `json:"alpha_stats,omitempty"`
	AlphaModel      imagex.AlphaModel  `json:"alpha_model,omitempty"`
	MOVMetadata     *mediameta.MOVInfo `json:"mov_metadata,omitempty"`
}

type VariantReport struct {
	File   FileReport    `json:"file"`
	Frames []FrameReport `json:"frames"`
}

type Report struct {
	SchemaVersion   string          `json:"schema_version"`
	Status          string          `json:"status"`
	Summary         string          `json:"summary"`
	Source          *FileReport     `json:"source,omitempty"`
	Variants        []VariantReport `json:"variants"`
	WrongParameters []WrongParam    `json:"wrong_parameters"`
	OutputDirectory string          `json:"output_directory,omitempty"`
	RepairedVideo   string          `json:"repaired_video,omitempty"`
	RepairScript    string          `json:"repair_script,omitempty"`
}
