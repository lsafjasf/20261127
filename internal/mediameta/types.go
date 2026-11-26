package mediameta

type ProbeReport struct {
	Streams []Stream `json:"streams"`
}

type Stream struct {
	Index            int               `json:"index"`
	CodecName        string            `json:"codec_name"`
	CodecLongName    string            `json:"codec_long_name"`
	Profile          string            `json:"profile"`
	CodecType        string            `json:"codec_type"`
	CodecTagString   string            `json:"codec_tag_string"`
	Width            int               `json:"width"`
	Height           int               `json:"height"`
	PixelFormat      string            `json:"pix_fmt"`
	ColorSpace       string            `json:"color_space"`
	ColorTransfer    string            `json:"color_transfer"`
	ColorPrimaries   string            `json:"color_primaries"`
	ColorRange       string            `json:"color_range"`
	AvgFrameRate     string            `json:"avg_frame_rate"`
	RFrameRate       string            `json:"r_frame_rate"`
	NbFrames         string            `json:"nb_frames"`
	BitsPerRawSample string            `json:"bits_per_raw_sample"`
	Tags             map[string]string `json:"tags"`
}

type FileMetadata struct {
	Path             string   `json:"path"`
	Size             int64    `json:"size"`
	FFProbeAvailable bool     `json:"ffprobe_available"`
	FFProbe          *Stream  `json:"ffprobe,omitempty"`
	MOV              *MOVInfo `json:"mov,omitempty"`
}

type MOVInfo struct {
	MajorBrand        string    `json:"major_brand,omitempty"`
	ColorBox          *ColorBox `json:"color_box,omitempty"`
	ProResAlphaTypes  []int     `json:"prores_alpha_types,omitempty"`
	ProResAlphaMarked bool      `json:"prores_alpha_marked"`
	ICPFCount         int       `json:"icpf_count"`
}

type ColorBox struct {
	Offset        int64  `json:"offset"`
	Size          int64  `json:"size"`
	ColorType     string `json:"color_type"`
	Primaries     uint16 `json:"primaries"`
	Transfer      uint16 `json:"transfer"`
	Matrix        uint16 `json:"matrix"`
	FullRangeFlag *bool  `json:"full_range_flag,omitempty"`
	PrimariesName string `json:"primaries_name,omitempty"`
	TransferName  string `json:"transfer_name,omitempty"`
	MatrixName    string `json:"matrix_name,omitempty"`
}
