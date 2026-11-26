package mp4

import (
	"encoding/binary"
	"fmt"
)

type stscEntry struct {
	firstChunk      uint32
	samplesPerChunk uint32
	description     uint32
}

func parseSampleTable(track *Track, payload []byte) error {
	boxes, err := ParseChildren(payload)
	if err != nil {
		return fmt.Errorf("parse stbl: %w", err)
	}
	if stsd := Find(boxes, "stsd"); stsd != nil {
		entries, err := parseStsd(stsd.Payload)
		if err != nil {
			return fmt.Errorf("parse stsd: %w", err)
		}
		track.Entries = entries
	} else {
		track.Warnings = append(track.Warnings, "stsd box is missing")
	}

	sizes, err := parseSizes(boxes)
	if err != nil {
		return err
	}
	chunks, err := parseChunks(boxes)
	if err != nil {
		return err
	}
	sc := parseSTSC(boxes)
	durations := parseSTTS(boxes)
	samples, err := buildSamples(sc, chunks, sizes)
	if err != nil {
		return err
	}
	for i := range samples {
		if i < len(durations) {
			samples[i].Duration = durations[i]
		}
	}
	track.Samples = samples
	return nil
}

func parseSTSC(boxes []Box) []stscEntry {
	b := Find(boxes, "stsc")
	if b == nil || len(b.Payload) < 8 {
		return nil
	}
	count := int(binary.BigEndian.Uint32(b.Payload[4:8]))
	out := make([]stscEntry, 0, count)
	pos := 8
	for i := 0; i < count && pos+12 <= len(b.Payload); i++ {
		out = append(out, stscEntry{
			firstChunk:      binary.BigEndian.Uint32(b.Payload[pos : pos+4]),
			samplesPerChunk: binary.BigEndian.Uint32(b.Payload[pos+4 : pos+8]),
			description:     binary.BigEndian.Uint32(b.Payload[pos+8 : pos+12]),
		})
		pos += 12
	}
	return out
}

func parseSTTS(boxes []Box) []uint32 {
	b := Find(boxes, "stts")
	if b == nil || len(b.Payload) < 8 {
		return nil
	}
	count := int(binary.BigEndian.Uint32(b.Payload[4:8]))
	var out []uint32
	pos := 8
	for i := 0; i < count && pos+8 <= len(b.Payload); i++ {
		n := binary.BigEndian.Uint32(b.Payload[pos : pos+4])
		delta := binary.BigEndian.Uint32(b.Payload[pos+4 : pos+8])
		for j := uint32(0); j < n; j++ {
			out = append(out, delta)
		}
		pos += 8
	}
	return out
}

func parseSizes(boxes []Box) ([]uint32, error) {
	if b := Find(boxes, "stsz"); b != nil {
		if len(b.Payload) < 12 {
			return nil, fmt.Errorf("truncated stsz")
		}
		fixed := binary.BigEndian.Uint32(b.Payload[4:8])
		count := int(binary.BigEndian.Uint32(b.Payload[8:12]))
		if fixed != 0 {
			out := make([]uint32, count)
			for i := range out {
				out[i] = fixed
			}
			return out, nil
		}
		if len(b.Payload) < 12+4*count {
			return nil, fmt.Errorf("truncated stsz entries")
		}
		out := make([]uint32, count)
		for i := range out {
			out[i] = binary.BigEndian.Uint32(b.Payload[12+i*4 : 16+i*4])
		}
		return out, nil
	}
	if b := Find(boxes, "stz2"); b != nil {
		if len(b.Payload) < 12 {
			return nil, fmt.Errorf("truncated stz2")
		}
		fieldSize := int(b.Payload[7])
		count := int(binary.BigEndian.Uint32(b.Payload[8:12]))
		out := make([]uint32, count)
		switch fieldSize {
		case 4:
			if len(b.Payload) < 12+(count+1)/2 {
				return nil, fmt.Errorf("truncated 4-bit stz2 entries")
			}
			for i := range out {
				v := b.Payload[12+i/2]
				if i%2 == 0 {
					out[i] = uint32(v >> 4)
				} else {
					out[i] = uint32(v & 0xf)
				}
			}
		case 8:
			if len(b.Payload) < 12+count {
				return nil, fmt.Errorf("truncated 8-bit stz2 entries")
			}
			for i := range out {
				out[i] = uint32(b.Payload[12+i])
			}
		case 16:
			if len(b.Payload) < 12+2*count {
				return nil, fmt.Errorf("truncated 16-bit stz2 entries")
			}
			for i := range out {
				out[i] = uint32(binary.BigEndian.Uint16(b.Payload[12+i*2 : 14+i*2]))
			}
		default:
			return nil, fmt.Errorf("unsupported stz2 field size %d", fieldSize)
		}
		return out, nil
	}
	return nil, nil
}

func parseChunks(boxes []Box) ([]uint64, error) {
	if b := Find(boxes, "stco"); b != nil {
		if len(b.Payload) < 8 {
			return nil, fmt.Errorf("truncated stco")
		}
		count := int(binary.BigEndian.Uint32(b.Payload[4:8]))
		if len(b.Payload) < 8+4*count {
			return nil, fmt.Errorf("truncated stco entries")
		}
		out := make([]uint64, count)
		for i := range out {
			out[i] = uint64(binary.BigEndian.Uint32(b.Payload[8+i*4 : 12+i*4]))
		}
		return out, nil
	}
	if b := Find(boxes, "co64"); b != nil {
		if len(b.Payload) < 8 {
			return nil, fmt.Errorf("truncated co64")
		}
		count := int(binary.BigEndian.Uint32(b.Payload[4:8]))
		if len(b.Payload) < 8+8*count {
			return nil, fmt.Errorf("truncated co64 entries")
		}
		out := make([]uint64, count)
		for i := range out {
			out[i] = binary.BigEndian.Uint64(b.Payload[8+i*8 : 16+i*8])
		}
		return out, nil
	}
	return nil, nil
}

func buildSamples(sc []stscEntry, chunks []uint64, sizes []uint32) ([]Sample, error) {
	if len(sizes) == 0 {
		return nil, nil
	}
	if len(sc) == 0 || len(chunks) == 0 {
		return nil, fmt.Errorf("sample sizes exist but chunk mapping is missing")
	}
	out := make([]Sample, 0, len(sizes))
	sampleIndex := 0
	scIndex := 0
	for chunkIndex := 1; chunkIndex <= len(chunks) && sampleIndex < len(sizes); chunkIndex++ {
		for scIndex+1 < len(sc) && int(sc[scIndex+1].firstChunk) <= chunkIndex {
			scIndex++
		}
		if int(sc[scIndex].firstChunk) > chunkIndex {
			return nil, fmt.Errorf("stsc does not cover chunk %d", chunkIndex)
		}
		offset := chunks[chunkIndex-1]
		for j := uint32(0); j < sc[scIndex].samplesPerChunk && sampleIndex < len(sizes); j++ {
			out = append(out, Sample{
				Index:  sampleIndex,
				Desc:   int(sc[scIndex].description) - 1,
				Offset: int64(offset),
				Size:   sizes[sampleIndex],
			})
			offset += uint64(sizes[sampleIndex])
			sampleIndex++
		}
	}
	if sampleIndex != len(sizes) {
		return nil, fmt.Errorf("mapped %d of %d samples", sampleIndex, len(sizes))
	}
	return out, nil
}
