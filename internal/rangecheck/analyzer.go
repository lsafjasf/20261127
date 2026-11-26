package rangecheck

import "fmt"

// Analyzer consumes frames in decode/frame order and retains compact reports.
type Analyzer struct {
	Report      *Report
	settings    Settings
	active      []*trackState
	nextTrackID int
}

type trackState struct {
	track       Track
	lastRect    Rect
	missed      int
	bestScore   float64
	stableScore float64
	matchCount  int
}

// NewAnalyzer creates a frame-by-frame analysis session.
func NewAnalyzer(input string, settings Settings, metadata Metadata) *Analyzer {
	settings = settings.withDefaults()
	if settings.Width == 0 && metadata.Width > 0 {
		settings.Width = metadata.Width
		settings.Height = metadata.Height
	}
	if settings.FPS.Num == 0 && metadata.FrameRate.Num != 0 {
		settings.FPS = metadata.FrameRate
	}
	report := &Report{
		SchemaVersion: "rangecheck/v1",
		Input:         input,
		Settings:      settings,
		Metadata:      metadata,
		LegalRanges: LegalRanges{
			Bits:   settings.Bits,
			Luma:   settings.LumaBounds(),
			Chroma: settings.ChromaBounds(),
		},
		Conclusions:  []string{},
		ManualChecks: []string{},
	}
	return &Analyzer{Report: report, settings: settings, nextTrackID: 1}
}

// Add analyzes one frame, updates spatial tracks, and appends its compact report.
func (a *Analyzer) Add(frame Frame) (*FrameReport, error) {
	if frame.Width == 0 {
		frame.Width = a.settings.Width
		frame.Height = a.settings.Height
	}
	if frame.PTSSeconds == 0 && frame.Index > 0 && a.settings.FPS.Num > 0 {
		frame.PTSSeconds = float64(frame.Index) / a.settings.FPS.Float64()
	}
	fr, err := AnalyzeFrame(frame, a.settings)
	if err != nil {
		return nil, err
	}
	a.assignTracks(fr)
	a.Report.Frames = append(a.Report.Frames, *fr)
	return fr, nil
}

func (a *Analyzer) assignTracks(fr *FrameReport) {
	usedTracks := make(map[int]bool)
	for ri := range fr.Regions {
		region := &fr.Regions[ri]
		bestIdx := -1
		bestScore := 0.0
		for ti, ts := range a.active {
			if usedTracks[ts.track.ID] || ts.track.Kind != region.Kind {
				continue
			}
			score := matchScore(ts.lastRect, region.Rect)
			if score > bestScore {
				bestScore = score
				bestIdx = ti
			}
		}

		if bestIdx >= 0 && bestScore >= 0.18 {
			ts := a.active[bestIdx]
			usedTracks[ts.track.ID] = true
			ts.missed = 0
			ts.matchCount++
			ts.stableScore += bestScore
			region.TrackID = ts.track.ID
			ts.lastRect = region.Rect
			ts.track.LastFrame = fr.Index
			ts.track.EndSeconds = fr.PTSSeconds
			ts.track.FrameCount++
			if region.Confidence > ts.bestScore {
				ts.bestScore = region.Confidence
				ts.track.BestConfidence = region.Confidence
				ts.track.Representative = region.Rect
				ts.track.Source = region.Source
				ts.track.Verdict = region.Verdict
			}
			if ts.matchCount >= 4 {
				region.Confidence = clampConfidence(region.Confidence + 0.06)
				if region.Source == SourceBurnedSubtitle {
					region.Reasons = appendReason(region.Reasons, "stable_line_box_across_multiple_frames")
				}
			}
			continue
		}

		id := a.nextTrackID
		a.nextTrackID++
		region.TrackID = id
		ts := &trackState{
			lastRect:  region.Rect,
			bestScore: region.Confidence,
			track: Track{
				ID: id, Kind: region.Kind, Source: region.Source, Verdict: region.Verdict,
				FirstFrame: fr.Index, LastFrame: fr.Index,
				StartSeconds: fr.PTSSeconds, EndSeconds: fr.PTSSeconds,
				FrameCount: 1, Representative: region.Rect,
				BestConfidence: region.Confidence, Reasons: append([]string(nil), region.Reasons...),
			},
		}
		a.active = append(a.active, ts)
		usedTracks[id] = true
	}

	next := a.active[:0]
	for _, ts := range a.active {
		if usedTracks[ts.track.ID] {
			next = append(next, ts)
			continue
		}
		ts.missed++
		if ts.missed <= 3 {
			next = append(next, ts)
			continue
		}
		a.finalizeTrack(ts)
		a.Report.Tracks = append(a.Report.Tracks, ts.track)
	}
	a.active = next
}

func matchScore(a, b Rect) float64 {
	ix := maxInt(0, minInt(a.X+a.Width, b.X+b.Width)-maxInt(a.X, b.X))
	iy := maxInt(0, minInt(a.Y+a.Height, b.Y+b.Height)-maxInt(a.Y, b.Y))
	intersection := ix * iy
	union := a.Area() + b.Area() - intersection
	iou := float64(intersection) / float64(max(1, union))
	if iou >= 0.18 {
		return iou
	}
	smallArea := float64(minInt(a.Area(), b.Area()))
	largeArea := float64(maxInt(a.Area(), b.Area()))
	if smallArea/max(1, largeArea) < 0.20 {
		return 0
	}
	acx, acy := a.X+a.Width/2, a.Y+a.Height/2
	bcx, bcy := b.X+b.Width/2, b.Y+b.Height/2
	dx := float64(acx - bcx)
	dy := float64(acy - bcy)
	scale := float64(maxInt(a.Width+b.Width, a.Height+b.Height)) / 2
	if scale <= 1 {
		return 0
	}
	distance := dx*dx + dy*dy
	normalized := distance / (scale * scale)
	if normalized < 0.36 {
		return 0.20 * (1 - normalized/0.36)
	}
	return 0
}

func appendReason(reasons []string, reason string) []string {
	for _, existing := range reasons {
		if existing == reason {
			return reasons
		}
	}
	return append(reasons, reason)
}

// Close finalizes tracks, FPS, segments, and cross-frame conclusions.
func (a *Analyzer) Close() (*Report, error) {
	rep := a.Report
	for _, ts := range a.active {
		a.finalizeTrack(ts)
		rep.Tracks = append(rep.Tracks, ts.track)
	}
	if err := finalizeReport(rep); err != nil {
		return nil, fmt.Errorf("finalize range report: %w", err)
	}
	return rep, nil
}

func (a *Analyzer) finalizeTrack(ts *trackState) {
	t := &ts.track
	if ts.matchCount >= 5 {
		avgStability := ts.stableScore / float64(ts.matchCount)
		if t.Source == SourceBurnedSubtitle && avgStability > 0.35 {
			t.BestConfidence = clampConfidence(t.BestConfidence + 0.08)
			t.Verdict = VerdictEncodingError
			t.Reasons = appendReason(t.Reasons, "temporal_persistence_supports_burned_in_subtitle")
		}
		if t.Source == SourceCodecEdge && avgStability < 0.25 {
			t.Reasons = appendReason(t.Reasons, "unstable_transient_halo_supports_codec_overshoot")
		}
	}
	if t.FrameCount == 1 && t.Source == SourceUnknown {
		t.Verdict = VerdictAmbiguous
	}
}
