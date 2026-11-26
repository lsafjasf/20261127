package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

// Clip is one segment on the finished timeline (sequence).
// Sequence frames are [SeqIn, SeqOut); SrcIn/SrcOut describe the source
// frames landing on SeqIn/SeqOut, so any constant speed (including
// reverse, where SrcOut < SrcIn) is representable.
type Clip struct {
	ID        string `json:"id"`
	Name      string `json:"name"` // source file name as referenced by the edit
	SeqIn     int64  `json:"seqIn"`
	SeqOut    int64  `json:"seqOut"`
	SrcIn     int64  `json:"srcIn"`
	SrcOut    int64  `json:"srcOut"`
	FirstHash uint64 `json:"firstHash"` // fingerprint of first timeline frame
	LastHash  uint64 `json:"lastHash"`  // fingerprint of last timeline frame
}

// FrameHash is one entry of a sparse fingerprint index over a source file.
type FrameHash struct {
	Frame int64  `json:"frame"`
	Hash  uint64 `json:"hash"`
}

// Source is one media file in the pool. Many sources may share Name.
type Source struct {
	Path   string      `json:"path"`
	Name   string      `json:"name"`
	Frames []FrameHash `json:"frames"`
}

func (s *Source) hashAt(frame int64) (uint64, bool) {
	for _, fh := range s.Frames {
		if fh.Frame == frame {
			return fh.Hash, true
		}
	}
	return 0, false
}

// rate returns source frames per sequence frame; negative means reverse.
func (c *Clip) rate() float64 {
	return float64(c.SrcOut-c.SrcIn) / float64(c.SeqOut-c.SeqIn)
}

// MapSeqToSrc converts a sequence (timeline) frame to a source frame,
// honouring the clip's speed change. It is the inverse lookup the
// director asks for: "which source frame sits at timeline frame t?".
func (c *Clip) MapSeqToSrc(seqFrame int64) (int64, error) {
	if seqFrame < c.SeqIn || seqFrame >= c.SeqOut {
		return 0, fmt.Errorf("sequence frame %d outside clip %s [%d,%d)", seqFrame, c.ID, c.SeqIn, c.SeqOut)
	}
	src := float64(c.SrcIn) + float64(seqFrame-c.SeqIn)*c.rate()
	return int64(src + 0.5), nil // round to nearest source frame
}

// endpointSrcFrames returns the source frames that the first and last
// timeline frames of the clip were sampled from.
func (c *Clip) endpointSrcFrames() (first, last int64, err error) {
	first, err = c.MapSeqToSrc(c.SeqIn)
	if err != nil {
		return 0, 0, err
	}
	last, err = c.MapSeqToSrc(c.SeqOut - 1)
	if err != nil {
		return 0, 0, err
	}
	return first, last, nil
}

// matchCandidates returns every pool source whose fingerprints confirm
// BOTH endpoint frames. File name is deliberately ignored here: names
// collide, only frame fingerprints prove identity.
func matchCandidates(c *Clip, pool []Source) ([]Source, error) {
	first, last, err := c.endpointSrcFrames()
	if err != nil {
		return nil, err
	}
	var out []Source
	for _, s := range pool {
		fh, ok := s.hashAt(first)
		if !ok || fh != c.FirstHash {
			continue
		}
		lh, ok := s.hashAt(last)
		if !ok || lh != c.LastHash {
			continue
		}
		out = append(out, s)
	}
	return out, nil
}

// ClipReport is the per-clip matching verdict.
type ClipReport struct {
	Clip       Clip     `json:"clip"`
	FirstSrc   int64    `json:"firstSrcFrame"`
	LastSrc    int64    `json:"lastSrcFrame"`
	Rate       float64  `json:"rate"`
	Status     string   `json:"status"` // unique | ambiguous | unmatched
	Candidates []string `json:"candidates"`
	NameAgrees bool     `json:"nameAgrees"` // informational only
}

func matchAll(clips []Clip, pool []Source) []ClipReport {
	reports := make([]ClipReport, 0, len(clips))
	for _, c := range clips {
		r := ClipReport{Clip: c, Rate: c.rate()}
		first, last, err := c.endpointSrcFrames()
		if err != nil {
			r.Status = "unmatched"
			reports = append(reports, r)
			continue
		}
		r.FirstSrc, r.LastSrc = first, last
		cands, err := matchCandidates(&c, pool)
		if err != nil {
			r.Status = "unmatched"
			reports = append(reports, r)
			continue
		}
		for _, s := range cands {
			r.Candidates = append(r.Candidates, s.Path)
		}
		sort.Strings(r.Candidates)
		switch len(cands) {
		case 0:
			r.Status = "unmatched"
		case 1:
			r.Status = "unique"
			r.NameAgrees = cands[0].Name == c.Name
		default:
			r.Status = "ambiguous"
		}
		reports = append(reports, r)
	}
	return reports
}

func loadJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}
