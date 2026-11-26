package laddercheck

import "sort"

func (vt *videoTrack) sampleOffsets() []int64 {
	n := vt.sampleCount
	if n > 0 && len(vt.fragOffsets) == n {
		return vt.fragOffsets
	}
	if n == 0 || len(vt.chunks) == 0 || len(vt.stsc) == 0 || len(vt.sizes) < n {
		return nil
	}
	offs := make([]int64, n)
	cursor := 0
	entry := 0
	for c := 0; c < len(vt.chunks); c++ {
		chunk1 := uint32(c + 1)
		for entry+1 < len(vt.stsc) && vt.stsc[entry+1].FirstChunk <= chunk1 {
			entry++
		}
		perChunk := int(vt.stsc[entry].SamplesPerChunk)
		pos := vt.chunks[c]
		for s := 0; s < perChunk && cursor < n; s++ {
			offs[cursor] = pos
			if cursor < len(vt.sizes) {
				pos += int64(vt.sizes[cursor])
			}
			cursor++
		}
	}
	if cursor != n {
		vt.warnings = append(vt.warnings, "stsc/stco expansion covered fewer samples than stts/stsz")
		return nil
	}
	return offs
}

// mapDisplay maps a media-timeline time to movie/display time through elst.
// The boolean reports whether the time falls inside a real (non-empty) edit.
func mapDisplayFull(t Rat, edits []EditFact) (Rat, bool) {
	if len(edits) == 0 {
		return t, true
	}
	movieStart := ZeroRat()
	var mappedBeforeEnd *Rat
	for i := range edits {
		e := edits[i]
		if e.Empty || e.MediaTime == nil {
			movieStart = movieStart.Add(e.SegmentDuration)
			continue
		}
		m := *e.MediaTime
		rate := e.MediaRate
		if rate.N == 0 {
			rate = NewRat(1, 1)
		}
		mediaDur := e.SegmentDuration.Quo(rate)
		end := m.Add(mediaDur)
		if t.Cmp(m) >= 0 {
			mapped := movieStart.Add(t.Sub(m).Quo(rate))
			if t.Cmp(end) <= 0 {
				return mapped, true
			}
			if mappedBeforeEnd == nil {
				v := mapped
				mappedBeforeEnd = &v
			}
		}
		movieStart = movieStart.Add(e.SegmentDuration)
	}
	if mappedBeforeEnd != nil {
		return *mappedBeforeEnd, true
	}
	return t, false
}

func (vt *videoTrack) toFacts(rd *readerAt) (*RenditionFacts, error) {
	mediaTS := vt.mediaTS
	if mediaTS == 0 {
		mediaTS = vt.movieTS
	}
	if mediaTS == 0 {
		mediaTS = 1
		vt.warnings = append(vt.warnings, "media timescale missing; using 1")
	}

	n := vt.sampleCount
	ptsTicks := make([]int64, n)
	var earliest int64
	for i := 0; i < n; i++ {
		dts := int64(0)
		if i < len(vt.dtsTicks) {
			dts = vt.dtsTicks[i]
		}
		cto := int64(0)
		if i < len(vt.ctoTicks) {
			cto = vt.ctoTicks[i]
		}
		ptsTicks[i] = dts + cto
		if i == 0 || ptsTicks[i] < earliest {
			earliest = ptsTicks[i]
		}
	}

	facts := &RenditionFacts{
		Container:       "iso-bmff",
		Codec:           vt.codec,
		CodecFamily:     vt.family,
		Width:           int(vt.width),
		Height:          int(vt.height),
		MovieTimescale:  vt.movieTS,
		MediaTimescale:  vt.mediaTS,
		SampleCount:     n,
		FirstDTS:        RatFromTicks(firstOrZero(vt.dtsTicks), mediaTS),
		FirstPTS:        RatFromTicks(firstOrZero(ptsTicks), mediaTS),
		EarliestPTS:     RatFromTicks(earliest, mediaTS),
		EditListPresent: vt.elst != nil,
		Edits:           vt.edits,
		GOPUniform:      true,
		Warnings:        dedupeStrings(vt.warnings),
	}

	vt.deriveFrameRate(facts, mediaTS)

	offsets := vt.sampleOffsets()

	// Resolve the zero-based keyframe sample indices.
	var keySamples []int
	if vt.haveSTSS {
		seen := map[int]bool{}
		for _, s1 := range vt.sync1b {
			idx := int(s1) - 1
			if idx >= 0 && idx < n && !seen[idx] {
				seen[idx] = true
				keySamples = append(keySamples, idx)
			}
		}
		sort.Ints(keySamples)
	} else {
		// ISO-BMFF: absent stss means every sample is a sync sample.
		for i := 0; i < n; i++ {
			keySamples = append(keySamples, i)
		}
		facts.Warnings = append(facts.Warnings, "stss absent; treating every sample as sync per ISO-BMFF")
	}

	for _, idx := range keySamples {
		pts := RatFromTicks(ptsTicks[idx], mediaTS)
		dts := RatFromTicks(vt.dtsTicks[idx], mediaTS)
		display, _ := mapDisplayFull(pts, vt.edits)
		kf := KeyframeFact{
			Sample:     idx,
			DTS:        dts,
			PTS:        pts,
			DisplayPTS: display,
			SyncSample: true,
		}
		if idx < len(vt.sizes) {
			kf.Size = int(vt.sizes[idx])
		}
		if offsets != nil && vt.family != "unknown" && idx < len(offsets) && kf.Size > 0 {
			if raw, err := rd.bytes(offsets[idx], int64(kf.Size)); err == nil {
				idr, nalTypes := classifyNAL(vt.family, raw, vt.nalLen)
				kf.IDR = idr
				kf.NALTypes = nalTypes
			} else {
				facts.Warnings = append(facts.Warnings, "could not read sample for IDR classification")
			}
		}
		facts.Keyframes = append(facts.Keyframes, kf)
	}

	// First presented frame on the movie timeline.
	firstDisplay := RatFromTicks(earliest, mediaTS)
	foundDisplay := false
	for _, pt := range ptsTicks {
		p := RatFromTicks(pt, mediaTS)
		if d, ok := mapDisplayFull(p, vt.edits); ok {
			if !foundDisplay || d.Cmp(firstDisplay) < 0 {
				firstDisplay = d
				foundDisplay = true
			}
		}
	}
	if d, ok := mapDisplayFull(facts.FirstPTS, vt.edits); ok && !foundDisplay {
		firstDisplay = d
	}
	facts.FirstDisplayPTS = firstDisplay

	computeGOPs(facts)
	return facts, nil
}

func (vt *videoTrack) deriveFrameRate(facts *RenditionFacts, mediaTS uint32) {
	if len(vt.dtsTicks) < 2 {
		return
	}
	deltas := map[int64]int{}
	var common int64
	uniform := true
	for i := 1; i < len(vt.dtsTicks); i++ {
		d := vt.dtsTicks[i] - vt.dtsTicks[i-1]
		if d <= 0 {
			continue
		}
		deltas[d]++
		if common == 0 {
			common = d
		} else if d != common {
			uniform = false
		}
	}
	if uniform && common > 0 {
		facts.FrameDuration = RatFromTicks(common, mediaTS)
		facts.FPS = NewRat(int64(mediaTS), common)
		return
	}
	facts.VariableFrameDuration = true
	first := vt.dtsTicks[0]
	last := vt.dtsTicks[len(vt.dtsTicks)-1]
	span := last - first
	if span > 0 && len(vt.dtsTicks) > 1 {
		den := int64(len(vt.dtsTicks)-1) * int64(mediaTS)
		facts.FrameDuration = NewRat(span, den)
		facts.FPS = NewRat(den, span)
	}
}

func computeGOPs(facts *RenditionFacts) {
	kfs := facts.Keyframes
	uniform := true
	var mediaGap Rat
	for i := 1; i < len(kfs); i++ {
		dm := kfs[i].PTS.Sub(kfs[i-1].PTS)
		dd := kfs[i].DisplayPTS.Sub(kfs[i-1].DisplayPTS)
		facts.GOPMediaDurations = append(facts.GOPMediaDurations, dm)
		facts.GOPDisplayDurations = append(facts.GOPDisplayDurations, dd)
		if i == 1 {
			mediaGap = dm
		} else if !dm.Equal(mediaGap) {
			uniform = false
		}
	}
	// A single keyframe gives no GOP evidence; do not claim uniformity.
	facts.GOPUniform = len(kfs) >= 2 && uniform
}

func firstOrZero(s []int64) int64 {
	if len(s) == 0 {
		return 0
	}
	return s[0]
}

func dedupeStrings(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
