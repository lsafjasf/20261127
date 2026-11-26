package timemap

import (
	"encoding/csv"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
)

// Pair holds the two pipelines side by side for one track and edit point.
type Pair struct {
	Track  string
	Point  TimelinePoint
	Ref    Event
	Built  Event
	Stage  string
	DMed   *big.Int
	DDisp  *big.Int
	Scaled *big.Rat // exact fractional media index before quantization (reference)
}

// BuildPairs runs both pipelines and computes per-stage deltas.
func BuildPairs(tracks []TrackConfig, edit EditResolution, refresh Rate, points []TimelinePoint) ([][]Pair, []Finding) {
	all := make([][]Pair, 0, len(tracks))
	findings := make([]Finding, 0, len(tracks))
	for _, cfg := range tracks {
		ref := make([]Event, len(points))
		built := make([]Event, len(points))
		pairs := make([]Pair, len(points))
		for i, p := range points {
			ref[i] = RunEvent(cfg, ModeReference, edit, refresh, p)
			built[i] = RunEvent(cfg, ModeAsBuilt, edit, refresh, p)
			dm := delta(built[i].Media.Index, ref[i].Media.Index)
			dd := delta(built[i].Display.Index, ref[i].Display.Index)
			stage := "-"
			switch {
			case ref[i].Edit.Index.Cmp(built[i].Edit.Index) != 0:
				stage = string(StageEdit)
			case dm.Sign() != 0:
				stage = string(StageMedia)
			case dd.Sign() != 0 && dm.Sign() == 0:
				stage = string(StageDisplay)
			}
			pairs[i] = Pair{
				Track:  cfg.Name,
				Point:  p,
				Ref:    ref[i],
				Built:  built[i],
				Stage:  stage,
				DMed:   dm,
				DDisp:  dd,
				Scaled: cfg.MediaRate.Scale(edit.TickToSeconds(p.Tick)),
			}
		}
		finding := Locate(cfg, edit, refresh, points, ref, built)
		findings = append(findings, finding)
		for j := range pairs {
			if pairs[j].Stage == string(StageDisplay) && finding.Stage == StageMedia {
				pairs[j].Stage = "media->display"
			}
		}
		all = append(all, pairs)
	}
	return all, findings
}

// WriteCSV emits the reviewable side-by-side table.
func WriteCSV(path string, all [][]Pair) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	header := []string{
		"track", "point", "tick_us", "edit_time_s",
		"ref_media_rate_per_s", "ref_media_index_exact", "ref_media_index", "ref_media_time_s",
		"ref_display_slot", "ref_display_time_s",
		"built_media_rate_per_s", "built_media_rule", "built_media_float_index", "built_media_index",
		"built_media_time_s", "built_display_rule", "built_display_slot", "built_display_time_s",
		"delta_media_index", "delta_display_slot", "first_diverged_stage",
	}
	if err := w.Write(header); err != nil {
		return err
	}

	for _, pairs := range all {
		for _, p := range pairs {
			row := []string{
				p.Track,
				p.Point.Label,
				fmt.Sprintf("%d", p.Point.Tick),
				FormatSeconds(p.Ref.Edit.Time, 6),
				rateLabel(p.Ref.Media.Rate),
				p.Scaled.RatString(),
				p.Ref.Media.Index.String(),
				FormatSeconds(p.Ref.Media.Time, 6),
				p.Ref.Display.Index.String(),
				FormatSeconds(p.Ref.Display.Time, 6),
				rateLabel(p.Built.Media.Rate),
				p.Built.Media.Rule.String(),
				fmt.Sprintf("%.17g", p.Built.Media.FloatVal),
				p.Built.Media.Index.String(),
				FormatSeconds(p.Built.Media.Time, 6),
				p.Built.Display.Rule.String(),
				p.Built.Display.Index.String(),
				FormatSeconds(p.Built.Display.Time, 6),
				p.DMed.String(),
				p.DDisp.String(),
				string(p.Stage),
			}
			if err := w.Write(row); err != nil {
				return err
			}
		}
	}
	return w.Error()
}
