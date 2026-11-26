package mp4seek

import (
	"fmt"
	"sort"
	"time"
)

type File struct {
	Path           string
	Size           int64
	MDATs          []PayloadSpan
	Tracks         []*Track
	MovieTimescale uint32

	data []byte
	moov box
}

type PayloadSpan struct {
	Offset uint64 `json:"offset"`
	Size   uint64 `json:"size"`
	SHA256 string `json:"sha256,omitempty"`
}

type Track struct {
	ID                 uint32   `json:"id"`
	Handler            string   `json:"handler"`
	Codec              string   `json:"codec"`
	Timescale          uint32   `json:"timescale"`
	Duration           uint64   `json:"duration"`
	EditOffset         int64    `json:"edit_offset"`
	Samples            []Sample `json:"samples,omitempty"`
	IndexedSyncSamples []uint32 `json:"indexed_sync_samples"`
	ActualSyncSamples  []uint32 `json:"actual_sync_samples"`
	ScanComplete       bool     `json:"scan_complete"`
	Warnings           []string `json:"warnings,omitempty"`

	stbl       box
	stblPath   []box
	nalLength  int
	hevc       bool
	configSeen bool
	hasSTSS    bool
	rawSTSS    []uint32
	ownerData  []byte
}

type Sample struct {
	Number           uint32 `json:"number"`
	DTS              int64  `json:"dts"`
	CTS              int64  `json:"cts"`
	Duration         uint32 `json:"duration"`
	Size             uint32 `json:"size"`
	Offset           uint64 `json:"offset"`
	IndexedSync      bool   `json:"indexed_sync"`
	RandomAccess     bool   `json:"random_access"`
	Kind             string `json:"kind"`
	Dependency       string `json:"dependency"`
	NALTypes         []int  `json:"nal_types,omitempty"`
	SDTPDependency   string `json:"sdtp_dependency,omitempty"`
	LeadingRASL      bool   `json:"leading_rasl,omitempty"`
	BitstreamScanned bool   `json:"-"`
}

func (s Sample) PTSMicros(timescale uint32) int64 {
	return scaleToMicros(s.CTS, timescale)
}

func (s Sample) DTMicros(timescale uint32) int64 {
	return scaleToMicros(s.DTS, timescale)
}

func (t *Track) PTSMicros(sample Sample) int64 {
	return scaleToMicros(sample.CTS+t.EditOffset, t.Timescale)
}

func (t *Track) presentationCTS(sample Sample) int64 {
	return sample.CTS + t.EditOffset
}

type GOP struct {
	TrackID     uint32 `json:"track_id"`
	StartSample uint32 `json:"start_sample"`
	EndSample   uint32 `json:"end_sample"`
	StartDTSUS  int64  `json:"start_dts_us"`
	StartPTSUS  int64  `json:"start_pts_us"`
	EndDTSUS    int64  `json:"end_dts_us"`
	EndPTSUS    int64  `json:"end_pts_us"`
	Offset      uint64 `json:"offset"`
	Size        uint32 `json:"size"`
	Kind        string `json:"kind"`
	Indexed     bool   `json:"indexed"`
	RASLLeading bool   `json:"has_leading_rasl"`
}

type SeekHit struct {
	TrackID            uint32 `json:"track_id"`
	Source             string `json:"source"`
	Sample             uint32 `json:"sample"`
	PTSUS              int64  `json:"pts_us"`
	Offset             uint64 `json:"offset"`
	Size               uint32 `json:"size"`
	Kind               string `json:"kind"`
	ActualRandomAccess bool   `json:"actual_random_access"`
}

type TimeRange struct {
	TrackID uint32 `json:"track_id"`
	StartUS int64  `json:"start_us"`
	EndUS   int64  `json:"end_us"`
	Reason  string `json:"reason"`
}

type Report struct {
	Path               string        `json:"path,omitempty"`
	Size               int64         `json:"size"`
	MDATs              []PayloadSpan `json:"mdats"`
	Tracks             []TrackReport `json:"tracks"`
	GOPs               []GOP         `json:"gops"`
	SeekHits           []SeekHit     `json:"seek_hits"`
	InexactRanges      []TimeRange   `json:"inexact_ranges"`
	BrokenRanges       []TimeRange   `json:"broken_ranges"`
	MissingSyncSamples []uint32      `json:"missing_sync_samples,omitempty"`
	FalseSyncSamples   []uint32      `json:"false_sync_samples,omitempty"`
	Warnings           []string      `json:"warnings,omitempty"`
}

type TrackReport struct {
	ID           uint32   `json:"id"`
	Handler      string   `json:"handler"`
	Codec        string   `json:"codec"`
	Timescale    uint32   `json:"timescale"`
	EditOffsetUS int64    `json:"edit_offset_us"`
	SampleCount  int      `json:"sample_count"`
	IndexedSync  []uint32 `json:"indexed_sync_samples"`
	ActualSync   []uint32 `json:"actual_sync_samples"`
	ScanComplete bool     `json:"scan_complete"`
	Samples      []Sample `json:"samples,omitempty"`
	Warnings     []string `json:"warnings,omitempty"`
}

type RepairReport struct {
	Input         string        `json:"input"`
	Output        string        `json:"output"`
	Changed       bool          `json:"changed"`
	OriginalSize  int64         `json:"original_size"`
	RepairedSize  int64         `json:"repaired_size"`
	MoovSizeDelta int64         `json:"moov_size_delta"`
	OffsetDelta   int64         `json:"chunk_offset_delta"`
	Tracks        []TrackRepair `json:"tracks"`
	PayloadOK     bool          `json:"payload_verified"`
	MDATsBefore   []PayloadSpan `json:"mdats_before"`
	MDATsAfter    []PayloadSpan `json:"mdats_after"`
}

type TrackRepair struct {
	ID      uint32   `json:"id"`
	OldSync []uint32 `json:"old_sync_samples"`
	NewSync []uint32 `json:"new_sync_samples"`
	Added   []uint32 `json:"added"`
	Removed []uint32 `json:"removed"`
}

type SeekResult struct {
	TrackID         uint32 `json:"track_id"`
	TargetUS        int64  `json:"target_us"`
	RequestedSample uint32 `json:"requested_sample"`
	RequestedPTSUS  int64  `json:"requested_pts_us"`
	IndexedSample   uint32 `json:"indexed_sample"`
	IndexedPTSUS    int64  `json:"indexed_pts_us"`
	IndexedOffset   uint64 `json:"indexed_offset"`
	IndexedIsRAP    bool   `json:"indexed_is_rap"`
	ActualSample    uint32 `json:"actual_sample"`
	ActualPTSUS     int64  `json:"actual_pts_us"`
	Exact           bool   `json:"exact"`
	FalseSuccess    bool   `json:"false_success"`
	PreciseAfterFix bool   `json:"precise_after_fix"`
}

func scaleToMicros(value int64, timescale uint32) int64 {
	if timescale == 0 {
		return 0
	}
	scale := int64(timescale)
	whole := value / scale
	rem := value % scale
	return whole*1_000_000 + rem*1_000_000/scale
}

func formatMicros(us int64) string {
	d := time.Duration(us) * time.Microsecond
	return d.String()
}

func (f *File) trackByID(id uint32) *Track {
	for _, track := range f.Tracks {
		if track.ID == id {
			return track
		}
	}
	return nil
}

func sortedSampleNumbers(samples []Sample, match func(Sample) bool) []uint32 {
	out := make([]uint32, 0)
	for _, sample := range samples {
		if match(sample) {
			out = append(out, sample.Number)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func validateSampleNumber(number uint32, count int) error {
	if number == 0 || int(number) > count {
		return fmt.Errorf("sample number %d outside 1..%d", number, count)
	}
	return nil
}
