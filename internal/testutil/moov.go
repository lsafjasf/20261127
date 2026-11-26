package testutil

import "encoding/binary"

// buildMoov 组装 moov 盒;stcos 为每轨 chunk 绝对偏移(全零表示占位)。
func buildMoov(tracks []TrackSpec, opts Options, stcos [][]int64) []byte {
	var duration uint32
	for _, t := range tracks {
		if d := uint32(len(t.Samples)) * t.Delta; d > duration {
			duration = d
		}
	}
	mvhdPayload := cat(
		u32(opts.ModTime), u32(opts.ModTime), // creation/modification
		u32(1000), u32(duration), // timescale/duration
		make([]byte, 80),           // rate..pre_defined
		u32(uint32(len(tracks))+1), // next_track_id
	)
	mvhd := fullbox("mvhd", 0, 0, mvhdPayload)

	var traks []byte
	for i, t := range tracks {
		traks = append(traks, buildTrak(t, opts, stcos[i])...)
	}
	var udta []byte
	if len(opts.Udta) > 0 {
		udta = box("udta", box("cnam", opts.Udta))
	}
	return box("moov", cat(mvhd, traks, udta))
}

func buildTrak(t TrackSpec, opts Options, stco []int64) []byte {
	duration := uint32(len(t.Samples)) * t.Delta
	tkhdPayload := cat(
		u32(opts.ModTime), u32(opts.ModTime),
		u32(t.TrackID), u32(0), u32(duration),
		make([]byte, 60),
	)
	tkhd := fullbox("tkhd", 0, 3, tkhdPayload)

	mdhdPayload := cat(
		u32(opts.ModTime), u32(opts.ModTime),
		u32(t.Timescale), u32(duration),
		u32(0x55c4), u32(0), // language=und
	)
	mdhd := fullbox("mdhd", 0, 0, mdhdPayload)

	hdlrPayload := cat(
		u32(0), []byte(t.Handler),
		make([]byte, 12),
		append([]byte("MediaHandler"), 0),
	)
	hdlr := fullbox("hdlr", 0, 0, hdlrPayload)
	mdia := box("mdia", cat(mdhd, hdlr, buildMinf(t, stco)))
	return box("trak", cat(tkhd, mdia))
}

func buildMinf(t TrackSpec, stco []int64) []byte {
	var mediaHeader []byte
	if t.Handler == "vide" {
		mediaHeader = fullbox("vmhd", 0, 1, make([]byte, 8))
	} else {
		mediaHeader = fullbox("smhd", 0, 0, make([]byte, 4))
	}
	dref := fullbox("dref", 0, 0, cat(u32(1), fullbox("url ", 0, 1, nil)))
	dinf := box("dinf", dref)
	return box("minf", cat(mediaHeader, dinf, buildStbl(t, stco)))
}

func buildStbl(t TrackSpec, stco []int64) []byte {
	// stsd: 单条目,仅类型字段有意义
	entry := cat(u32(16), []byte(t.Codec), make([]byte, 8))
	stsd := fullbox("stsd", 0, 0, cat(u32(1), entry))

	stts := fullbox("stts", 0, 0, cat(
		u32(1), u32(uint32(len(t.Samples))), u32(t.Delta),
	))

	chunks := chunkLayout(t)
	spc := 0
	if len(chunks) > 0 {
		spc = len(chunks[0])
	}
	stsc := fullbox("stsc", 0, 0, cat(
		u32(1), u32(1), u32(uint32(spc)), u32(1),
	))

	szPayload := cat(u32(0), u32(uint32(len(t.Samples))))
	for _, s := range t.Samples {
		szPayload = append(szPayload, u32(uint32(len(s)))...)
	}
	stsz := fullbox("stsz", 0, 0, szPayload)

	coPayload := u32(uint32(len(stco)))
	for _, off := range stco {
		b := make([]byte, 4)
		binary.BigEndian.PutUint32(b, uint32(off))
		coPayload = append(coPayload, b...)
	}
	stcoBox := fullbox("stco", 0, 0, coPayload)

	return box("stbl", cat(stsd, stts, stsc, stsz, stcoBox))
}
