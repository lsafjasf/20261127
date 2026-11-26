package manifest

import "subdeliver/internal/subtitle"

// CueSet is one track converted into canonical cues.
type CueSet struct {
	Language string
	Label    string
	Default  bool
	Cues     []subtitle.Cue
}

// Build converts every manifest track into format-independent canonical cues.
// All renderers (SRT, VTT, embedded) consume this exact same output, which is
// what guarantees the two deliverables originate from one source.
func (m *Manifest) Build() ([]CueSet, error) {
	sets := make([]CueSet, 0, len(m.Tracks))
	for ti := range m.Tracks {
		t := &m.Tracks[ti]
		set := CueSet{
			Language: t.Language,
			Label:    t.Label,
			Default:  t.Default,
			Cues:     make([]subtitle.Cue, 0, len(t.Cues)),
		}
		for ci := range t.Cues {
			src := &t.Cues[ci]
			start, err := subtitle.ParseTime(src.Start)
			if err != nil {
				return nil, err
			}
			end, err := subtitle.ParseTime(src.End)
			if err != nil {
				return nil, err
			}
			set.Cues = append(set.Cues, subtitle.Cue{
				Index:    ci + 1,
				ID:       src.ID,
				Start:    start,
				End:      end,
				Text:     subtitle.NormalizeText(src.Text),
				Language: t.Language,
			})
		}
		sets = append(sets, set)
	}
	return sets, nil
}
