package diag

// Issue categories keep container defects separate from bitstream
// contradictions and from capabilities of a particular playback device.
const (
	CategoryContainer  = "container_missing"
	CategoryConflict   = "bitstream_conflict"
	CategoryCapability = "player_capability"
	CategoryDRM        = "drm"
	CategoryEvidence   = "evidence"
)

const (
	SeverityInfo  = "info"
	SeverityWarn  = "warning"
	SeverityError = "error"
)

type Issue struct {
	Category string `json:"category"`
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type ContainerInfo struct {
	Format           string   `json:"format"`
	MajorBrand       string   `json:"major_brand,omitempty"`
	CompatibleBrands []string `json:"compatible_brands,omitempty"`
	Fragmented       bool     `json:"fragmented"`
	Encrypted        bool     `json:"encrypted"`
	EncryptionScheme string   `json:"encryption_scheme,omitempty"`
	OriginalFormat   string   `json:"original_format,omitempty"`
	PSSHPresent      bool     `json:"pssh_present"`
}

// DVConfig is the DOVIDecoderConfigurationRecord from dvcC or dvvC.
type DVConfig struct {
	Box                 string `json:"box"`
	VersionMajor        int    `json:"version_major"`
	VersionMinor        int    `json:"version_minor"`
	Profile             int    `json:"profile"`
	Level               int    `json:"level"`
	RPUPresent          bool   `json:"rpu_present"`
	ELPresent           bool   `json:"el_present"`
	BLPresent           bool   `json:"bl_present"`
	CompatibilityID     int    `json:"compatibility_id"`
	CompatibilityFormat string `json:"compatibility_format"`
	MetadataCompression int    `json:"metadata_compression"`
	CompressedMetadata  bool   `json:"compressed_metadata"`
	FeatureFlags        []bool `json:"feature_flags,omitempty"`
}

type HEVCConfig struct {
	ProfileSpace              int    `json:"profile_space"`
	TierFlag                  bool   `json:"tier_flag"`
	ProfileIDC                int    `json:"profile_idc"`
	LevelIDC                  int    `json:"level_idc"`
	ProfileCompatibilityFlags uint32 `json:"profile_compatibility_flags"`
	ChromaFormat              int    `json:"chroma_format"`
	BitDepthLuma              int    `json:"bit_depth_luma"`
	BitDepthChroma            int    `json:"bit_depth_chroma"`
	LengthSize                int    `json:"nal_length_size"`
	VPS                       int    `json:"vps_count"`
	SPS                       int    `json:"sps_count"`
	PPS                       int    `json:"pps_count"`
	SPSProfileIDC             int    `json:"sps_profile_idc,omitempty"`
	SPSLevelIDC               int    `json:"sps_level_idc,omitempty"`
	SPSCompatibilityFlags     uint32 `json:"sps_profile_compatibility_flags,omitempty"`
}

type ColourInfo struct {
	Type      string `json:"type"`
	Primaries int    `json:"primaries"`
	Transfer  int    `json:"transfer"`
	Matrix    int    `json:"matrix"`
	FullRange bool   `json:"full_range"`
}

type EnhancementInfo struct {
	Declared         bool                `json:"declared_by_dv_config"`
	HVCETrackPresent bool                `json:"hvce_present"`
	HVCECodec        *HEVCConfig         `json:"hvce_config,omitempty"`
	ObservedLayerIDs []int               `json:"observed_layer_ids,omitempty"`
	ObservedELNALs   int                 `json:"observed_el_nals"`
	TrackReferences  map[string][]uint32 `json:"track_references,omitempty"`
}

type RPURecord struct {
	SampleIndex     int  `json:"sample_index"`
	Profile         int  `json:"profile"`
	RPUType         int  `json:"rpu_type"`
	RPUFormat       int  `json:"rpu_format"`
	VDRRPUProfile   int  `json:"vdr_rpu_profile"`
	VDRRPULevel     int  `json:"vdr_rpu_level"`
	BLFullRange     bool `json:"bl_video_full_range"`
	ELResampling    bool `json:"el_spatial_resampling_filter"`
	DisableResidual bool `json:"disable_residual"`
	UsePreviousRPU  bool `json:"use_previous_rpu"`
	PreviousRPUID   int  `json:"previous_rpu_id"`
	MappingID       int  `json:"mapping_id"`
	CRCValid        bool `json:"crc_valid"`
}

type RPUStats struct {
	ScannedSamples    int         `json:"scanned_samples"`
	Complete          bool        `json:"scan_complete"`
	SamplesWithRPU    int         `json:"samples_with_rpu"`
	MissingRPUSamples []int       `json:"missing_rpu_samples,omitempty"`
	Profiles          []int       `json:"derived_profiles,omitempty"`
	ParseErrors       int         `json:"parse_errors"`
	CRCErrors         int         `json:"crc_errors"`
	ReferenceErrors   int         `json:"reference_errors"`
	ReferencesOK      bool        `json:"references_ok"`
	First             *RPURecord  `json:"first,omitempty"`
	NALCounts         map[int]int `json:"nal_counts,omitempty"`
	TotalNALCount     int         `json:"total_nal_count"`
}

type Availability struct {
	Status     string `json:"status"`
	Mode       string `json:"mode"`
	Usable     bool   `json:"usable"`
	Confidence string `json:"confidence"`
	Reason     string `json:"reason"`
}

type DeviceResult struct {
	Device   string  `json:"device"`
	Status   string  `json:"status"`
	Mode     string  `json:"mode"`
	Playable bool    `json:"playable"`
	Reason   string  `json:"reason"`
	Issues   []Issue `json:"issues,omitempty"`
}

type Segment struct {
	File                string          `json:"file"`
	TrackID             uint32          `json:"track_id"`
	SampleDescription   int             `json:"sample_description"`
	SampleEntry         string          `json:"sample_entry"`
	Codec               string          `json:"codec"`
	Width               int             `json:"width"`
	Height              int             `json:"height"`
	FPS                 float64         `json:"fps,omitempty"`
	SampleCount         int             `json:"sample_count"`
	Container           ContainerInfo   `json:"container"`
	DVConfig            *DVConfig       `json:"dolby_vision_config,omitempty"`
	HEVC                *HEVCConfig     `json:"hevc_config,omitempty"`
	Colour              *ColourInfo     `json:"colour,omitempty"`
	Enhancement         EnhancementInfo `json:"enhancement"`
	RPU                 *RPUStats       `json:"rpu,omitempty"`
	Issues              []Issue         `json:"issues,omitempty"`
	FullDolbyVision     Availability    `json:"full_dolby_vision"`
	MinimumAvailability Availability    `json:"minimum_availability"`
	Devices             []DeviceResult  `json:"devices,omitempty"`
}

type Report struct {
	Files []FileReport `json:"files"`
}

type FileReport struct {
	File     string    `json:"file"`
	Error    string    `json:"error,omitempty"`
	Segments []Segment `json:"segments,omitempty"`
}
