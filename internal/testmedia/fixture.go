package testmedia

// BuildAudioMP4 returns a valid 30-minute audio-only ISO BMFF fixture. It has
// 84,375 AAC frames, a movie timescale of 1,000, and an edit-list media_time
// of 1,024 ticks (the AAC priming frame).
func BuildAudioMP4() []byte {
	const (
		movieScale   = 1000
		movieDur     = 1_800_000
		audioScale   = 48_000
		samples      = 84_375
		editMedia    = 1024
		frameTicks   = 1024
		sampleBytes  = 16
		chunkSamples = 1000
	)
	chunks := (samples + chunkSamples - 1) / chunkSamples
	mvhd := fullbox("mvhd", 0, 0, concat(
		u32(0), u32(0), u32(movieScale), u32(movieDur),
		u32(0x00010000), u16(0x0100), []byte{0, 0}, u32(0), u32(0),
		matrix(), zeros(24), u32(2),
	))
	tkhd := fullbox("tkhd", 0, 3, concat(
		u32(0), u32(0), u32(1), u32(0), u32(movieDur),
		zeros(8), u16(0), u16(0), u16(0x0100), u16(0),
		matrix(), u32(0), u32(0),
	))
	mdhd := fullbox("mdhd", 0, 0, concat(
		u32(0), u32(0), u32(audioScale), u32(samples*frameTicks), u16(0x55c4), u16(0),
	))
	hdlr := fullbox("hdlr", 0, 0, concat(
		u32(0), []byte("soun"), zeros(12), []byte("SoundHandler"),
	))
	elst := fullbox("elst", 0, 0, concat(
		u32(1), u32(movieDur), u32(editMedia), u16(1), u16(0),
	))
	entry := box("mp4a", concat(
		zeros(6), u16(1), u16(0), u16(0), u32(0),
		u16(2), u16(16), u16(0), u16(0), u32(audioScale<<16),
	))
	stsd := fullbox("stsd", 0, 0, concat(u32(1), entry))
	stts := fullbox("stts", 0, 0, concat(u32(1), u32(samples), u32(frameTicks)))
	stsc := fullbox("stsc", 0, 0, concat(u32(1), u32(1), u32(chunkSamples), u32(1)))
	stsz := fullbox("stsz", 0, 0, concat(u32(sampleBytes), u32(samples)))
	stcoPayload := u32(uint32(chunks))
	for i := 0; i < chunks; i++ {
		stcoPayload = append(stcoPayload, u32(uint32(8_000+i*chunkSamples*sampleBytes))...)
	}
	stbl := box("stbl", concat(stsd, stts, stsc, stsz, fullbox("stco", 0, 0, stcoPayload)))
	trak := box("trak", concat(tkhd, box("edts", elst), box("mdia", concat(mdhd, hdlr, box("minf", stbl)))))
	moov := box("moov", concat(mvhd, trak))
	return concat(
		box("ftyp", concat([]byte("isom"), u32(0), []byte("isommp41"))),
		moov,
		box("mdat", make([]byte, samples*sampleBytes+16_000)),
	)
}
