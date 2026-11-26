package testmedia

import (
	"encoding/binary"

	"github.com/lsafjasf/20261127/internal/dovi"
	"github.com/lsafjasf/20261127/internal/hevc"
)

type bitWriter struct {
	data []byte
	pos  int
}

func (w *bitWriter) put(v uint64, n int) {
	for i := n - 1; i >= 0; i-- {
		if w.pos%8 == 0 {
			w.data = append(w.data, 0)
		}
		if (v>>i)&1 != 0 {
			w.data[len(w.data)-1] |= 1 << (7 - w.pos%8)
		}
		w.pos++
	}
}

func (w *bitWriter) ue(v uint64) {
	code := v + 1
	bits := 0
	for (uint64(1) << bits) <= code {
		bits++
	}
	w.put(0, bits-1)
	w.put(code, bits)
}

type RPUOptions struct {
	Profile     int
	UsePrevious bool
	PreviousID  int
	MappingID   int
	FullRange   bool
}

func MakeRPU(opts RPUOptions) []byte {
	if opts.Profile == 0 {
		opts.Profile = 8
	}
	vdrProfile := 0
	fullRange := opts.FullRange
	elResampling := false
	disableResidual := true
	vdrDepthMinus8 := 4
	switch opts.Profile {
	case 5:
		vdrProfile = 0
		fullRange = true
	case 7:
		vdrProfile = 1
		elResampling = true
		disableResidual = false
	case 4:
		vdrProfile = 1
		elResampling = true
		disableResidual = false
		vdrDepthMinus8 = 2
	case 8:
		vdrProfile = 1
	default:
		vdrProfile = 2
	}

	w := &bitWriter{}
	w.put(2, 6)   // rpu_type
	w.put(18, 11) // rpu_format
	w.put(uint64(vdrProfile), 4)
	w.put(0, 4) // vdr_rpu_level
	w.put(1, 1) // vdr_seq_info_present_flag
	w.put(0, 1) // chroma_resampling_explicit_filter_flag
	w.put(0, 2) // coefficient_data_type
	w.ue(23)    // coefficient_log2_denom
	w.put(1, 2) // vdr_rpu_normalized_idc
	if fullRange {
		w.put(1, 1)
	} else {
		w.put(0, 1)
	}
	w.ue(2) // bl_bit_depth_minus8
	w.ue(2) // el_bit_depth_minus8
	w.ue(uint64(vdrDepthMinus8))
	w.put(0, 1) // spatial_resampling_filter_flag
	w.put(0, 3) // reserved_zero_3bits
	if elResampling {
		w.put(1, 1)
	} else {
		w.put(0, 1)
	}
	if disableResidual {
		w.put(1, 1)
	} else {
		w.put(0, 1)
	}
	w.put(0, 1) // vdr_dm_metadata_present_flag
	if opts.UsePrevious {
		w.put(1, 1)
		w.ue(uint64(opts.PreviousID))
	} else {
		w.put(0, 1)
		w.ue(uint64(opts.MappingID))
	}
	w.put(0, 8-w.pos%8)

	payload := append([]byte{0x19}, w.data...)
	crc := dovi.CRC32MPEG2(w.data)
	out := make([]byte, 0, len(payload)+5)
	out = append(out, payload...)
	var crcBytes [4]byte
	binary.BigEndian.PutUint32(crcBytes[:], crc)
	out = append(out, crcBytes[:]...)
	out = append(out, 0x80)
	return out
}

func MakeRPUNAL(opts RPUOptions) []byte {
	payload := hevc.AddEmulationPrevention(MakeRPU(opts))
	return append([]byte{0x7c, 0x01}, payload...)
}
