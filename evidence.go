package laddercheck

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LadderInput is the JSON manifest/evidence document.
type LadderInput struct {
	Schema     string             `json:"schema,omitempty"`
	Constraint *ConstraintOptions `json:"constraint,omitempty"`
	// Files are plain media paths to parse directly.
	Files []string `json:"files,omitempty"`
	// Renditions are either path references (parsed) or inline evidence.
	Renditions []RenditionInput `json:"renditions,omitempty"`
}

// RenditionInput is one rung: set Path to parse a real file, or supply the
// timing/keyframe evidence fields directly.
type RenditionInput struct {
	Path string `json:"path,omitempty"`
	ID   string `json:"id,omitempty"`
	Role string `json:"role,omitempty"`

	Codec                 string         `json:"codec,omitempty"`
	Width                 int            `json:"width,omitempty"`
	Height                int            `json:"height,omitempty"`
	FPS                   Rat            `json:"fps,omitempty"`
	FrameDuration         Rat            `json:"frame_duration,omitempty"`
	VariableFrameDuration bool           `json:"variable_frame_duration,omitempty"`
	SampleCount           int            `json:"sample_count,omitempty"`
	MediaTimescale        uint32         `json:"media_timescale,omitempty"`
	MovieTimescale        uint32         `json:"movie_timescale,omitempty"`
	FirstDTS              Rat            `json:"first_dts,omitempty"`
	FirstPTS              Rat            `json:"first_pts,omitempty"`
	Edits                 []EditFact     `json:"edits,omitempty"`
	KeyframeSamples       []int          `json:"keyframe_samples,omitempty"`
	NonIDRSamples         []int          `json:"non_idr_samples,omitempty"`
	UnknownIDRSamples     []int          `json:"unknown_idr_samples,omitempty"`
	Keyframes             []KeyframeFact `json:"keyframes,omitempty"`
	Warnings              []string       `json:"warnings,omitempty"`
}

// Load turns one input rung into normalized facts.
func (in RenditionInput) Load() (RenditionFacts, error) {
	if in.Path != "" {
		f, err := ParseMP4File(in.Path)
		if err != nil {
			return RenditionFacts{}, fmt.Errorf("rung %q: %w", in.IDOrPath(), err)
		}
		if in.ID != "" {
			f.ID = in.ID
		} else {
			f.ID = shortID(in.Path)
		}
		if in.Role != "" {
			f.Role = in.Role
		}
		return *f, nil
	}
	return in.evidenceFacts(), nil
}

func (in RenditionInput) IDOrPath() string {
	if in.ID != "" {
		return in.ID
	}
	if in.Path != "" {
		return in.Path
	}
	return "inline-rung"
}

func (in RenditionInput) evidenceFacts() RenditionFacts {
	f := RenditionFacts{
		ID:                    in.ID,
		Role:                  in.Role,
		Codec:                 in.Codec,
		CodecFamily:           codecFamily(in.Codec),
		Width:                 in.Width,
		Height:                in.Height,
		FPS:                   in.FPS,
		FrameDuration:         in.FrameDuration,
		VariableFrameDuration: in.VariableFrameDuration,
		SampleCount:           in.SampleCount,
		MediaTimescale:        in.MediaTimescale,
		MovieTimescale:        in.MovieTimescale,
		FirstDTS:              in.FirstDTS,
		FirstPTS:              in.FirstPTS,
		Edits:                 in.Edits,
		EditListPresent:       len(in.Edits) > 0,
		Warnings:              in.Warnings,
	}
	if f.ID == "" {
		f.ID = "inline-rung"
	}

	fd := in.FrameDuration
	if fd.N <= 0 && in.FPS.N > 0 {
		fd = NewRat(in.FPS.D, in.FPS.N)
		f.FrameDuration = fd
	}
	if f.FPS.N <= 0 && fd.N > 0 {
		f.FPS = NewRat(fd.D, fd.N)
	}

	nonIDR := map[int]bool{}
	for _, s := range in.NonIDRSamples {
		nonIDR[s] = true
	}
	unknownIDR := map[int]bool{}
	for _, s := range in.UnknownIDRSamples {
		unknownIDR[s] = true
	}

	if len(in.Keyframes) > 0 {
		f.Keyframes = append(f.Keyframes, in.Keyframes...)
		for i := range f.Keyframes {
			k := &f.Keyframes[i]
			if k.DisplayPTS.N == 0 && k.DisplayPTS.D == 0 {
				if d, ok := mapDisplayFull(k.PTS, in.Edits); ok {
					k.DisplayPTS = d
				} else {
					k.DisplayPTS = k.PTS
				}
			}
		}
	} else {
		samples := append([]int(nil), in.KeyframeSamples...)
		sort.Ints(samples)
		if f.SampleCount == 0 && len(samples) > 0 {
			f.SampleCount = samples[len(samples)-1] + 1
		}
		for _, idx := range samples {
			dts := ZeroRat()
			pts := in.FirstPTS
			if fd.N > 0 {
				dts = fd.Mul(NewRat(int64(idx), 1))
				pts = in.FirstPTS.Add(dts)
			}
			display, ok := mapDisplayFull(pts, in.Edits)
			if !ok {
				display = pts
			}
			k := KeyframeFact{
				Sample: idx, DTS: dts, PTS: pts, DisplayPTS: display, SyncSample: true,
			}
			switch {
			case unknownIDR[idx]:
				k.IDR = nil
			case nonIDR[idx]:
				b := false
				k.IDR = &b
			default:
				b := true
				k.IDR = &b
			}
			f.Keyframes = append(f.Keyframes, k)
		}
	}

	if d, ok := mapDisplayFull(in.FirstPTS, in.Edits); ok {
		f.FirstDisplayPTS = d
	} else {
		f.FirstDisplayPTS = in.FirstPTS
	}
	f.EarliestPTS = in.FirstPTS
	computeGOPs(&f)
	return f
}

func codecFamily(codec string) string {
	switch strings.ToLower(strings.TrimSpace(codec)) {
	case "avc1", "avc3", "h264", "x264":
		return "avc"
	case "hvc1", "hev1", "hevc", "h265", "x265":
		return "hevc"
	}
	return "unknown"
}

// LoadLadder merges a JSON manifest with extra positional media paths.
func LoadLadder(doc *LadderInput, extraPaths []string) ([]RenditionFacts, ConstraintOptions, error) {
	var opts ConstraintOptions
	var facts []RenditionFacts
	if doc != nil {
		if doc.Constraint != nil {
			opts = *doc.Constraint
		}
		for _, r := range doc.Renditions {
			f, err := r.Load()
			if err != nil {
				return nil, opts, err
			}
			facts = append(facts, f)
		}
		for _, p := range doc.Files {
			rf, err := ParseMP4File(p)
			if err != nil {
				return nil, opts, err
			}
			rf.ID = shortID(p)
			facts = append(facts, *rf)
		}
	}
	for _, p := range extraPaths {
		rf, err := ParseMP4File(p)
		if err != nil {
			return nil, opts, err
		}
		rf.ID = shortID(p)
		facts = append(facts, *rf)
	}
	return facts, opts, nil
}

func shortID(p string) string {
	base := filepath.Base(p)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// ScanMedia collects parseable ISO-BMFF files under a directory (non-recursive
// by default; recursive when recursive is true).
func ScanMedia(dir string, recursive bool) ([]string, error) {
	exts := map[string]bool{".mp4": true, ".m4v": true, ".mov": true}
	var out []string
	walk := func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if !recursive && path != dir {
				return filepath.SkipDir
			}
			return nil
		}
		if exts[strings.ToLower(filepath.Ext(path))] {
			out = append(out, path)
		}
		return nil
	}
	if err := filepath.WalkDir(dir, walk); err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}
