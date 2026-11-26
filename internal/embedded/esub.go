// Package embedded implements the embedded subtitle container.
//
// The .esub format is a small, self-describing binary container carrying one
// or more language tracks inside a single deliverable:
//
//	magic     = "ESUB1\n"          (6 bytes)
//	mediaID   = u16 length + UTF-8 bytes
//	trackNum  = u16 big-endian
//	track     = lang (u8 len + bytes)
//	            label (u16 len + bytes)
//	            flags (u8; bit 0 = default)
//	            cueNum (u32)
//	cue       = start (u64 nanoseconds)
//	            end   (u64 nanoseconds)
//	            text  (u32 len + UTF-8 bytes)
package embedded

import (
	"encoding/binary"
	"errors"
	"fmt"

	"subdeliver/internal/manifest"
	"subdeliver/internal/subtitle"
)

// Magic is the container signature.
const Magic = "ESUB1\n"

// Marshal renders all canonical cue sets into one embedded container.
func Marshal(mediaID string, sets []manifest.CueSet) ([]byte, error) {
	initialCap := 6 + 2 + len(mediaID) + 2
	out := make([]byte, 0, initialCap)
	out = append(out, Magic...)
	out = appendU16(out, uint16(len(mediaID)))
	out = append(out, mediaID...)
	out = appendU16(out, uint16(len(sets)))

	for _, set := range sets {
		if len(set.Language) > 255 {
			return nil, fmt.Errorf("esub: language tag too long: %q", set.Language)
		}
		out = append(out, byte(len(set.Language)))
		out = append(out, set.Language...)
		out = appendU16(out, uint16(len(set.Label)))
		out = append(out, set.Label...)
		flags := byte(0)
		if set.Default {
			flags |= 1
		}
		out = append(out, flags)
		out = appendU32(out, uint32(len(set.Cues)))
		for _, cue := range set.Cues {
			out = appendU64(out, uint64(cue.Start))
			out = appendU64(out, uint64(cue.End))
			out = appendU32(out, uint32(len(cue.Text)))
			out = append(out, cue.Text...)
		}
	}
	return out, nil
}

// Unmarshal is an independent binary reader used by verification.
func Unmarshal(data []byte) (string, []manifest.CueSet, error) {
	r := &reader{data: data}
	magic := r.bytes(len(Magic))
	if string(magic) != Magic {
		return "", nil, errors.New("esub: bad magic")
	}
	mediaID := string(r.u16Prefixed())
	trackNum := int(r.u16())
	sets := make([]manifest.CueSet, 0, trackNum)
	for i := 0; i < trackNum; i++ {
		lang := string(r.bytePrefixed())
		label := string(r.u16Prefixed())
		flags := r.u8()
		cueNum := int(r.u32())
		set := manifest.CueSet{
			Language: lang,
			Label:    label,
			Default:  flags&1 == 1,
			Cues:     make([]subtitle.Cue, 0, cueNum),
		}
		for j := 0; j < cueNum; j++ {
			start := int64(r.u64())
			end := int64(r.u64())
			text := string(r.u32Prefixed())
			set.Cues = append(set.Cues, subtitle.Cue{
				Index:    j + 1,
				Start:    start,
				End:      end,
				Text:     subtitle.NormalizeText(text),
				Language: lang,
			})
		}
		sets = append(sets, set)
	}
	if r.err != nil {
		return "", nil, r.err
	}
	if len(r.data) != 0 {
		return "", nil, fmt.Errorf("esub: %d trailing bytes", len(r.data))
	}
	return mediaID, sets, nil
}

type reader struct {
	data []byte
	err  error
}

func (r *reader) take(n int) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 || n > len(r.data) {
		r.err = errors.New("esub: unexpected end of container")
		return nil
	}
	v := r.data[:n]
	r.data = r.data[n:]
	return v
}

func (r *reader) u8() byte {
	b := r.take(1)
	if b == nil {
		return 0
	}
	return b[0]
}
func (r *reader) bytePrefixed() []byte {
	return r.take(int(r.u8()))
}
func (r *reader) u16() uint16 {
	b := r.take(2)
	if b == nil {
		return 0
	}
	return binary.BigEndian.Uint16(b)
}
func (r *reader) u16Prefixed() []byte {
	return r.take(int(r.u16()))
}
func (r *reader) u32() uint32 {
	b := r.take(4)
	if b == nil {
		return 0
	}
	return binary.BigEndian.Uint32(b)
}
func (r *reader) u32Prefixed() []byte {
	return r.take(int(r.u32()))
}
func (r *reader) u64() uint64 {
	b := r.take(8)
	if b == nil {
		return 0
	}
	return binary.BigEndian.Uint64(b)
}
func (r *reader) bytes(n int) []byte {
	return r.take(n)
}

func appendU16(b []byte, v uint16) []byte {
	return binary.BigEndian.AppendUint16(b, v)
}
func appendU32(b []byte, v uint32) []byte {
	return binary.BigEndian.AppendUint32(b, v)
}
func appendU64(b []byte, v uint64) []byte {
	return binary.BigEndian.AppendUint64(b, v)
}
