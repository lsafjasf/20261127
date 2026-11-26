package mp4seek

import (
	"encoding/binary"
	"testing"
)

func buildFixtureMoov(t *testing.T, opts fixtureOptions, sizes []uint32, timescale uint32, sampleDelta uint32, chunkOffset uint64) []byte {
	t.Helper()
	avcC := makeBox("avcC", []byte{1, 0x42, 0x00, 0x1e, 0xff})
	visualHeader := make([]byte, 78)
	visualHeader[24] = 0x01
	visualHeader[25] = 0x00
	avc1 := makeBox("avc1", append(visualHeader, avcC...))
	stsdPayload := make([]byte, 8)
	binary.BigEndian.PutUint32(stsdPayload[4:8], 1)
	stsd := makeBox("stsd", append(stsdPayload, avc1...))

	sttsPayload := make([]byte, 16)
	binary.BigEndian.PutUint32(sttsPayload[4:8], 1)
	binary.BigEndian.PutUint32(sttsPayload[8:12], uint32(len(sizes)))
	binary.BigEndian.PutUint32(sttsPayload[12:16], sampleDelta)
	stts := makeBox("stts", sttsPayload)

	stscPayload := make([]byte, 20)
	binary.BigEndian.PutUint32(stscPayload[4:8], 1)
	binary.BigEndian.PutUint32(stscPayload[8:12], 1)
	binary.BigEndian.PutUint32(stscPayload[12:16], uint32(len(sizes)))
	binary.BigEndian.PutUint32(stscPayload[16:20], 1)
	stsc := makeBox("stsc", stscPayload)

	stszPayload := make([]byte, 12+len(sizes)*4)
	binary.BigEndian.PutUint32(stszPayload[8:12], uint32(len(sizes)))
	for i, size := range sizes {
		binary.BigEndian.PutUint32(stszPayload[12+i*4:], size)
	}
	stsz := makeBox("stsz", stszPayload)
	offsetType := "stco"
	if opts.useCO64 {
		offsetType = "co64"
	}
	stco := makeChunkOffsetBox(offsetType, chunkOffset)
	timing := withCompositionOffset(stts, len(sizes), opts.compositionShift)
	children := [][]byte{stsd, timing, stsc, stsz, stco}
	if opts.includeSTSS {
		children = append(children, makeSTSS(opts.syncEntries))
	}
	stbl := makeBox("stbl", concat(children...))
	minf := makeBox("minf", stbl)

	mdhdPayload := make([]byte, 24)
	binary.BigEndian.PutUint32(mdhdPayload[12:16], timescale)
	binary.BigEndian.PutUint32(mdhdPayload[16:20], uint32(len(sizes))*sampleDelta)
	mdhd := makeBox("mdhd", mdhdPayload)
	hdlrPayload := make([]byte, 24)
	copy(hdlrPayload[8:12], "vide")
	hdlr := makeBox("hdlr", append(hdlrPayload, []byte("video\x00")...))
	mdia := makeBox("mdia", concat(mdhd, hdlr, minf))

	tkhdPayload := make([]byte, 84)
	binary.BigEndian.PutUint32(tkhdPayload[12:16], 1)
	tkhd := makeBox("tkhd", tkhdPayload)
	trakChildren := [][]byte{tkhd}
	if opts.includeEdit {
		elstPayload := make([]byte, 20)
		binary.BigEndian.PutUint32(elstPayload[4:8], 1)
		binary.BigEndian.PutUint32(elstPayload[8:12], uint32(len(sizes))*sampleDelta/90)
		binary.BigEndian.PutUint32(elstPayload[12:16], uint32(opts.editMediaTime))
		binary.BigEndian.PutUint32(elstPayload[16:20], 1<<16)
		trakChildren = append(trakChildren, makeBox("edts", makeBox("elst", elstPayload)))
	}
	trakChildren = append(trakChildren, mdia)
	trak := makeBox("trak", concat(trakChildren...))
	mvhdPayload := make([]byte, 100)
	binary.BigEndian.PutUint32(mvhdPayload[12:16], 1000)
	mvhd := makeBox("mvhd", mvhdPayload)
	return makeBox("moov", concat(mvhd, trak))
}

func withCompositionOffset(stts []byte, sampleCount int, offset uint32) []byte {
	if offset == 0 {
		return stts
	}
	payload := make([]byte, 16)
	binary.BigEndian.PutUint32(payload[4:8], 1)
	binary.BigEndian.PutUint32(payload[8:12], uint32(sampleCount))
	binary.BigEndian.PutUint32(payload[12:16], offset)
	return concat(stts, makeBox("ctts", payload))
}

func makeSTSS(entries []uint32) []byte {
	payload := make([]byte, 8+len(entries)*4)
	binary.BigEndian.PutUint32(payload[4:8], uint32(len(entries)))
	for i, entry := range entries {
		binary.BigEndian.PutUint32(payload[8+i*4:], entry)
	}
	return makeBox("stss", payload)
}

func makeChunkOffsetBox(typ string, offset uint64) []byte {
	width := 4
	if typ == "co64" {
		width = 8
	}
	payload := make([]byte, 8+width)
	binary.BigEndian.PutUint32(payload[4:8], 1)
	if width == 8 {
		binary.BigEndian.PutUint64(payload[8:16], offset)
	} else {
		binary.BigEndian.PutUint32(payload[8:12], uint32(offset))
	}
	return makeBox(typ, payload)
}

func concat(parts ...[]byte) []byte {
	var out []byte
	for _, part := range parts {
		out = append(out, part...)
	}
	return out
}
