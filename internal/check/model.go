package check

type Layer string

const (
	LayerContainer Layer = "container"
	LayerBitstream Layer = "bitstream_crc"
	LayerDecode    Layer = "decode"
	LayerPixels    Layer = "pixels"
)

type Status string

const (
	StatusPass    Status = "pass"
	StatusFail    Status = "fail"
	StatusWarn    Status = "warn"
	StatusUnknown Status = "unknown"
)

type FrameRef struct {
	Key          string   `json:"key"`
	DecodeOrder  int      `json:"decode_order"`
	DisplayOrder *int     `json:"display_order,omitempty"`
	PTS          *int64   `json:"pts,omitempty"`
	DTS          *int64   `json:"dts,omitempty"`
	KeyFrame     *bool    `json:"key_frame,omitempty"`
	References   []string `json:"references,omitempty"`
}

type Observation struct {
	ID               string   `json:"id,omitempty"`
	Layer            Layer    `json:"layer"`
	Frame            FrameRef `json:"frame"`
	Check            string   `json:"check"`
	Status           Status   `json:"status"`
	TrustedReference bool     `json:"trusted_reference,omitempty"`
	Expected         string   `json:"expected,omitempty"`
	Actual           string   `json:"actual,omitempty"`
	Coverage         []string `json:"coverage,omitempty"`
	Message          string   `json:"message,omitempty"`
}

type Recovery struct {
	FrameKey           string `json:"frame_key"`
	GoodPacket         bool   `json:"good_packet_available,omitempty"`
	GoodPixels         bool   `json:"good_pixels_available,omitempty"`
	BitExactDecoder    bool   `json:"bit_exact_decoder,omitempty"`
	MetadataRedundancy bool   `json:"metadata_redundancy_available,omitempty"`
}

type Bundle struct {
	Observations []Observation `json:"observations"`
	Recovery     []Recovery    `json:"recovery,omitempty"`
}

type Reconstructability string

const (
	ReconstructYes     Reconstructability = "yes"
	ReconstructNo      Reconstructability = "no_from_current_evidence"
	ReconstructUnknown Reconstructability = "unknown_needs_trusted_source"
)

type Verdict struct {
	FrameKey        string             `json:"frame_key"`
	Reconstructable Reconstructability `json:"reconstructable"`
	Mode            string             `json:"mode,omitempty"`
	Reasons         []string           `json:"reasons,omitempty"`
	Blockers        []string           `json:"blockers,omitempty"`
	Warnings        []string           `json:"warnings,omitempty"`
	Recovery        *Recovery          `json:"recovery,omitempty"`
	LayerStatus     map[Layer]Status   `json:"layer_status"`
	ImpactedBy      []string           `json:"impacted_by_failures,omitempty"`
}

type Earliest struct {
	DecodeOrder int          `json:"decode_order"`
	FrameKey    string       `json:"frame_key"`
	Layer       Layer        `json:"layer"`
	Check       string       `json:"check"`
	Status      Status       `json:"status"`
	Observation *Observation `json:"observation,omitempty"`
}

type Report struct {
	FrameCount      int                `json:"frame_count"`
	Earliest        *Earliest          `json:"earliest,omitempty"`
	EarliestDisplay *Earliest          `json:"earliest_display,omitempty"`
	EarliestByLayer map[Layer]Earliest `json:"earliest_by_layer,omitempty"`
	FrameVerdicts   []Verdict          `json:"frame_verdicts"`
	GlobalWarnings  []string           `json:"global_warnings,omitempty"`
}
