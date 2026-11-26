package dovi

import (
	"errors"
	"fmt"

	"github.com/lsafjasf/20261127/internal/bits"
)

var (
	ErrRPUTooShort  = errors.New("RPU is too short")
	ErrRPUBadPrefix = errors.New("RPU prefix is not 0x19")
	ErrRPUBadEnd    = errors.New("RPU does not end with 0x80")
	ErrCRCMismatch  = errors.New("RPU CRC32 mismatch")
)

type RPU struct {
	Profile           int
	RPUType           int
	RPUFormat         int
	VDRRPUProfile     int
	VDRRPULevel       int
	SeqInfoPresent    bool
	BLFullRange       bool
	BLBitDepth        int
	ELBitDepth        int
	VDRBitDepth       int
	SpatialResampling bool
	ELResampling      bool
	DisableResidual   bool
	MetadataPresent   bool
	UsePreviousRPU    bool
	PreviousRPUID     int
	MappingID         int
	CRCValid          bool
	HeaderValidation  []string
}

// ParseRPU consumes the RBSP payload of an HEVC NAL unit type 62, after the
// two-byte NAL header and after start-code emulation prevention is removed.
func ParseRPU(data []byte) (*RPU, error) {
	end := len(data)
	for end > 0 && data[end-1] == 0 {
		end--
	}
	if end < 7 {
		return nil, ErrRPUTooShort
	}
	data = data[:end]
	if data[0] != 0x19 {
		return nil, ErrRPUBadPrefix
	}
	if data[end-1] != 0x80 {
		return nil, ErrRPUBadEnd
	}
	crcStart := end - 5
	received := uint32(data[crcStart])<<24 | uint32(data[crcStart+1])<<16 |
		uint32(data[crcStart+2])<<8 | uint32(data[crcStart+3])
	computed := CRC32MPEG2(data[1:crcStart])

	rpu, err := parseRPUHeader(data[1:crcStart])
	if err != nil {
		return nil, err
	}
	rpu.CRCValid = received == computed
	if !rpu.CRCValid {
		return rpu, ErrCRCMismatch
	}
	return rpu, nil
}

func parseRPUHeader(data []byte) (*RPU, error) {
	r := bits.NewReader(data)
	rpu := &RPU{PreviousRPUID: -1, MappingID: -1}

	var err error
	if rpu.RPUType, err = readInt(r, 6); err != nil {
		return nil, err
	}
	if rpu.RPUType != 2 {
		rpu.HeaderValidation = append(rpu.HeaderValidation,
			fmt.Sprintf("rpu_type is %d, expected 2", rpu.RPUType))
	}
	if rpu.RPUFormat, err = readInt(r, 11); err != nil {
		return nil, err
	}
	if rpu.VDRRPUProfile, err = readInt(r, 4); err != nil {
		return nil, err
	}
	if rpu.VDRRPULevel, err = readInt(r, 4); err != nil {
		return nil, err
	}
	if rpu.SeqInfoPresent, err = r.ReadBool(); err != nil {
		return nil, err
	}

	if rpu.SeqInfoPresent {
		if _, err = r.ReadBool(); err != nil { // chroma_resampling_explicit_filter_flag
			return nil, err
		}
		coefType, err := r.ReadBits(2)
		if err != nil {
			return nil, err
		}
		if coefType == 0 {
			if _, err = r.ReadUE(); err != nil {
				return nil, err
			}
		} else if coefType > 1 {
			rpu.HeaderValidation = append(rpu.HeaderValidation,
				fmt.Sprintf("invalid coefficient_data_type %d", coefType))
		}
		if _, err = r.ReadBits(2); err != nil { // vdr_rpu_normalized_idc
			return nil, err
		}
		if rpu.BLFullRange, err = r.ReadBool(); err != nil {
			return nil, err
		}
		if rpu.RPUFormat&0x700 == 0 {
			blDepth, err := r.ReadUE()
			if err != nil {
				return nil, err
			}
			elDepth, err := r.ReadUE()
			if err != nil {
				return nil, err
			}
			vdrDepth, err := r.ReadUE()
			if err != nil {
				return nil, err
			}
			rpu.BLBitDepth = int(blDepth) + 8
			rpu.ELBitDepth = int(elDepth&0xff) + 8
			rpu.VDRBitDepth = int(vdrDepth) + 8
			if rpu.SpatialResampling, err = r.ReadBool(); err != nil {
				return nil, err
			}
			if err = r.Skip(3); err != nil { // reserved_zero_3bits
				return nil, err
			}
			if rpu.ELResampling, err = r.ReadBool(); err != nil {
				return nil, err
			}
			if rpu.DisableResidual, err = r.ReadBool(); err != nil {
				return nil, err
			}
		}
	}
	if rpu.MetadataPresent, err = r.ReadBool(); err != nil {
		return nil, err
	}
	if rpu.UsePreviousRPU, err = r.ReadBool(); err != nil {
		return nil, err
	}
	if rpu.UsePreviousRPU {
		prev, err := r.ReadUE()
		if err != nil {
			return nil, err
		}
		rpu.PreviousRPUID = int(prev)
		if prev > 15 {
			rpu.HeaderValidation = append(rpu.HeaderValidation,
				fmt.Sprintf("previous_rpu_id %d is outside 0..15", prev))
		}
	} else {
		mappingID, err := r.ReadUE()
		if err != nil {
			return nil, err
		}
		rpu.MappingID = int(mappingID)
		if mappingID > 15 {
			rpu.HeaderValidation = append(rpu.HeaderValidation,
				fmt.Sprintf("vdr_rpu_id %d is outside 0..15", mappingID))
		}
	}

	rpu.Profile = deriveProfile(rpu)
	if rpu.VDRRPULevel != 0 {
		rpu.HeaderValidation = append(rpu.HeaderValidation,
			fmt.Sprintf("vdr_rpu_level is %d, expected 0", rpu.VDRRPULevel))
	}
	if rpu.BLBitDepth != 0 && rpu.BLBitDepth != 10 {
		rpu.HeaderValidation = append(rpu.HeaderValidation,
			fmt.Sprintf("BL bit depth is %d, expected 10", rpu.BLBitDepth))
	}
	if rpu.ELBitDepth != 0 && rpu.ELBitDepth != 10 {
		rpu.HeaderValidation = append(rpu.HeaderValidation,
			fmt.Sprintf("EL bit depth is %d, expected 10", rpu.ELBitDepth))
	}
	return rpu, nil
}

func deriveProfile(r *RPU) int {
	switch r.VDRRPUProfile {
	case 0:
		if r.BLFullRange {
			return 5
		}
	case 1:
		if r.ELResampling && !r.DisableResidual {
			if r.VDRBitDepth == 12 {
				return 7
			}
			return 4
		}
		return 8
	}
	return 0
}

func readInt(r *bits.Reader, n int) (int, error) {
	v, err := r.ReadBits(n)
	return int(v), err
}

// CRC32MPEG2 is the non-reflected CRC-32/MPEG-2 used at the end of an RPU.
func CRC32MPEG2(data []byte) uint32 {
	crc := uint32(0xffffffff)
	for _, b := range data {
		crc ^= uint32(b) << 24
		for i := 0; i < 8; i++ {
			if crc&0x80000000 != 0 {
				crc = crc<<1 ^ 0x04c11db7
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}
