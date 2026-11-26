package webm

import (
	"encoding/binary"
	"errors"
	"fmt"
)

var errUnsupportedHeader = errors.New("unsupported codec header")

// ParseCodecHeader parses the small uncompressed header that carries coded size.
func ParseCodecHeader(codec string, payload []byte) (*Bitstream, error) {
	switch codec {
	case "VP8":
		return parseVP8Header(payload)
	case "VP9":
		return parseVP9Header(payload)
	default:
		return nil, fmt.Errorf("%w: %s", errUnsupportedHeader, codec)
	}
}

func parseVP8Header(p []byte) (*Bitstream, error) {
	if len(p) < 3 {
		return nil, errors.New("vp8: short frame tag")
	}
	tag := uint32(p[0]) | uint32(p[1])<<8 | uint32(p[2])<<16
	keyframe := tag&1 == 0
	info := &Bitstream{Codec: "VP8", Keyframe: keyframe}
	if !keyframe {
		return info, nil
	}
	if len(p) < 10 {
		return nil, errors.New("vp8: short keyframe header")
	}
	if p[3] != 0x9d || p[4] != 0x01 || p[5] != 0x2a {
		return nil, errors.New("vp8: invalid keyframe start code")
	}
	rawWidth := binary.LittleEndian.Uint16(p[6:8])
	rawHeight := binary.LittleEndian.Uint16(p[8:10])
	info.Width = int(rawWidth & 0x3fff)
	info.Height = int(rawHeight & 0x3fff)
	if info.Width == 0 || info.Height == 0 {
		return nil, errors.New("vp8: zero coded dimension")
	}
	return info, nil
}

type bitReader struct {
	p   []byte
	pos int
}

func (r *bitReader) read(bits int) (uint32, error) {
	if bits < 0 || bits > 25 || r.pos+bits > len(r.p)*8 {
		return 0, errors.New("vp9: bit read past header")
	}
	var v uint32
	for i := 0; i < bits; i++ {
		bytePos := (r.pos + i) / 8
		bitPos := 7 - ((r.pos + i) % 8)
		v = v<<1 | uint32((r.p[bytePos]>>uint(bitPos))&1)
	}
	r.pos += bits
	return v, nil
}

func parseVP9Header(p []byte) (*Bitstream, error) {
	if len(p) < 2 {
		return nil, errors.New("vp9: short frame header")
	}
	r := &bitReader{p: p}
	marker, _ := r.read(2)
	if marker != 2 {
		return nil, errors.New("vp9: invalid frame marker")
	}
	profileLow, _ := r.read(1)
	profileHigh, _ := r.read(1)
	profile := int(profileLow | profileHigh<<1)
	if profile == 3 {
		if _, err := r.read(1); err != nil {
			return nil, err
		}
	}
	showExisting, err := r.read(1)
	if err != nil {
		return nil, err
	}
	info := &Bitstream{Codec: "VP9", Profile: profile}
	if showExisting == 1 {
		_, _ = r.read(3)
		info.Keyframe = false
		return info, nil
	}
	frameType, err := r.read(1)
	if err != nil {
		return nil, err
	}
	info.Keyframe = frameType == 0
	if _, err := r.read(1); err != nil { // show_frame
		return nil, err
	}
	if _, err := r.read(1); err != nil { // error_resilient_mode
		return nil, err
	}
	if !info.Keyframe {
		return info, nil
	}
	sync, err := r.read(24)
	if err != nil {
		return nil, err
	}
	if sync != 0x498342 {
		return nil, fmt.Errorf("vp9: invalid sync code %06x", sync)
	}
	if err := skipVP9ColorConfig(r, profile); err != nil {
		return nil, err
	}
	wMinus, err := r.read(16)
	if err != nil {
		return nil, err
	}
	hMinus, err := r.read(16)
	if err != nil {
		return nil, err
	}
	info.Width = int(wMinus) + 1
	info.Height = int(hMinus) + 1
	different, err := r.read(1)
	if err != nil {
		return nil, err
	}
	if different == 1 {
		rw, err := r.read(16)
		if err != nil {
			return nil, err
		}
		rh, err := r.read(16)
		if err != nil {
			return nil, err
		}
		info.RenderWidth = int(rw) + 1
		info.RenderHeight = int(rh) + 1
	}
	return info, nil
}

func skipVP9ColorConfig(r *bitReader, profile int) error {
	if profile >= 2 {
		if _, err := r.read(1); err != nil { // ten_or_twelve_bit
			return err
		}
	}
	colorSpace, err := r.read(3)
	if err != nil {
		return err
	}
	if colorSpace != 7 {
		if _, err := r.read(1); err != nil { // color_range
			return err
		}
		if profile == 1 || profile == 3 {
			if _, err := r.read(1); err != nil { // subsampling_x
				return err
			}
			if _, err := r.read(1); err != nil { // subsampling_y
				return err
			}
			if _, err := r.read(1); err != nil { // reserved_zero
				return err
			}
		}
	} else if profile == 1 || profile == 3 {
		if _, err := r.read(1); err != nil { // reserved_zero
			return err
		}
	}
	return nil
}
