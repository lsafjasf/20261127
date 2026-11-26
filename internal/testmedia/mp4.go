package testmedia

import (
	"encoding/binary"
)

type MP4Options struct {
	EntryType         string
	Width             int
	Height            int
	Timescale         uint32
	SampleDuration    uint32
	SampleCount       int
	IncludeHVCC       bool
	NoHVCC            bool
	IncludeDV         bool
	NoDV              bool
	DVBox             string
	DVProfile         int
	DVLevel           int
	DVCCID            int
	DVCompression     int
	DVRPU             bool
	DVEL              bool
	DVBL              bool
	IncludeHVCE       bool
	RPUProfile        int
	OmitRPU           bool
	FirstRPUUsePrev   bool
	UsePrevAfterFirst bool
	ColourTransfer    int
	HVCCLevelIDC      int
	SPSLevelIDC       int
	HVCCProfileIDC    int
	HVCELevelIDC      int
	Encrypted         bool
	OriginalFormat    string
	EncryptionScheme  string
}

func BuildMP4(opts MP4Options) []byte {
	opts = mp4Defaults(opts)
	samples := makeSamples(opts)
	ftyp := Box("ftyp", []byte("isom"), U32(0x200), []byte("isom"), []byte("iso6"), []byte("mp41"))
	moov0 := makeMoov(opts, 0)
	chunkOffset := uint64(len(ftyp) + len(moov0) + 8)
	moov := makeMoov(opts, chunkOffset)
	mdat := Box("mdat", Join(samples...))
	return Join(ftyp, moov, mdat)
}

func mp4Defaults(opts MP4Options) MP4Options {
	if opts.EntryType == "" {
		opts.EntryType = "hvc1"
	}
	if opts.Width == 0 {
		opts.Width = 1920
	}
	if opts.Height == 0 {
		opts.Height = 1080
	}
	if opts.Timescale == 0 {
		opts.Timescale = 1000
	}
	if opts.SampleDuration == 0 {
		opts.SampleDuration = 41
	}
	if opts.SampleCount == 0 {
		opts.SampleCount = 3
	}
	if !opts.IncludeHVCC && !opts.NoHVCC {
		opts.IncludeHVCC = true
	}
	if !opts.IncludeDV && !opts.NoDV {
		opts.IncludeDV = true
	}
	if opts.DVProfile == 0 {
		opts.DVProfile = 8
	}
	if opts.DVLevel == 0 {
		opts.DVLevel = 9
	}
	if opts.DVCCID == 0 && opts.DVProfile != 5 {
		opts.DVCCID = 1
	}
	if opts.DVBox == "" {
		if opts.DVProfile >= 8 && opts.DVProfile <= 10 {
			opts.DVBox = "dvvC"
		} else {
			opts.DVBox = "dvcC"
		}
	}
	if !opts.DVRPU && !opts.DVBL && !opts.DVEL {
		opts.DVRPU = true
		opts.DVBL = true
		if opts.DVProfile == 4 || opts.DVProfile == 7 {
			opts.DVEL = true
		}
	}
	if opts.RPUProfile == 0 {
		opts.RPUProfile = opts.DVProfile
	}
	if opts.HVCCLevelIDC == 0 {
		opts.HVCCLevelIDC = 153
	}
	if opts.SPSLevelIDC == 0 {
		opts.SPSLevelIDC = opts.HVCCLevelIDC
	}
	if opts.HVCCProfileIDC == 0 {
		opts.HVCCProfileIDC = 2
	}
	if opts.ColourTransfer == 0 {
		switch opts.DVCCID {
		case 1, 6:
			opts.ColourTransfer = 16
		case 3, 4:
			opts.ColourTransfer = 18
		default:
			opts.ColourTransfer = 1
		}
	}
	if opts.Encrypted {
		if opts.OriginalFormat == "" {
			opts.OriginalFormat = opts.EntryType
		}
		if opts.EncryptionScheme == "" {
			opts.EncryptionScheme = "cenc"
		}
	}
	return opts
}

func makeSamples(opts MP4Options) [][]byte {
	out := make([][]byte, 0, opts.SampleCount)
	for i := 0; i < opts.SampleCount; i++ {
		var sample []byte
		if !opts.OmitRPU && opts.RPUProfile >= 0 {
			usePrev := opts.FirstRPUUsePrev && i == 0
			if opts.UsePrevAfterFirst && i > 0 {
				usePrev = true
			}
			nal := MakeRPUNAL(RPUOptions{
				Profile:     opts.RPUProfile,
				UsePrevious: usePrev,
				PreviousID:  0,
				MappingID:   0,
			})
			sample = append(sample, lengthPrefixed(nal)...)
		}
		vcl := []byte{0x02, 0x01, 0x80}
		sample = append(sample, lengthPrefixed(vcl)...)
		out = append(out, sample)
	}
	return out
}

func lengthPrefixed(nal []byte) []byte {
	out := make([]byte, 4, 4+len(nal))
	binary.BigEndian.PutUint32(out, uint32(len(nal)))
	return append(out, nal...)
}
