package avsync

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
)

// Input describes pulse and visual-event timestamps on the concatenated
// timeline. Segment boundaries are half-open: [Start, End).
type Input struct {
	TimeUnit string    `json:"time_unit"`
	Unit     string    `json:"unit"`
	Segments []Segment `json:"segments"`
	Cuts     []float64 `json:"cuts"`

	Pulses []float64 `json:"pulses"`
	Events []float64 `json:"events"`

	AudioPulses []float64 `json:"audio_pulses"`
	VideoEvents []float64 `json:"video_events"`

	Options Options `json:"options"`
}

type Segment struct {
	ID     string  `json:"id"`
	Source string  `json:"source"`
	Start  float64 `json:"start"`
	End    float64 `json:"end"`
}

type Options struct {
	MatchWindowMS     float64 `json:"match_window_ms"`
	OffsetToleranceMS float64 `json:"offset_tolerance_ms"`
	DriftTolerancePPM float64 `json:"drift_tolerance_ppm"`
	MinPairs          int     `json:"min_pairs"`
	MinDriftSpanMS    float64 `json:"min_drift_span_ms"`
}

func DefaultOptions() Options {
	return Options{
		MatchWindowMS:     250,
		OffsetToleranceMS: 10,
		DriftTolerancePPM: 1000,
		MinPairs:          3,
		MinDriftSpanMS:    1000,
	}
}

func (o Options) withDefaults() Options {
	d := DefaultOptions()
	if o.MatchWindowMS <= 0 {
		o.MatchWindowMS = d.MatchWindowMS
	}
	if o.OffsetToleranceMS <= 0 {
		o.OffsetToleranceMS = d.OffsetToleranceMS
	}
	if o.DriftTolerancePPM <= 0 {
		o.DriftTolerancePPM = d.DriftTolerancePPM
	}
	if o.MinPairs <= 0 {
		o.MinPairs = d.MinPairs
	}
	if o.MinDriftSpanMS <= 0 {
		o.MinDriftSpanMS = d.MinDriftSpanMS
	}
	return o
}

type Result struct {
	TimeUnit       string          `json:"time_unit"`
	OffsetUnit     string          `json:"offset_unit"`
	Segments       []SegmentResult `json:"segments"`
	FirstAnomaly   *Anomaly        `json:"first_anomaly"`
	Before         ErrorStats      `json:"before"`
	After          ErrorStats      `json:"after"`
	GlobalAfter    ErrorStats      `json:"global_after"`
	MatchedPairs   int             `json:"matched_pairs"`
	UnmatchedPulse int             `json:"unmatched_pulses"`
	UnmatchedEvent int             `json:"unmatched_events"`
}

type SegmentResult struct {
	ID            string   `json:"id"`
	Source        string   `json:"source"`
	StartMS       float64  `json:"start_ms"`
	EndMS         float64  `json:"end_ms"`
	PairCount     int      `json:"pair_count"`
	OffsetMS      float64  `json:"offset_ms"`
	OffsetEndMS   float64  `json:"offset_end_ms"`
	DriftPPM      float64  `json:"drift_ppm"`
	DriftMSPerS   float64  `json:"drift_ms_per_s"`
	DriftGain     float64  `json:"drift_gain"`
	RMSBeforeMS   float64  `json:"rms_before_ms"`
	MaxBeforeMS   float64  `json:"max_before_ms"`
	RMSAfterMS    float64  `json:"rms_after_ms"`
	MaxAfterMS    float64  `json:"max_after_ms"`
	Anomalous     bool     `json:"anomalous"`
	AnomalyWhy    []string `json:"anomaly_reasons,omitempty"`
	FirstBadMS    *float64 `json:"first_bad_ms,omitempty"`
	EnoughPairs   bool     `json:"enough_pairs"`
	DriftReliable bool     `json:"drift_reliable"`
}

type ErrorStats struct {
	RMSMS float64 `json:"rms_ms"`
	MaxMS float64 `json:"max_ms"`
}

type Anomaly struct {
	SegmentID      string   `json:"segment_id"`
	Source         string   `json:"source"`
	SegmentStartMS float64  `json:"segment_start_ms"`
	FirstBadMS     *float64 `json:"first_bad_ms,omitempty"`
	Reasons        []string `json:"reasons"`
}

type pair struct {
	pulseMS float64
	eventMS float64
	segment int
}

type segmentWork struct {
	spec    Segment
	startMS float64
	endMS   float64
	pulses  []float64
	events  []float64
	pairs   []pair
	fit     lineFit
}

type lineFit struct {
	offset float64
	slope  float64
	n      int
}

func (f lineFit) predict(x float64) float64 {
	return f.offset + f.slope*x
}

func (in *Input) normalize() ([]Segment, []float64, []float64, Options, error) {
	if len(in.Pulses) == 0 {
		in.Pulses = in.AudioPulses
	}
	if len(in.Events) == 0 {
		in.Events = in.VideoEvents
	}
	if len(in.Pulses) < 2 || len(in.Events) < 2 {
		return nil, nil, nil, Options{}, fmt.Errorf("need at least two pulses and two events")
	}

	unit := strings.ToLower(strings.TrimSpace(in.TimeUnit))
	if unit == "" {
		unit = strings.ToLower(strings.TrimSpace(in.Unit))
	}
	scale, err := unitScale(unit)
	if err != nil {
		return nil, nil, nil, Options{}, err
	}

	pulses := scaledSorted(in.Pulses, scale)
	events := scaledSorted(in.Events, scale)
	segs := append([]Segment(nil), in.Segments...)
	inferred := false
	if len(segs) == 0 {
		segs = segmentsFromCuts(in.Cuts, scale, pulses, events)
		inferred = len(segs) > 0
	}
	if len(segs) == 0 {
		return nil, nil, nil, Options{}, fmt.Errorf("no segments: provide segments or cuts")
	}

	sort.SliceStable(segs, func(i, j int) bool { return segs[i].Start < segs[j].Start })
	for i := range segs {
		if !inferred {
			segs[i].Start *= scale
			segs[i].End *= scale
		}
		if segs[i].ID == "" {
			segs[i].ID = fmt.Sprintf("segment-%d", i+1)
		}
		if segs[i].Source == "" {
			segs[i].Source = segs[i].ID
		}
		if segs[i].End <= segs[i].Start {
			return nil, nil, nil, Options{}, fmt.Errorf("segment %q has non-positive duration", segs[i].ID)
		}
		if i > 0 && segs[i].Start < segs[i-1].End-1e-9 {
			return nil, nil, nil, Options{}, fmt.Errorf("segments %q and %q overlap", segs[i-1].ID, segs[i].ID)
		}
	}
	return segs, pulses, events, in.Options.withDefaults(), nil
}

func unitScale(unit string) (float64, error) {
	switch unit {
	case "", "s", "sec", "secs", "second", "seconds":
		return 1000, nil
	case "ms", "millisecond", "milliseconds":
		return 1, nil
	case "us", "usec", "microsecond", "microseconds", "µs":
		return 0.001, nil
	default:
		return 0, fmt.Errorf("unsupported time unit %q (use s, ms, or us)", unit)
	}
}

func scaledSorted(values []float64, scale float64) []float64 {
	out := make([]float64, len(values))
	for i, v := range values {
		out[i] = v * scale
	}
	sort.Float64s(out)
	return out
}

func segmentsFromCuts(cuts []float64, scale float64, pulseMS, eventMS []float64) []Segment {
	if len(cuts) == 0 {
		return nil
	}
	cuts = append([]float64(nil), cuts...)
	for i := range cuts {
		cuts[i] *= scale
	}
	sort.Float64s(cuts)
	minT, maxT := math.Inf(1), math.Inf(-1)
	for _, v := range pulseMS {
		minT = math.Min(minT, v)
		maxT = math.Max(maxT, v)
	}
	for _, v := range eventMS {
		minT = math.Min(minT, v)
		maxT = math.Max(maxT, v)
	}
	bounds := []float64{minT}
	for _, c := range cuts {
		if c > minT && c < maxT {
			bounds = append(bounds, c)
		}
	}
	bounds = append(bounds, maxT+1e-9)
	out := make([]Segment, 0, len(bounds)-1)
	for i := 0; i+1 < len(bounds); i++ {
		out = append(out, Segment{
			ID:    fmt.Sprintf("segment-%d", i+1),
			Start: bounds[i],
			End:   bounds[i+1],
		})
	}
	return out
}

// Unmarshal decodes JSON with strict unknown-field checking.
func Unmarshal(data []byte) (Input, error) {
	var in Input
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		return Input{}, err
	}
	return in, nil
}
