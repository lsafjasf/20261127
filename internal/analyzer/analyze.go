package analyzer

import (
	"fmt"
	"math"
)

// Analyze compares a source sequence with a scaled capture of the same viewport.
func Analyze(opts Options) (Report, error) {
	sourcePaths, err := expandPaths(opts.SourcePaths)
	if err != nil {
		return Report{}, fmt.Errorf("source: %w", err)
	}
	scaledPaths, err := expandPaths(opts.ScaledPaths)
	if err != nil {
		return Report{}, fmt.Errorf("scaled: %w", err)
	}
	if len(sourcePaths) != len(scaledPaths) {
		return Report{}, fmt.Errorf("frame counts differ: source has %d, scaled has %d", len(sourcePaths), len(scaledPaths))
	}
	if len(sourcePaths) < 1 {
		return Report{}, fmt.Errorf("at least one frame is required")
	}

	sourceW, sourceH, err := decodeSize(sourcePaths[0])
	if err != nil {
		return Report{}, err
	}
	scaledW, scaledH, err := decodeSize(scaledPaths[0])
	if err != nil {
		return Report{}, err
	}
	scaleX := float64(scaledW) / float64(sourceW)
	scaleY := float64(scaledH) / float64(sourceH)
	if math.Abs(scaleX-scaleY)/max(scaleX, scaleY) > 0.015 {
		return Report{}, fmt.Errorf("source and scaled aspect ratios differ: scale_x=%.4f scale_y=%.4f; capture the same viewport", scaleX, scaleY)
	}
	scale := math.Sqrt(scaleX * scaleY)
	if scale <= 0 || math.IsNaN(scale) || math.IsInf(scale, 0) {
		return Report{}, fmt.Errorf("invalid inferred scale %.4f", scale)
	}

	sourceROI := Rect{X: 0, Y: 0, W: sourceW, H: sourceH}
	if opts.ROI != nil {
		sourceROI = *opts.ROI
	}
	if sourceROI.X < 0 || sourceROI.Y < 0 || sourceROI.W <= 0 || sourceROI.H <= 0 ||
		sourceROI.X+sourceROI.W > sourceW || sourceROI.Y+sourceROI.H > sourceH {
		return Report{}, fmt.Errorf("source ROI %d,%d,%d,%d is outside %dx%d", sourceROI.X, sourceROI.Y, sourceROI.W, sourceROI.H, sourceW, sourceH)
	}
	scaledROI, _ := mapROI(sourceROI, scaleX, scaleY, scaledW, scaledH)

	blockSizes := normalizeBlockSizes(opts.BlockSizes)
	sourceStats, err := analyzeSequence(sourcePaths, sourceROI, 1, blockSizes)
	if err != nil {
		return Report{}, fmt.Errorf("source: %w", err)
	}
	scaledStats, err := analyzeSequence(scaledPaths, scaledROI, scale, blockSizes)
	if err != nil {
		return Report{}, fmt.Errorf("scaled: %w", err)
	}

	comparison := buildComparison(sourceStats, scaledStats, scaleX, scaleY)
	report := Report{
		Source:     sourceStats,
		Scaled:     scaledStats,
		Comparison: comparison,
	}
	classify(&report)
	return report, nil
}

func buildComparison(source, scaled SequenceStats, scaleX, scaleY float64) Comparison {
	scale := math.Sqrt(scaleX * scaleY)
	sharpnessRetention := scaled.Spatial.HFNorm / max(source.Spatial.HFNorm, 1e-9)
	gradientRetention := scaled.Spatial.Gradient.RMS / max(source.Spatial.Gradient.RMS, 1e-9)
	edgeWidthRatio := scaled.Spatial.EdgeWidthMedian / max(source.Spatial.EdgeWidthMedian, 1e-9)

	sourceBlock := sameBlockResult(source.Block, scaled.Block.Best)
	blockDelta := scaled.Block.Best.Excess - sourceBlock.Excess
	if scaled.Block.Best.Phase < 0 {
		blockDelta = 0
	}

	return Comparison{
		ScaleX:             scaleX,
		ScaleY:             scaleY,
		Scale:              scale,
		SharpnessRetention: sharpnessRetention,
		GradientRetention:  gradientRetention,
		EdgeWidthRatio:     edgeWidthRatio,
		BlockExcessDelta:   blockDelta,
		TemporalDeltaDiff:  scaled.Temporal.DeltaNorm - source.Temporal.DeltaNorm,
		EdgeNoiseDelta:     scaled.Temporal.EdgeNoiseNorm - source.Temporal.EdgeNoiseNorm,
	}
}

func sameBlockResult(stats BlockStats, target BlockSizeStats) BlockSizeStats {
	if target.Phase < 0 {
		return BlockSizeStats{}
	}
	for _, candidate := range stats.Candidates {
		if candidate.Size == target.Size && candidate.Axis == target.Axis {
			return candidate
		}
	}
	return BlockSizeStats{}
}
