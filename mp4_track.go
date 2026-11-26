package laddercheck

import (
	"bytes"
	"fmt"
)

type stscEntry struct {
	FirstChunk      uint32
	SamplesPerChunk uint32
	SampleDescIndex uint32
}

type videoTrack struct {
	movieTS uint32
	mediaTS uint32
	width   uint16
	height  uint16
	codec   string
	family  string
	nalLen  int

	sampleCount int
	dtsTicks    []int64
	ptsTicks    []int64
	ctoTicks    []int64
	haveCTOv1   bool

	haveSTSS bool
	sync1b   []uint32 // 1-based sample numbers

	sizes  []uint32
	stsc   []stscEntry
	chunks []int64

	fragOffsets []int64

	elst     *Box
	edits    []EditFact
	warnings []string
}

func factsFromTree(rd *readerAt, root []*Box, size int64) (*RenditionFacts, error) {
	moov := findTop(root, "moov")
	if moov == nil {
		// Could be a fragmented segment with no moov here; surface a typed hint.
		if findTop(root, "moof") != nil {
			return nil, fmt.Errorf("laddercheck: fragmented MP4 segment without moov is not supported by this entry point")
		}
		return nil, fmt.Errorf("laddercheck: no moov box found (not an ISO-BMFF/MP4 file?)")
	}

	movieTS, err := parseMVHDTimescale(rd, moov)
	if err != nil {
		return nil, err
	}

	traks := moov.childrenOf("trak")
	var vt *videoTrack
	for _, trak := range traks {
		hdlr := trak.path("mdia", "hdlr")
		if hdlr == nil {
			continue
		}
		handler, err := parseHDLR(rd, hdlr)
		if err != nil || handler != "vide" {
			continue
		}
		candidate, err := parseVideoTrak(rd, trak, movieTS)
		if err != nil {
			return nil, err
		}
		if err := candidate.parseFragments(rd, root, moov); err != nil {
			candidate.warnings = append(candidate.warnings, "fragments: "+err.Error())
		}
		vt = candidate
		break
	}
	if vt == nil {
		return nil, fmt.Errorf("laddercheck: no video track found")
	}

	facts, err := vt.toFacts(rd)
	if err != nil {
		return nil, err
	}
	if ftyp := findTop(root, "ftyp"); ftyp != nil && ftyp.Payload+4 <= ftyp.End {
		if brand, err := rd.bytes(ftyp.Payload, 4); err == nil {
			facts.Brand = string(brand)
		}
	}
	return facts, nil
}

func findTop(boxes []*Box, typ string) *Box {
	for _, b := range boxes {
		if b.Type == typ {
			return b
		}
	}
	return nil
}

func parseMVHDTimescale(rd *readerAt, moov *Box) (uint32, error) {
	mvhd := moov.child("mvhd")
	if mvhd == nil {
		return 0, fmt.Errorf("laddercheck: mvhd missing")
	}
	version, err := rd.u8(mvhd.Payload)
	if err != nil {
		return 0, err
	}
	// version(1)+flags(3); v0: creation(4)+mod(4)+timescale(4); v1: two u64.
	tsOff := mvhd.Payload + 4 + 8
	if version == 1 {
		tsOff = mvhd.Payload + 4 + 16
	}
	return rd.u32(tsOff)
}

func parseHDLR(rd *readerAt, hdlr *Box) (string, error) {
	// fullbox header 4 bytes; pre_defined 4; handler_type 4.
	b, err := rd.bytes(hdlr.Payload+8, 4)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func parseVideoTrak(rd *readerAt, trak *Box, movieTS uint32) (*videoTrack, error) {
	vt := &videoTrack{movieTS: movieTS, nalLen: 4}

	mdhd := trak.path("mdia", "mdhd")
	if mdhd != nil {
		version, err := rd.u8(mdhd.Payload)
		if err != nil {
			return nil, err
		}
		tsOff := mdhd.Payload + 4 + 8
		if version == 1 {
			tsOff = mdhd.Payload + 4 + 16
		}
		vt.mediaTS, err = rd.u32(tsOff)
		if err != nil {
			return nil, err
		}
	}

	tkhd := trak.child("tkhd")
	if tkhd != nil {
		// width/height 16.16 fixed point are the final 8 bytes of tkhd.
		w, err := rd.u32(tkhd.End - 8)
		if err == nil {
			vt.width = uint16(w >> 16)
		}
		h, err := rd.u32(tkhd.End - 4)
		if err == nil {
			vt.height = uint16(h >> 16)
		}
	}

	stsd := trak.path("mdia", "minf", "stbl", "stsd")
	if stsd != nil {
		if err := parseSTSD(rd, stsd, vt); err != nil {
			vt.warnings = append(vt.warnings, "stsd: "+err.Error())
		}
	}

	stbl := trak.path("mdia", "minf", "stbl")
	if stbl == nil {
		return nil, fmt.Errorf("laddercheck: stbl missing in video trak")
	}

	if err := vt.parseSTTS(rd, stbl.child("stts")); err != nil {
		// Fragmented MP4 may carry sample timing only in trun/tfdt.
		vt.warnings = append(vt.warnings, "stts: "+err.Error())
	}
	vt.parseCTTS(rd, stbl.child("ctts"))
	vt.parseSTSS(rd, stbl.child("stss"))
	if err := vt.parseSTSZ(rd, stbl.child("stsz"), stbl.child("stz2")); err != nil {
		return nil, err
	}
	vt.parseSTSC(rd, stbl.child("stsc"))
	if err := vt.parseChunkOffsets(rd, stbl.child("stco"), stbl.child("co64")); err != nil {
		vt.warnings = append(vt.warnings, "chunk offsets: "+err.Error())
	}

	edts := trak.child("edts")
	if edts != nil {
		vt.elst = edts.child("elst")
	}
	if vt.elst != nil {
		edits, err := parseELST(rd, vt.elst, vt.movieTS, vt.mediaTS)
		if err != nil {
			vt.warnings = append(vt.warnings, "elst: "+err.Error())
		} else {
			vt.edits = edits
		}
	}

	return vt, nil
}

func parseSTSD(rd *readerAt, stsd *Box, vt *videoTrack) error {
	// stsd is a full box: 4 bytes version/flags + u32 entry_count.
	entryType, err := rd.bytes(stsd.Payload+8+4, 4)
	if err != nil {
		return err
	}
	vt.codec = string(entryType)
	switch vt.codec {
	case "avc1", "avc3":
		vt.family = "avc"
	case "hvc1", "hev1":
		vt.family = "hevc"
	default:
		vt.family = "unknown"
	}
	// VisualSampleEntry width/height: entry box start = Payload+8.
	entryStart := stsd.Payload + 8
	if w, err := rd.u16(entryStart + 32); err == nil && vt.width == 0 {
		vt.width = w
	}
	if h, err := rd.u16(entryStart + 34); err == nil && vt.height == 0 {
		vt.height = h
	}

	// Locate the decoder config record by four-CC and read nal length size.
	payload, err := rd.bytes(stsd.Payload, stsd.End-stsd.Payload)
	if err != nil {
		return nil // non-fatal: keep default nalLen=4
	}
	if idx := bytes.Index(payload, []byte("avcC")); idx >= 0 && idx+9 < len(payload) {
		// payload index points at 'a' of avcC; config payload starts idx+4;
		// lengthSizeMinusOne is config byte 4 (low 2 bits).
		vt.nalLen = int(payload[idx+4+4]&0x03) + 1
	} else if idx := bytes.Index(payload, []byte("hvcC")); idx >= 0 && idx+26 <= len(payload) {
		vt.nalLen = int(payload[idx+4+21]&0x03) + 1
	}
	return nil
}
