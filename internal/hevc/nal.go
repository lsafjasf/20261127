package hevc

import (
	"encoding/binary"
	"fmt"
)

const (
	NALTrailN    = 0
	NALVPS       = 32
	NALSPS       = 33
	NALPPS       = 34
	NALAUD       = 35
	NALEOS       = 36
	NALEOB       = 37
	NALFD        = 38
	NALSEIPrefix = 39
	NALSEISuffix = 40
	NALRPU       = 62
	NALUnspec63  = 63
)

type NAL struct {
	Type       int
	LayerID    int
	TemporalID int
	Data       []byte
}

func (n NAL) RBSP() []byte {
	if len(n.Data) <= 2 {
		return nil
	}
	return RemoveEmulationPrevention(n.Data[2:])
}

func SplitLengthPrefixed(data []byte, lengthSize int) ([]NAL, error) {
	if lengthSize < 1 || lengthSize > 4 {
		return nil, fmt.Errorf("invalid NAL length size %d", lengthSize)
	}
	var out []NAL
	for pos := 0; pos < len(data); {
		if len(data)-pos < lengthSize {
			return out, fmt.Errorf("truncated NAL length at byte %d", pos)
		}
		size := 0
		for i := 0; i < lengthSize; i++ {
			size = size<<8 | int(data[pos+i])
		}
		pos += lengthSize
		if size < 2 || size > len(data)-pos {
			return out, fmt.Errorf("invalid NAL size %d at byte %d", size, pos)
		}
		raw := data[pos : pos+size]
		out = append(out, NAL{
			Type:       (int(raw[0]) >> 1) & 0x3f,
			LayerID:    (int(raw[0])&1)<<5 | int(raw[1])>>3,
			TemporalID: int(raw[1]&7) - 1,
			Data:       raw,
		})
		pos += size
	}
	return out, nil
}

func SplitAnnexB(data []byte) ([]NAL, error) {
	var starts []int
	for i := 0; i+3 <= len(data); {
		if data[i] == 0 && data[i+1] == 0 && data[i+2] == 1 {
			starts = append(starts, i)
			i += 3
			continue
		}
		if i+4 <= len(data) && data[i] == 0 && data[i+1] == 0 && data[i+2] == 0 && data[i+3] == 1 {
			starts = append(starts, i)
			i += 4
			continue
		}
		i++
	}
	if len(starts) == 0 {
		return nil, fmt.Errorf("no Annex B start code found")
	}
	out := make([]NAL, 0, len(starts))
	for i, start := range starts {
		prefix := 3
		if start+3 < len(data) && data[start+2] == 0 {
			prefix = 4
		}
		begin := start + prefix
		end := len(data)
		if i+1 < len(starts) {
			end = starts[i+1]
		}
		for end > begin && data[end-1] == 0 {
			end--
		}
		if end-begin < 2 {
			continue
		}
		raw := data[begin:end]
		out = append(out, NAL{
			Type:       (int(raw[0]) >> 1) & 0x3f,
			LayerID:    (int(raw[0])&1)<<5 | int(raw[1])>>3,
			TemporalID: int(raw[1]&7) - 1,
			Data:       raw,
		})
	}
	return out, nil
}

func RemoveEmulationPrevention(data []byte) []byte {
	out := make([]byte, 0, len(data))
	zeros := 0
	for _, b := range data {
		if zeros >= 2 && b == 3 {
			zeros = 0
			continue
		}
		out = append(out, b)
		if b == 0 {
			zeros++
		} else {
			zeros = 0
		}
	}
	return out
}

func AddEmulationPrevention(data []byte) []byte {
	out := make([]byte, 0, len(data)+len(data)/32)
	zeros := 0
	for _, b := range data {
		if zeros >= 2 && b <= 3 {
			out = append(out, 3)
			zeros = 0
		}
		out = append(out, b)
		if b == 0 {
			zeros++
		} else {
			zeros = 0
		}
	}
	return out
}

func Uint16(data []byte) uint16 {
	return binary.BigEndian.Uint16(data)
}
