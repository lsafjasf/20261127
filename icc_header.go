package iccproof

import (
	"encoding/binary"
	"fmt"
	"strings"
)

// ParseICCBytes validates the ICC magic and extracts header/tag facts without
// pretending to implement a full CMM.
func ParseICCBytes(data []byte) (*ProfileInfo, error) {
	if len(data) < 132 {
		return nil, fmt.Errorf("%w: too short: %d bytes", ErrInvalidProfile, len(data))
	}
	if string(data[36:40]) != "acsp" {
		return nil, fmt.Errorf("%w: signature at offset 36 is %q, want acsp", ErrInvalidProfile, strings.TrimSpace(string(data[36:40])))
	}
	declared := binary.BigEndian.Uint32(data[0:4])
	if declared != 0 && int(declared) > len(data) {
		return nil, fmt.Errorf("%w: header size %d exceeds file %d", ErrInvalidProfile, declared, len(data))
	}
	info := &ProfileInfo{
		Size:        len(data),
		Version:     fmt.Sprintf("%d.%d", data[8], data[9]>>4),
		DeviceClass: iccSignature(data[12:16]),
		ColorSpace:  iccSignature(data[16:20]),
		PCS:         iccSignature(data[20:24]),
	}
	count := binary.BigEndian.Uint32(data[128:132])
	if int(count) > (len(data)-132)/12 {
		return nil, fmt.Errorf("%w: tag count %d exceeds profile capacity", ErrInvalidProfile, count)
	}
	info.TagCount = int(count)
	for i := uint32(0); i < count; i++ {
		off := 132 + int(i)*12
		info.Tags = append(info.Tags, iccSignature(data[off:off+4]))
	}
	return info, nil
}

func iccSignature(b []byte) string {
	return strings.TrimRight(strings.TrimSpace(string(b)), "\x00")
}
