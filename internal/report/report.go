// Package report renders per-frame compensation reports as CSV and JSON.
package report

import (
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"

	"blacklift/internal/compensate"
)

// Summary is the machine-readable run summary.
type Summary struct {
	Frames            int     `json:"frames"`
	StyleLiftFrames   int     `json:"styleLiftFrames"`
	MeanSigma8bit     float64 `json:"meanSigma8bit"`
	MeanApplied8bit   float64 `json:"meanApplied8bit"`
	MaxApplied8bit    float64 `json:"maxApplied8bit"`
	MeanNewClipFrac   float64 `json:"meanNewClippedLumaFrac"`
	MaxNewClipFrac    float64 `json:"maxNewClippedLumaFrac"`
	TotalNewClipLuma  int64   `json:"totalNewClippedLumaPixels"`
	TotalNewClipAny   int64   `json:"totalNewClippedAnyChannelPixels"`
	MeanGuardFrac     float64 `json:"meanGuardFrac"`
	ShadowDetailMean  float64 `json:"meanShadowDetailBefore8bit"`
	ShadowDetailAfter float64 `json:"meanShadowDetailAfter8bit"`
}

var csvHeader = []string{
	"frame", "file", "width", "height",
	"style_lift", "style_score",
	"sigma_8bit", "mask_base_8bit", "mask_corner_8bit", "mask_spread_8bit", "mask_r2",
	"raw_mean_8bit", "raw_max_8bit",
	"bounded_mean_8bit", "bounded_max_8bit",
	"applied_mean_8bit", "applied_max_8bit", "guard_frac",
	"shadow_detail_before_8bit", "shadow_detail_after_8bit",
	"new_clipped_luma_px", "new_clipped_luma_frac",
	"new_clipped_anychan_px", "new_clipped_anychan_frac",
}

func f8(v float64) float64 { return v * 255 }

func fs(v float64) string { return strconv.FormatFloat(v, 'f', 5, 64) }

// WriteCSV writes one row per frame.
func WriteCSV(path string, results []*compensate.FrameResult) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	if err := w.Write(csvHeader); err != nil {
		return err
	}
	for _, r := range results {
		row := []string{
			strconv.Itoa(r.Index),
			filepath.Base(r.Path),
			strconv.Itoa(r.Width),
			strconv.Itoa(r.Height),
			strconv.FormatBool(r.StyleLift),
			fs(r.StyleScore),
			fs(f8(r.Sigma)),
			fs(f8(r.MaskBase)),
			fs(f8(r.MaskCorner)),
			fs(f8(r.MaskSpread)),
			fs(r.MaskRSquared),
			fs(f8(r.RawMean)),
			fs(f8(r.RawMax)),
			fs(f8(r.BoundedMean)),
			fs(f8(r.BoundedMax)),
			fs(f8(r.AppliedMean)),
			fs(f8(r.AppliedMax)),
			fs(r.GuardFrac),
			fs(f8(r.ShadowDetailBefore)),
			fs(f8(r.ShadowDetailAfter)),
			strconv.FormatInt(r.NewClippedLuma, 10),
			fs(r.NewClippedFrac),
			strconv.FormatInt(r.NewClippedAnyChan, 10),
			fs(r.NewClippedAnyFrac),
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

// WriteJSON writes the run summary plus all frame rows.
func WriteJSON(path string, results []*compensate.FrameResult) error {
	s := BuildSummary(results)
	doc := struct {
		Summary Summary                   `json:"summary"`
		Frames  []*compensate.FrameResult `json:"frames"`
	}{Summary: s, Frames: results}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// BuildSummary aggregates per-frame results.
func BuildSummary(results []*compensate.FrameResult) Summary {
	var s Summary
	s.Frames = len(results)
	for _, r := range results {
		if r.StyleLift {
			s.StyleLiftFrames++
		}
		s.MeanSigma8bit += f8(r.Sigma)
		s.MeanApplied8bit += f8(r.AppliedMean)
		s.MeanNewClipFrac += r.NewClippedFrac
		s.MeanGuardFrac += r.GuardFrac
		s.ShadowDetailMean += f8(r.ShadowDetailBefore)
		s.ShadowDetailAfter += f8(r.ShadowDetailAfter)
		s.TotalNewClipLuma += r.NewClippedLuma
		s.TotalNewClipAny += r.NewClippedAnyChan
		if f8(r.AppliedMax) > s.MaxApplied8bit {
			s.MaxApplied8bit = f8(r.AppliedMax)
		}
		if r.NewClippedFrac > s.MaxNewClipFrac {
			s.MaxNewClipFrac = r.NewClippedFrac
		}
	}
	if s.Frames > 0 {
		n := float64(s.Frames)
		s.MeanSigma8bit /= n
		s.MeanApplied8bit /= n
		s.MeanNewClipFrac /= n
		s.MeanGuardFrac /= n
		s.ShadowDetailMean /= n
		s.ShadowDetailAfter /= n
	}
	return s
}
