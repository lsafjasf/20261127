package iccproof

// BoundaryStage identifies which profile boundary produced a report.
type BoundaryStage string

const (
	BoundaryNone   BoundaryStage = "none"
	BoundarySource BoundaryStage = "source"
	BoundaryMedia  BoundaryStage = "media"
)

// Classification is the mutually exclusive top-level category.
type Classification string

const (
	ClassOK                    Classification = "ok"
	ClassSourceUnrepresentable Classification = "source_unrepresentable"
	ClassDeviceClipped         Classification = "device_clipped"
	ClassDeviceOutOfGamut      Classification = "device_out_of_gamut"
	ClassError                 Classification = "error"
)

// Stable reason codes used in BoundaryReport.Flags and channel records.
const (
	ReasonSourceCodeOutOfRange    = "source_code_out_of_range"
	ReasonSourceGamut             = "outside_source_profile_gamut"
	ReasonSourceIntentUnsupported = "source_intent_unsupported"
	ReasonMediaGamut              = "outside_media_gamut"
	ReasonDeviceChannelClip       = "device_channel_clipped"
	ReasonGamutAlarm              = "gamut_alarm"
	ReasonQuantizedToLimit        = "quantized_to_encoding_limit"
)

// ChannelBoundary gives numerical evidence for one channel crossing a limit.
type ChannelBoundary struct {
	Stage       BoundaryStage `json:"stage"`
	Channel     int           `json:"channel"`
	ChannelName string        `json:"channel_name"`
	Value       float64       `json:"value"`
	After       float64       `json:"after,omitempty"`
	Min         float64       `json:"min"`
	Max         float64       `json:"max"`
	Exceedance  float64       `json:"exceedance"`
	AtLimit     bool          `json:"at_limit,omitempty"`
	Reason      string        `json:"reason"`
}

// BoundaryReport separates upstream source failures from output-device clips.
type BoundaryReport struct {
	Classification      Classification    `json:"classification"`
	Stage               BoundaryStage     `json:"stage"`
	SourceRepresentable bool              `json:"source_representable"`
	SourceViolations    []ChannelBoundary `json:"source_violations,omitempty"`
	SourceGamutAlarm    *bool             `json:"source_gamut_alarm,omitempty"`
	MediaInGamut        *bool             `json:"media_in_gamut,omitempty"`
	MediaGamutAlarm     bool              `json:"media_gamut_alarm,omitempty"`
	MediaViolations     []ChannelBoundary `json:"media_violations,omitempty"`
	DeviceClipped       bool              `json:"device_clipped"`
	ClippedChannels     []ChannelBoundary `json:"clipped_channels,omitempty"`
	Flags               []string          `json:"flags,omitempty"`
	Messages            []string          `json:"messages,omitempty"`
}

func newOKBoundary() BoundaryReport {
	in := true
	return BoundaryReport{
		Classification:      ClassOK,
		Stage:               BoundaryNone,
		SourceRepresentable: true,
		MediaInGamut:        &in,
	}
}

func (b *BoundaryReport) addFlag(flag string) {
	for _, existing := range b.Flags {
		if existing == flag {
			return
		}
	}
	b.Flags = append(b.Flags, flag)
}

// Finalize chooses one mutually exclusive classification. Source problems win
// because device clipping is not evaluated for an unusable source color.
func (b *BoundaryReport) Finalize() {
	if len(b.SourceViolations) > 0 || (b.SourceGamutAlarm != nil && *b.SourceGamutAlarm) {
		b.SourceRepresentable = false
		b.Classification = ClassSourceUnrepresentable
		b.Stage = BoundarySource
		b.addFlag(string(ClassSourceUnrepresentable))
		return
	}
	b.SourceRepresentable = true
	if b.MediaGamutAlarm || len(b.MediaViolations) > 0 {
		in := false
		b.MediaInGamut = &in
		b.addFlag(ReasonMediaGamut)
		if b.MediaGamutAlarm {
			b.addFlag(ReasonGamutAlarm)
		}
	} else if b.MediaInGamut == nil {
		in := true
		b.MediaInGamut = &in
	}
	if b.DeviceClipped || len(b.ClippedChannels) > 0 {
		b.DeviceClipped = true
		b.Classification = ClassDeviceClipped
		b.Stage = BoundaryMedia
		b.addFlag(ReasonDeviceChannelClip)
		return
	}
	if b.MediaInGamut != nil && !*b.MediaInGamut {
		b.Classification = ClassDeviceOutOfGamut
		b.Stage = BoundaryMedia
		return
	}
	b.Classification = ClassOK
	b.Stage = BoundaryNone
}
