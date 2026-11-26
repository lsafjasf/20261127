package webm

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"

	"alphacheck/ebml"
)

// Options controls inspection verbosity and tolerances.
type Options struct {
	IncludeFrameDetails bool
	MaxFrameDetails     int
	IntervalTolerance   float64
}

// DefaultOptions returns detailed but bounded frame output.
func DefaultOptions() Options {
	return Options{IncludeFrameDetails: true, MaxFrameDetails: 24, IntervalTolerance: 0.20}
}

// InspectFile reads and inspects a WebM file using only standard library APIs.
func InspectFile(path string, opts Options) (*Report, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	report, err := Inspect(data, opts)
	if err != nil {
		return nil, err
	}
	report.Path = path
	return report, nil
}

// Inspect parses EBML bytes and validates alpha, crop, bitstream, and timing.
func Inspect(data []byte, opts Options) (*Report, error) {
	elems, err := ebml.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("parse EBML: %w", err)
	}
	rep := &Report{
		FileSize:        int64(len(data)),
		TimecodeScaleNS: 1_000_000,
		Verdict: Verdict{
			Status:      "pass",
			ErrorCounts: map[string]int{},
		},
	}

	var segment *ebml.Element
	for _, el := range elems {
		if el.ID == ebml.IDSegment {
			segment = el
			break
		}
	}
	if segment == nil {
		return nil, fmt.Errorf("webm: Segment element not found")
	}

	if info := segment.Find(ebml.IDInfo); info != nil {
		if scale := info.Find(ebml.IDTimecodeScale); scale != nil && scale.Int() != 0 {
			rep.TimecodeScaleNS = scale.Int()
		}
		rep.MuxingApp = info.Find(ebml.IDMuxingApp).SafeString()
		rep.WritingApp = info.Find(ebml.IDWritingApp).SafeString()
		if dur := info.Find(ebml.IDDuration); dur != nil {
			rep.ContainerDurationNS = durationElementNS(dur.Data, rep.TimecodeScaleNS)
		}
	} else {
		addFinding(rep, Finding{Severity: "error", Layer: "container", Class: "muxing_error", Code: "missing_info", Message: "Segment Info is missing"})
	}

	trackByNumber := map[uint64]*TrackReport{}
	if tracks := segment.Find(ebml.IDTracks); tracks != nil {
		for _, entry := range tracks.FindAll(ebml.IDTrackEntry) {
			track := parseTrack(entry)
			rep.Tracks = append(rep.Tracks, track)
			if track.TrackNumber != 0 {
				rep.Tracks[len(rep.Tracks)-1] = track
				trackByNumber[track.TrackNumber] = &rep.Tracks[len(rep.Tracks)-1]
			}
		}
	} else {
		addFinding(rep, Finding{Severity: "error", Layer: "container", Class: "muxing_error", Code: "missing_tracks", Message: "Tracks element is missing"})
	}

	frameCounts := map[uint64]int{}
	for _, cluster := range segment.FindAll(ebml.IDCluster) {
		clusterTC := int64(cluster.Find(ebml.IDTimecode).SafeUint())
		processCluster(cluster, clusterTC, rep.TimecodeScaleNS, trackByNumber, frameCounts, rep, opts)
	}

	finalizeTracks(rep)
	analyzeAndValidateTiming(rep, opts)
	if opts.MaxFrameDetails > 0 && len(rep.Frames) > opts.MaxFrameDetails {
		rep.Frames = rep.Frames[:opts.MaxFrameDetails]
	}
	rep.Verdict = buildVerdict(rep.Findings)
	return rep, nil
}

func durationElementNS(b []byte, scale uint64) int64 {
	var ticks float64
	switch len(b) {
	case 4:
		ticks = float64(math.Float32frombits(binary.BigEndian.Uint32(b)))
	case 8:
		ticks = math.Float64frombits(binary.BigEndian.Uint64(b))
	default:
		return 0
	}
	return int64(math.Round(ticks * float64(scale)))
}

func addFinding(rep *Report, f Finding) {
	if f.Severity == "" {
		f.Severity = "error"
	}
	rep.Findings = append(rep.Findings, f)
	if f.Severity == "error" {
		rep.Verdict.ErrorCounts[f.Class]++
	}
}
