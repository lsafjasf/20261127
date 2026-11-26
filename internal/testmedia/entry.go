package testmedia

import (
	"encoding/binary"
)

func makeVisualEntry(opts MP4Options) []byte {
	visual := make([]byte, 78)
	binary.BigEndian.PutUint16(visual[6:8], 1)
	binary.BigEndian.PutUint16(visual[24:26], uint16(opts.Width))
	binary.BigEndian.PutUint16(visual[26:28], uint16(opts.Height))
	binary.BigEndian.PutUint32(visual[28:32], 0x00480000)
	binary.BigEndian.PutUint32(visual[32:36], 0x00480000)
	binary.BigEndian.PutUint16(visual[40:42], 1)
	binary.BigEndian.PutUint16(visual[74:76], 24)
	binary.BigEndian.PutUint16(visual[76:78], 0xffff)

	var children [][]byte
	if opts.IncludeHVCC {
		children = append(children, Box("hvcC", makeHVCC(opts.HVCCProfileIDC, opts.HVCCLevelIDC, opts.SPSLevelIDC)))
	}
	if opts.IncludeDV {
		children = append(children, Box(opts.DVBox, makeDVConfig(opts)))
	}
	if opts.IncludeHVCE {
		children = append(children, Box("hvcE", makeHVCC(2, opts.HVCELevelIDC, opts.HVCELevelIDC)))
	}
	children = append(children, Box("colr", makeColr(opts.ColourTransfer)))
	entryType := opts.EntryType
	if opts.Encrypted {
		entryType = "encv"
		schm := FullBox("schm", 0, 0, Join(
			[]byte(opts.EncryptionScheme), U32(0x00010000),
		))
		children = append(children, Box("sinf",
			Box("frma", []byte(opts.OriginalFormat)), schm,
		))
	}
	return Box(entryType, visual, Join(children...))
}

func makeDVConfig(opts MP4Options) []byte {
	out := make([]byte, 24)
	out[0] = 1
	out[1] = 0
	v := uint16(opts.DVProfile&0x7f)<<9 |
		uint16(opts.DVLevel&0x3f)<<3
	if opts.DVRPU {
		v |= 1 << 2
	}
	if opts.DVEL {
		v |= 1 << 1
	}
	if opts.DVBL {
		v |= 1
	}
	binary.BigEndian.PutUint16(out[2:4], v)
	out[4] = byte(opts.DVCCID&0xf)<<4 | byte(opts.DVCompression&3)<<2
	return out
}

func makeHVCC(profileIDC, level, spsLevel int) []byte {
	if level == 0 {
		level = 153
	}
	out := make([]byte, 23)
	out[0] = 1
	out[1] = byte(profileIDC & 0x1f)
	binary.BigEndian.PutUint32(out[2:6], 0x60000000)
	out[12] = byte(level)
	binary.BigEndian.PutUint16(out[13:15], 0xf000)
	out[15] = 0xfc
	out[16] = 0xfd
	out[17] = 0xfa
	out[18] = 0xfa
	out[21] = 0x03
	out[22] = 3

	vps := makeVPS(profileIDC, spsLevel)
	sps := makeSPS(profileIDC, spsLevel)
	pps := []byte{0x44, 0x01, 0xc0}
	out = append(out, nalArray(32, vps)...)
	out = append(out, nalArray(33, sps)...)
	out = append(out, nalArray(34, pps)...)
	return out
}

func nalArray(nalType byte, nal []byte) []byte {
	out := []byte{0x80 | nalType, 0, 1}
	out = append(out, U16(uint16(len(nal)))...)
	return append(out, nal...)
}

func makeVPS(profileIDC, level int) []byte {
	ptl := makePTL(profileIDC, level)
	return append([]byte{0x40, 0x01, 0, 0}, ptl...)
}

func makeSPS(profileIDC, level int) []byte {
	ptl := makePTL(profileIDC, level)
	return append([]byte{0x42, 0x01, 0}, ptl...)
}

func makePTL(profileIDC, level int) []byte {
	out := make([]byte, 12)
	out[0] = byte(profileIDC & 0x1f)
	binary.BigEndian.PutUint32(out[1:5], 0x60000000)
	out[11] = byte(level)
	return out
}

func makeColr(transfer int) []byte {
	out := make([]byte, 11)
	copy(out[:4], "nclx")
	binary.BigEndian.PutUint16(out[4:6], 9)
	binary.BigEndian.PutUint16(out[6:8], uint16(transfer))
	binary.BigEndian.PutUint16(out[8:10], 9)
	return out
}
