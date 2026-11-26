package mp4

import (
	"fmt"
	"io"
	"sort"
)

type chunkRun struct {
	FirstChunk      uint32
	SamplesPerChunk uint32
}

func parseSamples(r io.ReaderAt, stbl *Box, t *Track) error {
	sizes, err := readSampleSizes(r, child(stbl, "stsz"), child(stbl, "stz2"))
	if err != nil {
		return fmt.Errorf("track %d: %w", t.ID, err)
	}
	chunks, err := readChunkOffsets(r, child(stbl, "stco"), child(stbl, "co64"))
	if err != nil {
		return fmt.Errorf("track %d: %w", t.ID, err)
	}
	runs, err := readSampleToChunk(r, child(stbl, "stsc"))
	if err != nil {
		return fmt.Errorf("track %d: %w", t.ID, err)
	}
	times, durations, err := readDecodeTimes(r, child(stbl, "stts"))
	if err != nil {
		return fmt.Errorf("track %d: %w", t.ID, err)
	}
	ctts, err := readCompositionOffsets(r, child(stbl, "ctts"))
	if err != nil {
		return fmt.Errorf("track %d: %w", t.ID, err)
	}
	if len(sizes) > 50000000 || len(chunks) > 10000000 {
		return fmt.Errorf("track %d: sample tables are unreasonably large", t.ID)
	}
	if len(sizes) > 0 && len(runs) == 0 {
		return fmt.Errorf("stsc is missing")
	}
	if len(times) != len(sizes) {
		return fmt.Errorf("stts describes %d samples, stsz/stz2 describes %d", len(times), len(sizes))
	}
	if len(ctts) != 0 && len(ctts) != len(sizes) {
		return fmt.Errorf("ctts describes %d samples, stsz/stz2 describes %d", len(ctts), len(sizes))
	}
	samples := make([]Sample, 0, len(sizes))
	sampleIndex := 0
	for chunkIndex, chunkOffset := range chunks {
		spc := samplesPerChunk(runs, uint32(chunkIndex+1))
		if spc == 0 {
			return fmt.Errorf("chunk %d has zero samples", chunkIndex+1)
		}
		var inChunk uint64
		for j := uint32(0); j < spc && sampleIndex < len(sizes); j++ {
			s := Sample{
				Index:         uint32(sampleIndex + 1),
				Size:          sizes[sampleIndex],
				Chunk:         uint32(chunkIndex + 1),
				OffsetInChunk: inChunk,
				FileOffset:    chunkOffset + inChunk,
			}
			if sampleIndex < len(times) {
				s.DecodeTime = times[sampleIndex]
				s.Duration = durations[sampleIndex]
			}
			if sampleIndex < len(ctts) {
				s.CompositionTime = ctts[sampleIndex]
			}
			samples = append(samples, s)
			inChunk += uint64(s.Size)
			sampleIndex++
		}
	}
	if sampleIndex != len(sizes) {
		return fmt.Errorf("sample tables describe %d sizes but chunks contain %d samples", len(sizes), sampleIndex)
	}
	t.Samples = samples
	for _, index := range t.SyncSamples {
		if int(index) >= len(samples) {
			return fmt.Errorf("stss references sample %d but only %d samples exist", index+1, len(samples))
		}
	}
	if len(t.SyncSamples) == 0 {
		t.SyncSamples = make([]uint32, len(samples))
		for i := range samples {
			t.SyncSamples[i] = uint32(i)
		}
	}
	return nil
}

func readSampleSizes(r io.ReaderAt, stsz, stz2 *Box) ([]uint32, error) {
	if stsz != nil {
		if err := payloadLimit(stsz, 12); err != nil {
			return nil, err
		}
		fixed, err := readU32(r, stsz.Payload()+4)
		if err != nil {
			return nil, err
		}
		count, err := readU32(r, stsz.Payload()+8)
		if err != nil {
			return nil, err
		}
		if int64(count)*4 > stsz.End()-(stsz.Payload()+12) && fixed == 0 {
			return nil, fmt.Errorf("stsz entry count %d exceeds box payload", count)
		}
		sizes := make([]uint32, count)
		if fixed != 0 {
			for i := range sizes {
				sizes[i] = fixed
			}
			return sizes, nil
		}
		pos := stsz.Payload() + 12
		for i := range sizes {
			sizes[i], err = readU32(r, pos)
			if err != nil {
				return nil, err
			}
			pos += 4
		}
		return sizes, nil
	}
	if stz2 == nil {
		return nil, fmt.Errorf("stsz/stz2 is missing")
	}
	if err := payloadLimit(stz2, 12); err != nil {
		return nil, err
	}
	fieldSize, err := readU16(r, stz2.Payload()+7)
	if err != nil {
		return nil, err
	}
	count, err := readU32(r, stz2.Payload()+8)
	if err != nil {
		return nil, err
	}
	if fieldSize != 4 && fieldSize != 8 && fieldSize != 16 {
		return nil, fmt.Errorf("invalid stz2 field size %d", fieldSize)
	}
	payloadNeed := int64(count) * int64(fieldSize) / 8
	if payloadNeed > stz2.End()-(stz2.Payload()+12) {
		return nil, fmt.Errorf("stz2 entry count %d exceeds box payload", count)
	}
	sizes := make([]uint32, count)
	pos := stz2.Payload() + 12
	var bits uint32
	var bitsLeft uint
	for i := range sizes {
		if bitsLeft < uint(fieldSize) {
			v, rerr := readU16(r, pos)
			if rerr != nil {
				return nil, rerr
			}
			bits = uint32(v) << 16
			pos += 2
			bitsLeft += 16
		}
		shift := bitsLeft - uint(fieldSize)
		mask := uint32(1<<fieldSize) - 1
		sizes[i] = (bits >> shift) & mask
		bitsLeft -= uint(fieldSize)
	}
	return sizes, nil
}

func readChunkOffsets(r io.ReaderAt, stco, co64 *Box) ([]uint64, error) {
	b := stco
	wide := false
	if b == nil {
		b, wide = co64, true
	}
	if b == nil {
		return nil, fmt.Errorf("stco/co64 is missing")
	}
	if err := payloadLimit(b, 8); err != nil {
		return nil, err
	}
	count, err := readU32(r, b.Payload()+4)
	if err != nil {
		return nil, err
	}
	width := int64(4)
	if wide {
		width = 8
	}
	if int64(count)*width > b.End()-(b.Payload()+8) {
		return nil, fmt.Errorf("chunk offset count %d exceeds box payload", count)
	}
	offsets := make([]uint64, count)
	pos := b.Payload() + 8
	for i := range offsets {
		if wide {
			offsets[i], err = readU64(r, pos)
			pos += 8
		} else {
			var v uint32
			v, err = readU32(r, pos)
			offsets[i] = uint64(v)
			pos += 4
		}
		if err != nil {
			return nil, err
		}
	}
	return offsets, nil
}

func readSampleToChunk(r io.ReaderAt, stsc *Box) ([]chunkRun, error) {
	if stsc == nil {
		return nil, nil
	}
	if err := payloadLimit(stsc, 8); err != nil {
		return nil, err
	}
	count, err := readU32(r, stsc.Payload()+4)
	if err != nil {
		return nil, err
	}
	if int64(count)*12 > stsc.End()-(stsc.Payload()+8) {
		return nil, fmt.Errorf("stsc entry count %d exceeds box payload", count)
	}
	runs := make([]chunkRun, 0, count)
	pos := stsc.Payload() + 8
	for i := uint32(0); i < count; i++ {
		first, err := readU32(r, pos)
		if err != nil {
			return nil, err
		}
		spc, err := readU32(r, pos+4)
		if err != nil {
			return nil, err
		}
		runs = append(runs, chunkRun{FirstChunk: first, SamplesPerChunk: spc})
		pos += 12
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].FirstChunk < runs[j].FirstChunk })
	if len(runs) > 0 && runs[0].FirstChunk != 1 {
		return nil, fmt.Errorf("first stsc run starts at chunk %d, want 1", runs[0].FirstChunk)
	}
	return runs, nil
}

func samplesPerChunk(runs []chunkRun, chunk uint32) uint32 {
	var spc uint32
	for _, run := range runs {
		if run.FirstChunk > chunk {
			break
		}
		spc = run.SamplesPerChunk
	}
	return spc
}
