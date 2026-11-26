package laddercheck

import "encoding/binary"

// classifyNAL walks a length-prefixed sample and decides whether it is a true
// closed IDR picture. A non-nil false means "known sync/RAP sample but not an
// IDR" (open-GOP / CRA / BLA / non-IDR I); nil means undecidable.
func classifyNAL(family string, sample []byte, nalLenSize int) (*bool, []int) {
	if nalLenSize < 1 || nalLenSize > 4 {
		nalLenSize = 4
	}
	types := make([]int, 0, 6)
	seen := map[int]bool{}
	sawVCL := false
	sawIDR := false
	sawNonIDRRAP := false

	off := 0
	for off+nalLenSize <= len(sample) {
		var n int
		switch nalLenSize {
		case 1:
			n = int(sample[off])
		case 2:
			n = int(binary.BigEndian.Uint16(sample[off : off+2]))
		default:
			n = int(binary.BigEndian.Uint32(sample[off : off+4]))
		}
		off += nalLenSize
		if n <= 0 || off+n > len(sample) {
			break
		}
		hdr := sample[off]
		if family == "avc" {
			t := int(hdr & 0x1f)
			if !seen[t] {
				types = append(types, t)
				seen[t] = true
			}
			if t == 5 {
				sawIDR = true
			}
			if t >= 1 && t <= 5 {
				sawVCL = true
				if t != 5 {
					// A coded slice in a sample stss marks as sync but lacking
					// type 5 is a non-IDR / open-GOP random access point.
					sawNonIDRRAP = true
				}
			}
		} else if family == "hevc" {
			if off+1 >= len(sample) {
				break
			}
			t := int((sample[off] >> 1) & 0x3f)
			if !seen[t] {
				types = append(types, t)
				seen[t] = true
			}
			if t <= 31 {
				sawVCL = true
			}
			if t == 19 || t == 20 { // IDR_W_RADL / IDR_N_LP
				sawIDR = true
			}
			if (t >= 16 && t <= 18) || t == 21 { // BLA / CRA
				sawNonIDRRAP = true
			}
		}
		off += n
	}

	if sawIDR {
		v := true
		return &v, types
	}
	// Only call it non-IDR when we actually found picture NAL units.
	if sawVCL && (sawNonIDRRAP || len(types) > 0) {
		v := false
		return &v, types
	}
	return nil, types
}
