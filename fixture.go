package laddercheck

import "fmt"

// FixtureEdit is a version-0 edit list entry expressed in raw track units.
type FixtureEdit struct {
	Empty                bool
	SegmentDurationTicks uint32 // movie timescale units
	MediaTimeTicks       int32  // media timescale units (ignored when Empty)
}

// FixtureSpec describes a small synthetic-but-structurally-valid MP4 rung.
type FixtureSpec struct {
	ID                 string
	Role               string
	Width              uint16
	Height             uint16
	Family             string // "avc" (default) or "hevc"
	MovieTimescale     uint32 // default 1000
	MediaTimescale     uint32 // default 30000
	Frames             int
	FrameDurationTicks uint32 // default 1001 when media ts 30000? caller sets
	CTOConstant        int32
	KeyframeSamples    []int
	NonIDR             map[int]bool
	Edits              []FixtureEdit
}

// BuildFixtureMP4 serializes the spec to ISO-BMFF bytes. All samples live in a
// single chunk so stco/stsc stay minimal and exact.
func BuildFixtureMP4(spec FixtureSpec) ([]byte, error) {
	if spec.Frames <= 0 {
		return nil, fmt.Errorf("fixture: Frames must be > 0")
	}
	mts := spec.MediaTimescale
	if mts == 0 {
		mts = 30000
	}
	mvts := spec.MovieTimescale
	if mvts == 0 {
		mvts = 1000
	}
	fd := spec.FrameDurationTicks
	if fd == 0 {
		fd = 1001
	}
	family := spec.Family
	if family == "" {
		family = "avc"
	}
	w, h := spec.Width, spec.Height
	if w == 0 {
		w = 640
	}
	if h == 0 {
		h = 360
	}

	keySet := map[int]bool{}
	for _, k := range spec.KeyframeSamples {
		keySet[k] = true
	}

	var mdatPayload []byte
	sizes := make([]uint32, spec.Frames)
	for i := 0; i < spec.Frames; i++ {
		nal := fixtureNAL(family, keySet[i], spec.NonIDR[i])
		sample := make([]byte, 4+len(nal))
		sample[0] = byte(len(nal) >> 24)
		sample[1] = byte(len(nal) >> 16)
		sample[2] = byte(len(nal) >> 8)
		sample[3] = byte(len(nal))
		copy(sample[4:], nal)
		sizes[i] = uint32(len(sample))
		mdatPayload = append(mdatPayload, sample...)
	}

	mediaDuration := uint32(int64(spec.Frames) * int64(fd))
	movieDuration := uint32(int64(mediaDuration) * int64(mvts) / int64(mts))

	buildMoov := func(chunkOffset uint32) *fbox {
		stblKids := []*fbox{
			fixtureSTSD(family, w, h),
			fixtureSTTS(spec.Frames, fd),
		}
		if spec.CTOConstant != 0 {
			stblKids = append(stblKids, fixtureCTTS(spec.Frames, spec.CTOConstant))
		}
		if len(spec.KeyframeSamples) > 0 {
			stblKids = append(stblKids, fixtureSTSS(spec.KeyframeSamples))
		}
		stblKids = append(stblKids,
			fixtureSTSZ(sizes),
			fixtureSTSC(spec.Frames),
			fixtureSTCO(chunkOffset),
		)
		stbl := container("stbl", stblKids...)
		minf := container("minf", stbl)
		mdia := container("mdia", mdhdBox(mts, mediaDuration), hdlrBox(), minf)

		trakKids := []*fbox{tkhdBox(1, movieDuration, w, h)}
		if len(spec.Edits) > 0 {
			trakKids = append(trakKids, container("edts", fixtureELST(spec.Edits)))
		}
		trakKids = append(trakKids, mdia)
		trak := container("trak", trakKids...)
		return container("moov", mvhdBox(mvts, movieDuration), trak)
	}

	ftyp := ftypBox().encode()
	moovMeasure := buildMoov(0).encode()
	chunkOffset := uint32(len(ftyp) + len(moovMeasure) + 8)
	moov := buildMoov(chunkOffset).encode()

	out := append([]byte{}, ftyp...)
	out = append(out, moov...)
	mdatHeader := make([]byte, 8)
	mdatSize := uint32(8 + len(mdatPayload))
	mdatHeader[0] = byte(mdatSize >> 24)
	mdatHeader[1] = byte(mdatSize >> 16)
	mdatHeader[2] = byte(mdatSize >> 8)
	mdatHeader[3] = byte(mdatSize)
	copy(mdatHeader[4:8], []byte("mdat"))
	out = append(out, mdatHeader...)
	out = append(out, mdatPayload...)
	return out, nil
}

func fixtureNAL(family string, isKey, nonIDR bool) []byte {
	if family == "hevc" {
		var t int
		switch {
		case isKey && !nonIDR:
			t = 19 // IDR_W_RADL
		case isKey && nonIDR:
			t = 21 // CRA (open GOP)
		default:
			t = 1 // TRAIL_R
		}
		return []byte{byte(t << 1), 0x01, 0xAA, 0xBB}
	}
	var t byte
	switch {
	case isKey && !nonIDR:
		t = 0x65 // ref_idc=3, type 5 IDR
	case isKey && nonIDR:
		t = 0x41 // ref_idc=2, type 1 non-IDR sync
	default:
		t = 0x41 // P slice
	}
	return []byte{t, 0xAA, 0xBB}
}

func fixtureSTSD(family string, w, h uint16) *fbox {
	entryType := "avc1"
	if family == "hevc" {
		entryType = "hvc1"
	}
	ew := &fbw{}
	ew.bytes(make([]byte, 6)) // reserved
	ew.u16(1)                 // data reference index
	ew.u16(0)
	ew.u16(0)
	ew.u32(0)
	ew.u32(0)
	ew.u32(0) // pre-defined/reserved 16 bytes total
	ew.u16(w)
	ew.u16(h)
	ew.u32(0x00480000) // horiz resolution 72 dpi
	ew.u32(0x00480000) // vert resolution
	ew.u32(0)
	ew.u16(1) // frame count
	name := make([]byte, 32)
	copy(name, []byte("LadderCheck"))
	ew.bytes(name)
	ew.u16(0x0018) // depth
	ew.u16(0xFFFF) // pre-defined -1

	var codecBox *fbox
	if family == "hevc" {
		cfg := make([]byte, 23)
		cfg[0] = 1
		cfg[21] = 0xFF // lengthSizeMinusOne = 3 -> 4-byte lengths
		codecBox = boxBytes("hvcC", cfg)
	} else {
		cw := &fbw{}
		cw.u8(1)
		cw.u8(0x42)
		cw.u8(0x00)
		cw.u8(0x1E)
		cw.u8(0xFF) // 4-byte NAL lengths
		cw.u8(0xE1)
		sps := []byte{0x67, 0x42, 0x00, 0x1E}
		cw.u16(uint16(len(sps)))
		cw.bytes(sps)
		cw.u8(1)
		pps := []byte{0x68, 0xCE, 0x38, 0x80}
		cw.u16(uint16(len(pps)))
		cw.bytes(pps)
		codecBox = boxBytes("avcC", cw.b)
	}
	// The sample entry keeps fixed binary fields as a prefix followed by the
	// decoder-config child box.
	entry := rawWithChildren(entryType, ew.b, codecBox)

	body := &fbw{}
	body.u32(1) // entry count
	stsdPayload := append(body.b, entry.encode()...)
	return fullBox("stsd", 0, 0, stsdPayload)
}

// rawWithChildren creates a box whose payload is fixed binary fields followed
// by one or more child boxes.
func rawWithChildren(typ string, prefix []byte, kids ...*fbox) *fbox {
	b := &fbox{typ: typ}
	b.body = prefix
	for _, k := range kids {
		b.body = append(b.body, k.encode()...)
	}
	return b
}

func fixtureSTTS(frames int, delta uint32) *fbox {
	w := &fbw{}
	w.u32(1)
	w.u32(uint32(frames))
	w.u32(delta)
	return fullBox("stts", 0, 0, w.b)
}

func fixtureCTTS(frames int, offset int32) *fbox {
	ver := byte(0)
	if offset < 0 {
		ver = 1
	}
	w := &fbw{}
	w.u32(1)
	w.u32(uint32(frames))
	w.u32(uint32(offset))
	return fullBox("ctts", ver, 0, w.b)
}

func fixtureSTSS(samples []int) *fbox {
	w := &fbw{}
	w.u32(uint32(len(samples)))
	for _, s := range samples {
		w.u32(uint32(s + 1))
	}
	return fullBox("stss", 0, 0, w.b)
}

func fixtureSTSZ(sizes []uint32) *fbox {
	w := &fbw{}
	w.u32(0)
	w.u32(uint32(len(sizes)))
	for _, sz := range sizes {
		w.u32(sz)
	}
	return fullBox("stsz", 0, 0, w.b)
}

func fixtureSTSC(frames int) *fbox {
	w := &fbw{}
	w.u32(1)
	w.u32(1)
	w.u32(uint32(frames))
	w.u32(1)
	return fullBox("stsc", 0, 0, w.b)
}

func fixtureSTCO(offset uint32) *fbox {
	w := &fbw{}
	w.u32(1)
	w.u32(offset)
	return fullBox("stco", 0, 0, w.b)
}

func fixtureELST(edits []FixtureEdit) *fbox {
	w := &fbw{}
	w.u32(uint32(len(edits)))
	for _, e := range edits {
		w.u32(e.SegmentDurationTicks)
		if e.Empty {
			w.u32(0xFFFFFFFF) // media_time = -1
		} else {
			w.u32(uint32(e.MediaTimeTicks))
		}
		w.u16(1) // media rate integer
		w.u16(0) // fraction
	}
	return fullBox("elst", 0, 0, w.b)
}
