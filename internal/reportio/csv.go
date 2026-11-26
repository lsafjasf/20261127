package reportio

import (
	"clipforensic/internal/analyze"
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
)

func WriteJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o644)
}

func WriteCandidateCSV(path string, report analyze.Report) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	_ = w.Write([]string{
		"file", "id", "status", "incident", "classification", "start_sec", "end_sec",
		"evidence_sec", "channel", "channels", "confidence", "severity",
		"plateau_runs", "plateau_samples", "longest_run", "exact_fs_samples",
		"steep_edge_ratio", "median_rise_samples", "centroid_hz",
		"high_air_ratio", "splatter_delta_db", "f0_hz", "harmonic_strength", "attribution",
	})
	for _, file := range report.Files {
		all := append(append([]analyze.Candidate{}, file.Candidates...), file.Artistic...)
		for _, c := range all {
			_ = w.Write([]string{
				file.File, c.ID, c.Status, strconv.FormatBool(c.Incident), c.Classification,
				sec(c.StartSec), sec(c.EndSec), sec(float64(c.EvidenceSample) / float64(file.SampleRate)),
				strconv.Itoa(c.Channel), ints(c.Channels), f3(c.Confidence), c.Severity,
				strconv.Itoa(c.Plateau.RunCount), strconv.Itoa(c.Plateau.PlateauSamples),
				strconv.Itoa(c.Plateau.LongestRun), strconv.Itoa(c.Plateau.ExactFullScaleSamples),
				f3(c.Edge.SteepEdgeRatio), f3(c.Edge.MedianRiseSamples), f1(c.Spectrum.CentroidHz),
				f6(c.Spectrum.HighRatio + c.Spectrum.AirRatio), f1(c.Spectrum.SplatterDeltaDB),
				f1(c.Spectrum.EstimatedF0Hz),
				f3(c.Spectrum.HarmonicStrength), c.Attribution,
			})
		}
	}
	w.Flush()
	return w.Error()
}

func WriteWaveformCSV(path string, report analyze.Report) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	_ = w.Write([]string{
		"file", "candidate_id", "status", "channel", "index_in_snippet",
		"sample", "time_sec", "value", "plateau", "evidence_marker",
	})
	for _, file := range report.Files {
		all := append(append([]analyze.Candidate{}, file.Candidates...), file.Artistic...)
		for _, cand := range all {
			for ch, vals := range cand.Waveform.Channels {
				for i, v := range vals {
					sample := cand.Waveform.FirstSample + i
					plateau := false
					if ch < len(cand.Waveform.PlateauMask) && i < len(cand.Waveform.PlateauMask[ch]) {
						plateau = cand.Waveform.PlateauMask[ch][i]
					}
					_ = w.Write([]string{
						file.File, cand.ID, cand.Status, strconv.Itoa(ch), strconv.Itoa(i),
						strconv.Itoa(sample), f6(float64(sample) / float64(file.SampleRate)),
						f6(v), strconv.FormatBool(plateau),
						strconv.FormatBool(sample == cand.Waveform.EvidenceSample),
					})
				}
			}
		}
	}
	w.Flush()
	return w.Error()
}

func sec(v float64) string { return strconv.FormatFloat(v, 'f', 6, 64) }
func f1(v float64) string  { return strconv.FormatFloat(v, 'f', 1, 64) }
func f3(v float64) string  { return strconv.FormatFloat(v, 'f', 3, 64) }
func f6(v float64) string  { return strconv.FormatFloat(v, 'f', 6, 64) }

func ints(xs []int) string {
	out := ""
	for i, x := range xs {
		if i > 0 {
			out += "|"
		}
		out += strconv.Itoa(x)
	}
	return out
}
