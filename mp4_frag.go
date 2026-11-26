package laddercheck

// parseFragments populates sample tables from moof/traf/trun boxes for
// fragmented MP4/CMAF. Progressive files have no top-level moof and return.
func (vt *videoTrack) parseFragments(rd *readerAt, root []*Box, moov *Box) error {
	var moofs []*Box
	for _, b := range root {
		if b.Type == "moof" {
			moofs = append(moofs, b)
		}
	}
	if len(moofs) == 0 {
		return nil
	}

	var trexDur, trexSize, trexFlags uint32
	if mvex := moov.child("mvex"); mvex != nil {
		if trex := mvex.child("trex"); trex != nil {
			p := trex.Payload
			trexDur, _ = rd.u32(p + 12)
			trexSize, _ = rd.u32(p + 16)
			trexFlags, _ = rd.u32(p + 20)
		}
	}

	for _, moof := range moofs {
		traf := moof.child("traf")
		if traf == nil {
			continue
		}
		_, tfhdFlags, tfhdBase, tfhdDur, tfhdSize, tfhdSFlags, tfhdHasBase, tfhdHasDef :=
			parseTFHDDefaults(rd, traf.child("tfhd"))

		baseDTS := parseTFDT(rd, traf.child("tfdt"))

		for _, trun := range traf.childrenOf("trun") {
			vt.parseTRUN(rd, trun, moof, tfhdFlags, tfhdBase, tfhdHasBase,
				tfhdDur, tfhdSize, tfhdSFlags, tfhdHasDef,
				baseDTS, trexDur, trexSize, trexFlags)
		}
	}

	vt.sampleCount = len(vt.dtsTicks)
	if len(vt.sync1b) > 0 {
		vt.haveSTSS = true
	} else {
		// Fragmented streams signal sync via sample flags; if none were found
		// keep haveSTSS false so the spec fallback does not mislabel samples.
		vt.warnings = append(vt.warnings, "no sync samples resolved from fragment flags")
	}
	return nil
}

func parseTFHDDefaults(rd *readerAt, tfhd *Box) (trackID uint32, flags uint32,
	base uint64, defDur, defSize, defFlags uint32, hasBase bool, hasDef bool) {
	if tfhd == nil {
		return
	}
	fl, err := rd.bytes(tfhd.Payload+1, 3)
	if err != nil {
		return
	}
	flags = uint32(fl[0])<<16 | uint32(fl[1])<<8 | uint32(fl[2])
	trackID, _ = rd.u32(tfhd.Payload + 4)
	pos := tfhd.Payload + 8
	if flags&0x000001 != 0 {
		base, _ = rd.u64(pos)
		hasBase = true
		pos += 8
	}
	if flags&0x000002 != 0 {
		pos += 4 // sample description index
	}
	if flags&0x000008 != 0 {
		defDur, _ = rd.u32(pos)
		hasDef = true
		pos += 4
	}
	if flags&0x000010 != 0 {
		defSize, _ = rd.u32(pos)
		hasDef = true
		pos += 4
	}
	if flags&0x000020 != 0 {
		defFlags, _ = rd.u32(pos)
		hasDef = true
	}
	return
}

func parseTFDT(rd *readerAt, tfdt *Box) int64 {
	if tfdt == nil {
		return 0
	}
	version, err := rd.u8(tfdt.Payload)
	if err != nil {
		return 0
	}
	if version == 1 {
		v, _ := rd.u64(tfdt.Payload + 4)
		return int64(v)
	}
	v, _ := rd.u32(tfdt.Payload + 4)
	return int64(v)
}

func (vt *videoTrack) parseTRUN(rd *readerAt, trun *Box, moof *Box,
	tfhdFlags uint32, tfhdBase uint64, tfhdHasBase bool,
	tfhdDur, tfhdSize, tfhdSFlags uint32, tfhdHasDef bool,
	baseDTS int64, trexDur, trexSize, trexFlags uint32) {

	fl, err := rd.bytes(trun.Payload+1, 3)
	if err != nil {
		return
	}
	version, _ := rd.u8(trun.Payload)
	flags := uint32(fl[0])<<16 | uint32(fl[1])<<8 | uint32(fl[2])
	count, err := rd.u32(trun.Payload + 4)
	if err != nil {
		return
	}
	pos := trun.Payload + 8
	var dataOffset int32
	if flags&0x000001 != 0 {
		raw, _ := rd.u32(pos)
		dataOffset = int32(raw)
		pos += 4
	}
	var firstFlags uint32
	haveFirstFlags := false
	if flags&0x000004 != 0 {
		firstFlags, _ = rd.u32(pos)
		haveFirstFlags = true
		pos += 4
	}

	var baseData int64
	switch {
	case tfhdHasBase:
		baseData = int64(tfhdBase)
	case tfhdFlags&0x020000 != 0:
		baseData = moof.Start
	default:
		baseData = moof.Start
	}
	dataPos := baseData + int64(dataOffset)
	dts := baseDTS

	for i := uint32(0); i < count; i++ {
		var dur, size, sflags uint32
		var cto int64
		if flags&0x000100 != 0 {
			dur, _ = rd.u32(pos)
			pos += 4
		} else if tfhdHasDef {
			dur = tfhdDur
		} else {
			dur = trexDur
		}
		if flags&0x000200 != 0 {
			size, _ = rd.u32(pos)
			pos += 4
		} else if tfhdHasDef {
			size = tfhdSize
		} else {
			size = trexSize
		}
		if flags&0x000400 != 0 {
			sflags, _ = rd.u32(pos)
			pos += 4
		} else if i == 0 && haveFirstFlags {
			sflags = firstFlags
		} else if tfhdFlags&0x000020 != 0 {
			sflags = tfhdSFlags
		} else {
			sflags = trexFlags
		}
		if flags&0x000800 != 0 {
			raw, _ := rd.u32(pos)
			if version == 1 {
				cto = int64(int32(raw))
			} else {
				cto = int64(raw)
			}
			pos += 4
		}

		globalIndex := len(vt.dtsTicks)
		vt.dtsTicks = append(vt.dtsTicks, dts)
		vt.ctoTicks = append(vt.ctoTicks, cto)
		vt.sizes = append(vt.sizes, size)
		vt.fragOffsets = append(vt.fragOffsets, dataPos)
		if sflags&0x00010000 == 0 { // sample_is_non_sync_sample clear => sync
			vt.sync1b = append(vt.sync1b, uint32(globalIndex+1))
		}
		dataPos += int64(size)
		dts += int64(dur)
	}
}
