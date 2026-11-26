package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"seekdiag/internal/diag"
	"seekdiag/internal/mp4"
	"seekdiag/internal/seek"
)

func main() {
	var (
		seekAt       = flag.Duration("seek", 20*time.Minute, "seek target in the movie timeline")
		obsPath      = flag.String("observations", "", "player probe JSON file ('-' for stdin)")
		jsonOut      = flag.Bool("json", false, "write the complete report as JSON")
		minDriftMS   = flag.Float64("min-drift-ms", 10, "minimum post-seek error change")
		minDriftMSps = flag.Float64("min-drift-ms-per-s", 0.1, "minimum post-seek drift rate")
		boundaryTol  = flag.Float64("boundary-tolerance-ms", 2, "allowed static timing offset")
	)
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: seekdiag [flags] file.mp4")
		flag.PrintDefaults()
		os.Exit(2)
	}
	f, err := mp4.Open(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "open media:", err)
		os.Exit(1)
	}
	obs, err := loadObservations(*obsPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load observations:", err)
		os.Exit(1)
	}
	opts := diag.DefaultOptions()
	opts.MinDriftMS = *minDriftMS
	opts.MinDriftMSps = *minDriftMSps
	opts.BoundaryTolMS = *boundaryTol
	result := diag.Analyze(f, *seekAt, obs, opts)
	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(result); err != nil {
			fmt.Fprintln(os.Stderr, "encode report:", err)
			os.Exit(1)
		}
		return
	}
	writeText(os.Stdout, result)
}

func loadObservations(path string) (*diag.ObservationSet, error) {
	if path == "" {
		return nil, nil
	}
	var r io.Reader = os.Stdin
	if path != "-" {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		r = f
	}
	var set diag.ObservationSet
	dec := json.NewDecoder(r)
	if err := dec.Decode(&set); err != nil {
		return nil, err
	}
	if set.SeekMS == 0 && len(set.After) == 0 {
		return nil, fmt.Errorf("observations must contain an 'after' array")
	}
	return &set, nil
}

func writeText(w io.Writer, r *diag.Result) {
	fmt.Fprintf(w, "文件: %s\n", r.File)
	fmt.Fprintf(w, "seek: %.3f ms\n", r.SeekMS)
	fmt.Fprintf(w, "结论: %s (%s)\n", verdictText(r.Verdict), r.Confidence)
	fmt.Fprintf(w, "说明: %s\n", r.Summary)
	if r.SeekPlan != nil {
		fmt.Fprintf(w, "movie timescale: %d, target tick: %d\n", r.SeekPlan.MovieTimescale, r.SeekPlan.TargetMovieTick)
	}
	if r.SeekAnchor != nil {
		fmt.Fprintf(w, "一次性锚点: movie=%.3f ms, audible PCM=%d, sample=%d, file_offset=%d\n",
			r.SeekAnchor.MovieMS, r.SeekAnchor.Audio.AnchorPCM, r.SeekAnchor.AudioSample, r.SeekAnchor.FileOffset)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "[轨道/编辑列表]")
	for _, t := range r.Tracks {
		fmt.Fprintf(w, "track %d %s %s ts=%d samples=%d edit_list=%v\n",
			t.TrackID, t.Kind, t.Codec, t.Timescale, t.SampleCount, t.EditListFound)
		for _, e := range t.Edits {
			fmt.Fprintf(w, "  elst[%d] dur=%d media_time=%d rate=%.4f movie=[%.3f,%.3f] media=[%.3f,%.3f] empty=%v\n",
				e.Index, e.SegmentDuration, e.MediaTime, e.MediaRate,
				e.MovieStartMS, e.MovieEndMS, e.MediaStartMS, e.MediaEndMS, e.Empty)
		}
	}
	if r.SeekPlan != nil {
		fmt.Fprintln(w, "\n[seek 边界样本]")
		for _, tp := range r.SeekPlan.Tracks {
			fmt.Fprintf(w, "track %d %s edit=%d\n", tp.TrackID, tp.Kind, firstEdit(tp))
			writePosition(w, "前", tp.Boundary.Previous)
			writePosition(w, "命中", tp.Boundary.Landing)
			writePosition(w, "后", tp.Boundary.Next)
		}
	}
	if r.Observations != nil {
		fmt.Fprintln(w, "\n[探针漂移]")
		if len(r.Observations.Drift) == 0 {
			fmt.Fprintln(w, "  无足够 seek 后探针")
		}
		for _, d := range r.Observations.Drift {
			fmt.Fprintf(w, "  %-14s delta=%.3f ms span=%.3f s slope=%.3f ms/s samples=%d\n",
				d.Layer, d.DeltaMS, d.SpanMS/1000, d.SlopeMSps, d.Samples)
		}
	}
	if len(r.Findings) > 0 {
		fmt.Fprintln(w, "\n[证据]")
		for _, f := range r.Findings {
			fmt.Fprintf(w, "  %-5s %-12s %s\n", f.Severity, f.Layer, f.Message)
		}
	}
	fmt.Fprintf(w, "\n注意: %s\n", r.ResyncWarning)
}

func firstEdit(tp seek.TrackPlan) int {
	if tp.Boundary.Landing != nil {
		return tp.Boundary.Landing.EditIndex
	}
	return -1
}

func writePosition(w io.Writer, label string, p *seek.Position) {
	if p == nil {
		fmt.Fprintf(w, "  %s: 无\n", label)
		return
	}
	fmt.Fprintf(w, "  %s: sample=%d pts=%.3f ms dur=%.3f ms trim=%.3f file=[%d,+%d] chunk=%d off=%d pcm=%d key=%v\n",
		label, p.SampleIndex, p.SamplePTSMS, p.SampleDurationMS, p.TrimBeforeMS,
		p.FileOffset, p.SampleSize, p.Chunk, p.OffsetInChunk, p.PCMStart, p.Keyframe)
}

func verdictText(v string) string {
	switch v {
	case diag.VerdictContainer:
		return "容器"
	case diag.VerdictDecoder:
		return "解码器"
	case diag.VerdictClock:
		return "播放时钟"
	case diag.VerdictOK:
		return "未复现故障"
	default:
		return "证据不足"
	}
}
