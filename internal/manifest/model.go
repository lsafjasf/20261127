package manifest

const (
	OutputSchema     = "hdr-sdr-delivery-manifest/v1"
	StatusComplete   = "complete"
	StatusIncomplete = "incomplete"
)

type Report struct {
	Schema      string         `json:"schema"`
	GeneratedAt string         `json:"generated_at"`
	Generator   GeneratorInfo  `json:"generator"`
	Project     ProjectConfig  `json:"project"`
	Status      string         `json:"status"`
	Complete    bool           `json:"complete"`
	Summary     Summary        `json:"summary"`
	Shared      SharedReport   `json:"shared"`
	Versions    VersionsReport `json:"versions"`
	Failures    []Failure      `json:"failures"`
}

type GeneratorInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type Summary struct {
	TotalChecks  int    `json:"total_checks"`
	PassedChecks int    `json:"passed_checks"`
	FailedChecks int    `json:"failed_checks"`
	HDRStatus    string `json:"hdr_status"`
	SDRStatus    string `json:"sdr_status"`
	SharedStatus string `json:"shared_status"`
}

type VersionsReport struct {
	HDR VersionReport `json:"hdr"`
	SDR VersionReport `json:"sdr"`
}

type VersionReport struct {
	Role           string         `json:"role"`
	Label          string         `json:"label"`
	Path           string         `json:"path"`
	Status         string         `json:"status"`
	FileSHA256     string         `json:"file_sha256,omitempty"`
	Video          VideoInfo      `json:"video"`
	Color          ColorReport    `json:"color"`
	Cuts           CutsReport     `json:"cuts"`
	Defaults       DefaultsReport `json:"default_tracks"`
	EmbeddedTracks []TrackSummary `json:"embedded_tracks"`
	Checks         []Check        `json:"checks"`
}

type VideoInfo struct {
	Index               int     `json:"index"`
	Codec               string  `json:"codec,omitempty"`
	Profile             string  `json:"profile,omitempty"`
	Width               int     `json:"width,omitempty"`
	Height              int     `json:"height,omitempty"`
	PixelFormat         string  `json:"pixel_format,omitempty"`
	DurationMS          int64   `json:"duration_ms"`
	DurationAvailable   bool    `json:"duration_available"`
	DurationSource      string  `json:"duration_source,omitempty"`
	StartMS             int64   `json:"start_ms"`
	StartAvailable      bool    `json:"start_available"`
	FrameRate           float64 `json:"frame_rate,omitempty"`
	FrameRateAvailable  bool    `json:"frame_rate_available"`
	FrameRateText       string  `json:"frame_rate_text,omitempty"`
	TimeBase            string  `json:"time_base,omitempty"`
	FrameCount          int64   `json:"frame_count,omitempty"`
	FrameCountAvailable bool    `json:"frame_count_available"`
	FrameCountSource    string  `json:"frame_count_source,omitempty"`
	EstimatedFrames     int64   `json:"estimated_frames,omitempty"`
}

type ColorReport struct {
	Status   string      `json:"status"`
	Standard string      `json:"standard,omitempty"`
	Actual   ColorActual `json:"actual"`
	Checks   []Check     `json:"checks"`
}

type ColorActual struct {
	Primaries               string `json:"primaries,omitempty"`
	Transfer                string `json:"transfer,omitempty"`
	Matrix                  string `json:"matrix,omitempty"`
	Range                   string `json:"range,omitempty"`
	PixelFormat             string `json:"pixel_format,omitempty"`
	BitDepth                int    `json:"bit_depth,omitempty"`
	MasteringDisplay        bool   `json:"mastering_display"`
	ContentLight            bool   `json:"content_light"`
	MaxCLL                  int    `json:"max_cll,omitempty"`
	MaxFALL                 int    `json:"max_fall,omitempty"`
	MasteringDisplayDetails string `json:"mastering_display_details,omitempty"`
}

type TimelineReport struct {
	Status string         `json:"status"`
	HDR    TimelineActual `json:"hdr"`
	SDR    TimelineActual `json:"sdr"`
	Checks []Check        `json:"checks"`
}

type TimelineActual struct {
	DurationMS          int64   `json:"duration_ms"`
	DurationAvailable   bool    `json:"duration_available"`
	StartMS             int64   `json:"start_ms"`
	StartAvailable      bool    `json:"start_available"`
	FrameRate           float64 `json:"frame_rate"`
	FrameRateAvailable  bool    `json:"frame_rate_available"`
	FrameRateText       string  `json:"frame_rate_text,omitempty"`
	FrameCount          int64   `json:"frame_count,omitempty"`
	FrameCountAvailable bool    `json:"frame_count_available"`
	FrameCountSource    string  `json:"frame_count_source,omitempty"`
	EstimatedFrames     int64   `json:"estimated_frames,omitempty"`
	DurationSource      string  `json:"duration_source,omitempty"`
}

type CutsReport struct {
	Mode         string      `json:"mode"`
	Status       string      `json:"status"`
	ToleranceMS  int64       `json:"tolerance_ms"`
	ChapterCount int         `json:"chapter_count"`
	Malformed    int         `json:"malformed"`
	Expected     []CutPoint  `json:"expected"`
	Actual       []CutMarker `json:"actual"`
	Missing      []CutPoint  `json:"missing,omitempty"`
	Unexpected   []CutMarker `json:"unexpected,omitempty"`
	Checks       []Check     `json:"checks"`
}

type CutMarker struct {
	ID     string `json:"id,omitempty"`
	Label  string `json:"label,omitempty"`
	TimeMS int64  `json:"time_ms"`
}

type DefaultsReport struct {
	Status   string        `json:"status"`
	Audio    *TrackSummary `json:"audio,omitempty"`
	Subtitle *TrackSummary `json:"subtitle,omitempty"`
	Checks   []Check       `json:"checks"`
}

type TrackSummary struct {
	Index             int    `json:"index"`
	Type              string `json:"type"`
	Codec             string `json:"codec,omitempty"`
	Language          string `json:"language,omitempty"`
	Title             string `json:"title,omitempty"`
	Default           bool   `json:"default"`
	Forced            bool   `json:"forced,omitempty"`
	StreamSHA256      string `json:"stream_sha256,omitempty"`
	StartMS           int64  `json:"start_ms"`
	StartAvailable    bool   `json:"start_available"`
	DurationMS        int64  `json:"duration_ms"`
	DurationAvailable bool   `json:"duration_available"`
	Channels          int    `json:"channels,omitempty"`
	ChannelLayout     string `json:"channel_layout,omitempty"`
	SampleRate        int    `json:"sample_rate,omitempty"`
}

type SharedReport struct {
	Status    string            `json:"status"`
	Timeline  TimelineReport    `json:"timeline"`
	Assets    []AssetReport     `json:"assets"`
	Audio     EmbeddedSetReport `json:"audio"`
	Subtitles EmbeddedSetReport `json:"subtitles"`
	Checks    []Check           `json:"checks"`
}

type AssetReport struct {
	ID              string       `json:"id"`
	Kind            string       `json:"kind"`
	Path            string       `json:"path"`
	Status          string       `json:"status"`
	SHA256          string       `json:"sha256,omitempty"`
	SizeBytes       int64        `json:"size_bytes,omitempty"`
	StreamSHA256    string       `json:"stream_sha256,omitempty"`
	Codec           string       `json:"codec,omitempty"`
	Language        string       `json:"language,omitempty"`
	Title           string       `json:"title,omitempty"`
	DeclaredDefault *bool        `json:"declared_default,omitempty"`
	HDR             *EmbeddedRef `json:"hdr,omitempty"`
	SDR             *EmbeddedRef `json:"sdr,omitempty"`
	Checks          []Check      `json:"checks"`
}

type EmbeddedRef struct {
	Found    bool   `json:"found"`
	Index    int    `json:"index,omitempty"`
	Default  bool   `json:"default"`
	Language string `json:"language,omitempty"`
	Title    string `json:"title,omitempty"`
}

type EmbeddedSetReport struct {
	Status        string   `json:"status"`
	HDRCount      int      `json:"hdr_count"`
	SDRCount      int      `json:"sdr_count"`
	HDRHashes     []string `json:"hdr_hashes,omitempty"`
	SDRHashes     []string `json:"sdr_hashes,omitempty"`
	HDRSignatures []string `json:"hdr_signatures,omitempty"`
	SDRSignatures []string `json:"sdr_signatures,omitempty"`
	Checks        []Check  `json:"checks"`
}

type Check struct {
	ID       string `json:"id"`
	Category string `json:"category"`
	Required bool   `json:"required"`
	Pass     bool   `json:"pass"`
	Expected any    `json:"expected,omitempty"`
	Actual   any    `json:"actual,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

type Failure struct {
	Scope    string `json:"scope"`
	CheckID  string `json:"check_id"`
	Category string `json:"category"`
	Detail   string `json:"detail,omitempty"`
	Expected any    `json:"expected,omitempty"`
	Actual   any    `json:"actual,omitempty"`
}
