package testmedia

func makeMoov(opts MP4Options, chunkOffset uint64) []byte {
	duration := uint32(opts.SampleCount) * opts.SampleDuration
	mvhd := FullBox("mvhd", 0, 0, Join(
		U32(0), U32(0), U32(opts.Timescale), U32(duration),
		U32(0x00010000), U16(0x0100), U16(0), U32(0), U32(0),
		matrix(), make([]byte, 24), U32(2),
	))
	trak := makeTrak(opts, chunkOffset, duration)
	return Box("moov", mvhd, trak)
}

func makeTrak(opts MP4Options, chunkOffset uint64, duration uint32) []byte {
	tkhd := FullBox("tkhd", 0, 3, Join(
		U32(0), U32(0), U32(1), U32(0), U32(duration),
		make([]byte, 8), U16(0), U16(0), U16(0), U16(0),
		matrix(), U32(uint32(opts.Width)<<16), U32(uint32(opts.Height)<<16),
	))
	edit := Box("edts", FullBox("elst", 0, 0, Join(
		U32(1), U32(duration), U32(0), U16(1), U16(0),
	)))
	mdhd := FullBox("mdhd", 0, 0, Join(
		U32(0), U32(0), U32(opts.Timescale), U32(duration),
		U16(0x55c4), U16(0),
	))
	hdlr := FullBox("hdlr", 0, 0, Join(
		U32(0), []byte("vide"), make([]byte, 12), []byte("VideoHandler\x00"),
	))
	vmhd := FullBox("vmhd", 0, 1, Join(U16(0), U16(0), U16(0), U16(0)))
	dref := FullBox("dref", 0, 0, Join(
		U32(1), FullBox("url ", 0, 1, nil),
	))
	dinf := Box("dinf", dref)
	stbl := makeSTBL(opts, chunkOffset)
	minf := Box("minf", vmhd, dinf, stbl)
	mdia := Box("mdia", mdhd, hdlr, minf)
	return Box("trak", tkhd, edit, mdia)
}

func makeSTBL(opts MP4Options, chunkOffset uint64) []byte {
	entry := makeVisualEntry(opts)
	stsd := FullBox("stsd", 0, 0, Join(U32(1), entry))
	stts := FullBox("stts", 0, 0, Join(
		U32(1), U32(uint32(opts.SampleCount)), U32(opts.SampleDuration),
	))
	stsc := FullBox("stsc", 0, 0, Join(
		U32(1), U32(1), U32(uint32(opts.SampleCount)), U32(1),
	))
	sizes := Join(U32(0), U32(uint32(opts.SampleCount)))
	for i := 0; i < opts.SampleCount; i++ {
		sample := Join(maybeRPU(opts, i), lengthPrefixed([]byte{0x02, 0x01, 0x80}))
		sizes = append(sizes, U32(uint32(len(sample)))...)
	}
	stsz := FullBox("stsz", 0, 0, sizes)
	stco := FullBox("stco", 0, 0, Join(U32(1), U32(uint32(chunkOffset))))
	return Box("stbl", stsd, stts, stsc, stsz, stco)
}

func maybeRPU(opts MP4Options, sampleIndex int) []byte {
	if opts.OmitRPU || opts.RPUProfile < 0 {
		return nil
	}
	usePrev := opts.FirstRPUUsePrev && sampleIndex == 0
	if opts.UsePrevAfterFirst && sampleIndex > 0 {
		usePrev = true
	}
	return lengthPrefixed(MakeRPUNAL(RPUOptions{
		Profile:     opts.RPUProfile,
		UsePrevious: usePrev,
		PreviousID:  0,
		MappingID:   0,
	}))
}

func matrix() []byte {
	return Join(
		U32(0x00010000), U32(0), U32(0),
		U32(0), U32(0x00010000), U32(0),
		U32(0), U32(0), U32(0x40000000),
	)
}
