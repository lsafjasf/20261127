package tearcheck

import "image"

// Analyzer incrementally extracts observations from a frame sequence.
// It retains only the previous luma frame.
type Analyzer struct {
	Metadata         Metadata
	BlockSizes       []int
	MaxSeamsPerFrame int

	prev           *LumaFrame
	observations   []Observation
	framesAnalyzed int
}

// NewAnalyzer creates a streaming analyzer.
func NewAnalyzer(meta Metadata) *Analyzer {
	sizes := DefaultBlockSizes(meta.Codec)
	return &Analyzer{
		Metadata:         meta,
		BlockSizes:       sizes,
		MaxSeamsPerFrame: 4,
	}
}

// Push adds one compact luma frame.
func (a *Analyzer) Push(frame *LumaFrame) []Observation {
	if frame == nil {
		return nil
	}
	frame.Index = a.framesAnalyzed
	if len(a.BlockSizes) == 0 {
		a.BlockSizes = DefaultBlockSizes(a.Metadata.Codec)
	}
	observations := ExtractObservations(frame, a.prev, a.BlockSizes)
	a.observations = append(a.observations, observations...)
	a.prev = frame
	a.framesAnalyzed++
	return observations
}

// PushImage converts and analyzes a standard image.
func (a *Analyzer) PushImage(img image.Image, pts float64) []Observation {
	frame := LumaFromImage(img, a.framesAnalyzed, pts)
	observations := a.Push(frame)
	if colored, ok := img.(image.Image); ok && len(observations) > 0 {
		for i := range observations {
			if observations[i].SeamY > 0 {
				observations[i].ChromaEffect = ChromaEffectAt(colored, observations[i].SeamY)
			}
		}
		// Replace the appended zero-chroma copies from Push.
		start := len(a.observations) - len(observations)
		for i := range observations {
			a.observations[start+i] = observations[i]
		}
	}
	return observations
}

// Finish classifies all accumulated observations and merges metadata.
func (a *Analyzer) Finish() *Report {
	report := AnalyzeObservations(a.observations, a.Metadata)
	report.Summary.FramesAnalyzed = a.framesAnalyzed
	if a.Metadata.Width > 0 {
		report.Width = a.Metadata.Width
	}
	if a.Metadata.Height > 0 {
		report.Height = a.Metadata.Height
	}
	if a.prev != nil {
		if report.Width == 0 {
			report.Width = a.prev.Width
		}
		if report.Height == 0 {
			report.Height = a.prev.Height
		}
	}
	if a.Metadata.Codec != "" {
		report.Codec = a.Metadata.Codec
	}
	return report
}

// AnalyzeLumaFrames is a convenience entry point for in-memory frame sequences.
func AnalyzeLumaFrames(frames []*LumaFrame, meta Metadata) *Report {
	analyzer := NewAnalyzer(meta)
	for _, frame := range frames {
		analyzer.Push(frame)
	}
	return analyzer.Finish()
}

// AnalyzeImages is a convenience entry point for standard library images.
func AnalyzeImages(frames []image.Image, meta Metadata) *Report {
	analyzer := NewAnalyzer(meta)
	for i, frame := range frames {
		pts := 0.0
		if i < len(meta.FrameTiming) {
			pts = meta.FrameTiming[i].PTSSeconds
		}
		analyzer.PushImage(frame, pts)
	}
	return analyzer.Finish()
}

// ExtractObservations converts raw frame features into normalized observations.
func ExtractObservations(cur, prev *LumaFrame, blockSizes []int) []Observation {
	if cur == nil {
		return nil
	}
	if len(blockSizes) == 0 {
		blockSizes = DefaultBlockSizes("")
	}
	seams := DetectSeams(cur, blockSizes)
	out := make([]Observation, 0, len(seams))
	for _, seam := range seams {
		match, contrast, side, shift := TemporalSplice(cur, prev, seam.y)
		blockArtifact := BlockArtifactScore(cur, seam.y, blockSizes)
		blockRepeat := TemporalBlockRepeat(cur, prev, seam.y, blockSizes)
		gridRelevance := 0.18 + 0.82*seam.blockAlign
		blockArtifact = Clamp01(blockArtifact * gridRelevance)
		blockRepeat = Clamp01(blockRepeat * gridRelevance)
		arbitrary := Clamp01(seam.coverage * (1 - 0.82*seam.blockAlign))
		intraTear := seam.energy
		seamCoverage := seam.coverage
		if side == SideBoth {
			intraTear *= 0.22
			seamCoverage *= 0.65
			arbitrary *= 0.25
		}
		o := Observation{
			FrameIndex:          cur.Index,
			Time:                cur.PTS,
			SeamY:               seam.y,
			IntraFrameTear:      Clamp01(intraTear),
			SeamCoverage:        Clamp01(seamCoverage),
			AdjacentFrameMatch:  match,
			MatchContrast:       contrast,
			RepeatedSide:        side,
			HorizontalShift:     shift,
			ArbitraryScanline:   Clamp01(arbitrary),
			BlockAlignment:      seam.blockAlign,
			BlockArtifact:       blockArtifact,
			TemporalBlockRepeat: blockRepeat,
		}
		if side != SideNone {
			o.Messages = append(o.Messages, "adjacent-frame repeat on "+string(side)+" side")
		}
		if seam.blockAlign >= 0.9 {
			o.Messages = append(o.Messages, "fault aligns with coding block grid")
		}
		out = append(out, o)
	}
	if len(out) >= 2 {
		for i := range out {
			if len(out) >= 4 {
				out[i].EffectCoherence = 0.85
			} else {
				out[i].EffectCoherence = 0.58
			}
		}
	}
	return out
}
