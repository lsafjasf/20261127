package laddercheck

import "fmt"

func (vt *videoTrack) parseSTTS(rd *readerAt, stts *Box) error {
	if stts == nil {
		return fmt.Errorf("stts missing")
	}
	count, err := rd.u32(stts.Payload + 4)
	if err != nil {
		return err
	}
	p := stts.Payload + 8
	dts := int64(0)
	ticks := make([]int64, 0)
	for i := uint32(0); i < count; i++ {
		sampleCount, err1 := rd.u32(p)
		delta, err2 := rd.u32(p + 4)
		if err1 != nil || err2 != nil {
			return fmt.Errorf("truncated stts")
		}
		for j := uint32(0); j < sampleCount; j++ {
			ticks = append(ticks, dts)
			dts += int64(delta)
		}
		p += 8
	}
	vt.dtsTicks = ticks
	vt.sampleCount = len(ticks)
	vt.ctoTicks = make([]int64, vt.sampleCount)
	return nil
}

func (vt *videoTrack) parseCTTS(rd *readerAt, ctts *Box) {
	if ctts == nil || vt.sampleCount == 0 {
		return
	}
	version, err := rd.u8(ctts.Payload)
	if err != nil {
		return
	}
	if version == 1 {
		vt.haveCTOv1 = true
	}
	count, err := rd.u32(ctts.Payload + 4)
	if err != nil {
		return
	}
	p := ctts.Payload + 8
	idx := 0
	for i := uint32(0); i < count && idx < vt.sampleCount; i++ {
		n, err1 := rd.u32(p)
		offRaw, err2 := rd.u32(p + 4)
		if err1 != nil || err2 != nil {
			return
		}
		off := int64(offRaw)
		if version == 1 {
			off = int64(int32(offRaw))
		}
		for j := uint32(0); j < n && idx < vt.sampleCount; j++ {
			vt.ctoTicks[idx] = off
			idx++
		}
		p += 8
	}
}

func (vt *videoTrack) parseSTSS(rd *readerAt, stss *Box) {
	if stss == nil {
		return
	}
	vt.haveSTSS = true
	count, err := rd.u32(stss.Payload + 4)
	if err != nil {
		return
	}
	p := stss.Payload + 8
	for i := uint32(0); i < count; i++ {
		n, err := rd.u32(p)
		if err != nil {
			return
		}
		vt.sync1b = append(vt.sync1b, n)
		p += 4
	}
}

func (vt *videoTrack) parseSTSZ(rd *readerAt, stsz, stz2 *Box) error {
	if stsz != nil {
		uniform, err := rd.u32(stsz.Payload + 4)
		if err != nil {
			return err
		}
		count, err := rd.u32(stsz.Payload + 8)
		if err != nil {
			return err
		}
		if vt.sampleCount == 0 {
			vt.sampleCount = int(count)
			vt.ctoTicks = make([]int64, vt.sampleCount)
			vt.dtsTicks = make([]int64, vt.sampleCount)
		}
		vt.sizes = make([]uint32, count)
		if uniform != 0 {
			for i := range vt.sizes {
				vt.sizes[i] = uniform
			}
			return nil
		}
		p := stsz.Payload + 12
		for i := uint32(0); i < count; i++ {
			sz, err := rd.u32(p)
			if err != nil {
				return err
			}
			vt.sizes[i] = sz
			p += 4
		}
		return nil
	}
	if stz2 != nil {
		vt.warnings = append(vt.warnings, "stz2 compact sizes are not decoded; IDR bitstream check disabled")
		return nil
	}
	vt.warnings = append(vt.warnings, "stsz missing; sample offsets/IDR classification unavailable")
	return nil
}

func (vt *videoTrack) parseSTSC(rd *readerAt, stsc *Box) {
	if stsc == nil {
		return
	}
	count, err := rd.u32(stsc.Payload + 4)
	if err != nil {
		return
	}
	p := stsc.Payload + 8
	for i := uint32(0); i < count; i++ {
		fc, e1 := rd.u32(p)
		spc, e2 := rd.u32(p + 4)
		sdi, e3 := rd.u32(p + 8)
		if e1 != nil || e2 != nil || e3 != nil {
			return
		}
		vt.stsc = append(vt.stsc, stscEntry{FirstChunk: fc, SamplesPerChunk: spc, SampleDescIndex: sdi})
		p += 12
	}
}

func (vt *videoTrack) parseChunkOffsets(rd *readerAt, stco, co64 *Box) error {
	switch {
	case stco != nil:
		count, err := rd.u32(stco.Payload + 4)
		if err != nil {
			return err
		}
		p := stco.Payload + 8
		for i := uint32(0); i < count; i++ {
			off, err := rd.u32(p)
			if err != nil {
				return err
			}
			vt.chunks = append(vt.chunks, int64(off))
			p += 4
		}
	case co64 != nil:
		count, err := rd.u32(co64.Payload + 4)
		if err != nil {
			return err
		}
		p := co64.Payload + 8
		for i := uint32(0); i < count; i++ {
			off, err := rd.u64(p)
			if err != nil {
				return err
			}
			vt.chunks = append(vt.chunks, int64(off))
			p += 8
		}
	default:
		return fmt.Errorf("neither stco nor co64 present")
	}
	return nil
}

func parseELST(rd *readerAt, elst *Box, movieTS, mediaTS uint32) ([]EditFact, error) {
	version, err := rd.u8(elst.Payload)
	if err != nil {
		return nil, err
	}
	count, err := rd.u32(elst.Payload + 4)
	if err != nil {
		return nil, err
	}
	if movieTS == 0 {
		movieTS = 1000
	}
	if mediaTS == 0 {
		mediaTS = movieTS
	}
	entrySize := int64(12)
	p := elst.Payload + 8
	if version == 1 {
		entrySize = 20
	}
	edits := make([]EditFact, 0, count)
	for i := uint32(0); i < count; i++ {
		var segDurTicks uint64
		var mediaTimeTicks int64
		var rateInt, rateFrac uint16
		if version == 1 {
			segDurTicks, err = rd.u64(p)
			if err != nil {
				return nil, err
			}
			mt, err := rd.u64(p + 8)
			if err != nil {
				return nil, err
			}
			mediaTimeTicks = int64(mt)
			rateInt, _ = rd.u16(p + 16)
			rateFrac, _ = rd.u16(p + 18)
		} else {
			sd, err := rd.u32(p)
			if err != nil {
				return nil, err
			}
			segDurTicks = uint64(sd)
			mt, err := rd.u32(p + 4)
			if err != nil {
				return nil, err
			}
			mediaTimeTicks = int64(int32(mt))
			rateInt, _ = rd.u16(p + 8)
			rateFrac, _ = rd.u16(p + 10)
		}
		ef := EditFact{
			SegmentDuration:   RatFromTicks(int64(segDurTicks), movieTS),
			RawMediaTimeTicks: mediaTimeTicks,
			MediaRate:         NewRat(int64(rateInt)*65536+int64(rateFrac), 65536),
		}
		if mediaTimeTicks == -1 {
			ef.Empty = true
			ef.MediaTime = nil
		} else {
			mt := RatFromTicks(mediaTimeTicks, mediaTS)
			ef.MediaTime = &mt
		}
		edits = append(edits, ef)
		p += entrySize
	}
	return edits, nil
}
