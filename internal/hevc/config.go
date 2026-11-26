package hevc

import (
	"fmt"

	"github.com/lsafjasf/20261127/internal/bits"
)

type PTL struct {
	ProfileSpace              int
	TierFlag                  bool
	ProfileIDC                int
	ProfileCompatibilityFlags uint32
	ConstraintFlags           [6]byte
	LevelIDC                  int
}

type DecoderConfig struct {
	ProfileSpace              int
	TierFlag                  bool
	ProfileIDC                int
	LevelIDC                  int
	ProfileCompatibilityFlags uint32
	ChromaFormat              int
	BitDepthLuma              int
	BitDepthChroma            int
	LengthSize                int
	Arrays                    map[int][]NAL
	VPS                       int
	SPS                       int
	PPS                       int
	SPSPTL                    *PTL
}

func ParseDecoderConfig(payload []byte) (*DecoderConfig, error) {
	if len(payload) < 23 {
		return nil, fmt.Errorf("hvcC is %d bytes; need at least 23", len(payload))
	}
	if payload[0] != 1 {
		return nil, fmt.Errorf("unsupported hvcC configurationVersion %d", payload[0])
	}
	cfg := &DecoderConfig{
		Arrays: make(map[int][]NAL),
	}
	profile := payload[1]
	cfg.ProfileSpace = int(profile >> 6)
	cfg.TierFlag = profile&(1<<5) != 0
	cfg.ProfileIDC = int(profile & 0x1f)
	cfg.ProfileCompatibilityFlags = uint32(payload[2])<<24 |
		uint32(payload[3])<<16 | uint32(payload[4])<<8 | uint32(payload[5])
	cfg.LevelIDC = int(payload[12])
	cfg.ChromaFormat = int(payload[16] & 3)
	cfg.BitDepthLuma = int(payload[17]&7) + 8
	cfg.BitDepthChroma = int(payload[18]&7) + 8
	cfg.LengthSize = int(payload[21]&3) + 1

	pos := 23
	if pos >= len(payload) {
		return cfg, nil
	}
	numArrays := int(payload[22])
	for i := 0; i < numArrays; i++ {
		if pos+3 > len(payload) {
			return nil, fmt.Errorf("truncated hvcC NAL array %d", i)
		}
		nalType := int(payload[pos] & 0x3f)
		count := int(payload[pos+1])<<8 | int(payload[pos+2])
		pos += 3
		for j := 0; j < count; j++ {
			if pos+2 > len(payload) {
				return nil, fmt.Errorf("truncated hvcC NAL length in array %d", i)
			}
			size := int(payload[pos])<<8 | int(payload[pos+1])
			pos += 2
			if size < 2 || pos+size > len(payload) {
				return nil, fmt.Errorf("invalid hvcC NAL size %d", size)
			}
			raw := payload[pos : pos+size]
			nal := NAL{
				Type:       (int(raw[0]) >> 1) & 0x3f,
				LayerID:    (int(raw[0])&1)<<5 | int(raw[1])>>3,
				TemporalID: int(raw[1]&7) - 1,
				Data:       raw,
			}
			cfg.Arrays[nalType] = append(cfg.Arrays[nalType], nal)
			pos += size
		}
		switch nalType {
		case NALVPS:
			cfg.VPS += count
		case NALSPS:
			cfg.SPS += count
		case NALPPS:
			cfg.PPS += count
		}
	}
	if len(cfg.Arrays[NALSPS]) > 0 {
		if ptl, err := ParseSPSPTL(cfg.Arrays[NALSPS][0]); err == nil {
			cfg.SPSPTL = &ptl
		}
	}
	return cfg, nil
}

func ParseSPSPTL(nal NAL) (PTL, error) {
	rbsp := nal.RBSP()
	if len(rbsp) < 2 {
		return PTL{}, fmt.Errorf("SPS is too short")
	}
	r := bits.NewReader(rbsp)
	if err := r.Skip(8); err != nil {
		return PTL{}, err
	}
	return readPTL(r, 0)
}

func ParseVPSPTL(nal NAL) (PTL, error) {
	rbsp := nal.RBSP()
	if len(rbsp) < 2 {
		return PTL{}, fmt.Errorf("VPS is too short")
	}
	r := bits.NewReader(rbsp)
	if err := r.Skip(16); err != nil {
		return PTL{}, err
	}
	return readPTL(r, 0)
}

func readPTL(r *bits.Reader, maxSubLayersMinus1 int) (PTL, error) {
	var ptl PTL
	space, err := r.ReadBits(2)
	if err != nil {
		return ptl, err
	}
	tier, err := r.ReadBits(1)
	if err != nil {
		return ptl, err
	}
	profile, err := r.ReadBits(5)
	if err != nil {
		return ptl, err
	}
	compat, err := r.ReadBits(32)
	if err != nil {
		return ptl, err
	}
	ptl.ProfileSpace = int(space)
	ptl.TierFlag = tier != 0
	ptl.ProfileIDC = int(profile)
	ptl.ProfileCompatibilityFlags = uint32(compat)
	for i := range ptl.ConstraintFlags {
		v, err := r.ReadBits(8)
		if err != nil {
			return ptl, err
		}
		ptl.ConstraintFlags[i] = byte(v)
	}
	level, err := r.ReadBits(8)
	if err != nil {
		return ptl, err
	}
	ptl.LevelIDC = int(level)

	if maxSubLayersMinus1 > 0 {
		if err := r.Skip(2 * maxSubLayersMinus1); err != nil {
			return ptl, err
		}
	}
	return ptl, nil
}

func ProfileName(profileIDC int) string {
	switch profileIDC {
	case 1:
		return "Main"
	case 2:
		return "Main10"
	case 3:
		return "Main Still Picture"
	case 4:
		return "Range Extensions"
	default:
		return fmt.Sprintf("HEVC profile_idc=%d", profileIDC)
	}
}
