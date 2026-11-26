package mp4seek

import (
	"encoding/binary"
	"fmt"
	"sort"
)

type timeEntry struct {
	count uint32
	value int64
}

func (f *File) parseSampleTables(track *Track) error {
	sizes, err := f.parseSampleSizes(track)
	if err != nil {
		return err
	}
	sampleCount := len(sizes)
	chunkOffsets, err := f.parseChunkOffsets(track)
	if err != nil {
		return err
	}
	chunkSamples, err := f.parseChunkMapping(track, len(chunkOffsets))
	if err != nil {
		return err
	}
	durations, err := f.parseTimeToSample(track, sampleCount)
	if err != nil {
		return err
	}
	composition, err := f.parseCompositionOffsets(track, sampleCount)
	if err != nil {
		return err
	}
	track.Samples = make([]Sample, sampleCount)
	if err := assignSampleOffsets(track, sizes, chunkOffsets, chunkSamples, f.Size); err != nil {
		return err
	}
	var dts int64
	for i := range track.Samples {
		sample := &track.Samples[i]
		sample.Number = uint32(i + 1)
		sample.DTS = dts
		sample.CTS = dts + composition[i]
		sample.Duration = durations[i]
		dts += int64(durations[i])
	}
	if err := f.parseSyncSamples(track); err != nil {
		return err
	}
	f.parseSampleDependencies(track)
	return nil
}

func (f *File) parseSampleSizes(track *Track) ([]uint32, error) {
	stsz, hasStsz, err := findBox(f.data, track.stbl, "stsz")
	if err != nil {
		return nil, err
	}
	if hasStsz {
		if err := needBytes(f.data, stsz.payload, 12, "stsz"); err != nil {
			return nil, err
		}
		fixed := binary.BigEndian.Uint32(f.data[stsz.payload+4 : stsz.payload+8])
		count := binary.BigEndian.Uint32(f.data[stsz.payload+8 : stsz.payload+12])
		if fixed != 0 {
			sizes := make([]uint32, count)
			for i := range sizes {
				sizes[i] = fixed
			}
			return sizes, nil
		}
		if err := needBytes(f.data, stsz.payload+12, int(count)*4, "stsz entries"); err != nil {
			return nil, err
		}
		sizes := make([]uint32, count)
		for i := range sizes {
			sizes[i] = binary.BigEndian.Uint32(f.data[stsz.payload+12+i*4:])
		}
		return sizes, nil
	}

	stz2, hasStz2, err := findBox(f.data, track.stbl, "stz2")
	if err != nil {
		return nil, err
	}
	if !hasStz2 {
		return nil, fmt.Errorf("missing stsz/stz2")
	}
	if err := needBytes(f.data, stz2.payload, 12, "stz2"); err != nil {
		return nil, err
	}
	fieldSize := f.data[stz2.payload+7]
	count := binary.BigEndian.Uint32(f.data[stz2.payload+8 : stz2.payload+12])
	sizes := make([]uint32, count)
	dataOff := stz2.payload + 12
	switch fieldSize {
	case 4:
		if err := needBytes(f.data, dataOff, (int(count)+1)/2, "stz2 entries"); err != nil {
			return nil, err
		}
		for i := range sizes {
			b := f.data[dataOff+i/2]
			if i%2 == 0 {
				sizes[i] = uint32(b >> 4)
			} else {
				sizes[i] = uint32(b & 0x0f)
			}
		}
	case 8:
		if err := needBytes(f.data, dataOff, int(count), "stz2 entries"); err != nil {
			return nil, err
		}
		for i := range sizes {
			sizes[i] = uint32(f.data[dataOff+i])
		}
	case 16:
		if err := needBytes(f.data, dataOff, int(count)*2, "stz2 entries"); err != nil {
			return nil, err
		}
		for i := range sizes {
			sizes[i] = uint32(binary.BigEndian.Uint16(f.data[dataOff+i*2:]))
		}
	default:
		return nil, fmt.Errorf("unsupported stz2 field size %d", fieldSize)
	}
	return sizes, nil
}

func (f *File) parseChunkOffsets(track *Track) ([]uint64, error) {
	stco, has32, err := findBox(f.data, track.stbl, "stco")
	if err != nil {
		return nil, err
	}
	co64, has64, err := findBox(f.data, track.stbl, "co64")
	if err != nil {
		return nil, err
	}
	if !has32 && !has64 {
		return nil, fmt.Errorf("missing stco/co64")
	}
	offsetBox := stco
	width := 4
	if has64 {
		offsetBox = co64
		width = 8
	}
	if err := needBytes(f.data, offsetBox.payload, 8, "chunk offset header"); err != nil {
		return nil, err
	}
	count := binary.BigEndian.Uint32(f.data[offsetBox.payload+4 : offsetBox.payload+8])
	if err := needBytes(f.data, offsetBox.payload+8, int(count)*width, "chunk offsets"); err != nil {
		return nil, err
	}
	offsets := make([]uint64, count)
	for i := range offsets {
		off := offsetBox.payload + 8 + i*width
		if width == 8 {
			offsets[i] = binary.BigEndian.Uint64(f.data[off:])
		} else {
			offsets[i] = uint64(binary.BigEndian.Uint32(f.data[off:]))
		}
	}
	return offsets, nil
}

func (f *File) parseChunkMapping(track *Track, chunkCount int) ([]uint32, error) {
	stsc, ok, err := findBox(f.data, track.stbl, "stsc")
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("missing stsc")
	}
	if err := needBytes(f.data, stsc.payload, 8, "stsc header"); err != nil {
		return nil, err
	}
	count := binary.BigEndian.Uint32(f.data[stsc.payload+4 : stsc.payload+8])
	if err := needBytes(f.data, stsc.payload+8, int(count)*12, "stsc entries"); err != nil {
		return nil, err
	}
	type mapping struct {
		firstChunk uint32
		samples    uint32
	}
	mappings := make([]mapping, count)
	for i := range mappings {
		off := stsc.payload + 8 + i*12
		mappings[i] = mapping{
			firstChunk: binary.BigEndian.Uint32(f.data[off:]),
			samples:    binary.BigEndian.Uint32(f.data[off+4:]),
		}
		if mappings[i].firstChunk == 0 {
			return nil, fmt.Errorf("stsc entry %d has zero first_chunk", i+1)
		}
		if i > 0 && mappings[i].firstChunk <= mappings[i-1].firstChunk {
			return nil, fmt.Errorf("stsc first_chunk values are not increasing")
		}
	}
	out := make([]uint32, chunkCount)
	for i, mapping := range mappings {
		endChunk := uint32(chunkCount)
		if i+1 < len(mappings) {
			endChunk = mappings[i+1].firstChunk - 1
		}
		if mapping.firstChunk > uint32(chunkCount) {
			return nil, fmt.Errorf("stsc first_chunk %d exceeds chunk count %d", mapping.firstChunk, chunkCount)
		}
		for chunk := mapping.firstChunk; chunk <= endChunk; chunk++ {
			out[chunk-1] = mapping.samples
		}
	}
	return out, nil
}

func assignSampleOffsets(track *Track, sizes []uint32, chunks []uint64, chunkSamples []uint32, fileSize int64) error {
	sampleIndex := 0
	for chunkIndex, samplesInChunk := range chunkSamples {
		offset := chunks[chunkIndex]
		for i := uint32(0); i < samplesInChunk; i++ {
			if sampleIndex >= len(sizes) {
				return fmt.Errorf("chunk mapping has more samples than stsz")
			}
			size := sizes[sampleIndex]
			if offset+uint64(size) > uint64(fileSize) {
				return fmt.Errorf("sample %d range [%d,%d) exceeds file size %d", sampleIndex+1, offset, offset+uint64(size), fileSize)
			}
			track.Samples[sampleIndex].Size = size
			track.Samples[sampleIndex].Offset = offset
			offset += uint64(size)
			sampleIndex++
		}
	}
	if sampleIndex != len(sizes) {
		return fmt.Errorf("chunk mapping covers %d samples, stsz has %d", sampleIndex, len(sizes))
	}
	return nil
}

func (f *File) parseTimeToSample(track *Track, sampleCount int) ([]uint32, error) {
	stts, ok, err := findBox(f.data, track.stbl, "stts")
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("missing stts")
	}
	entries, err := parseRunLengthEntries(f.data, stts, "stts")
	if err != nil {
		return nil, err
	}
	out := make([]uint32, 0, sampleCount)
	for _, entry := range entries {
		for i := uint32(0); i < entry.count && len(out) < sampleCount; i++ {
			out = append(out, uint32(entry.value))
		}
	}
	if len(out) != sampleCount {
		return nil, fmt.Errorf("stts covers %d samples, expected %d", len(out), sampleCount)
	}
	return out, nil
}

func (f *File) parseCompositionOffsets(track *Track, sampleCount int) ([]int64, error) {
	ctts, ok, err := findBox(f.data, track.stbl, "ctts")
	if err != nil {
		return nil, err
	}
	if !ok {
		return make([]int64, sampleCount), nil
	}
	entries, err := parseRunLengthEntries(f.data, ctts, "ctts")
	if err != nil {
		return nil, err
	}
	signed := false
	if err := needBytes(f.data, ctts.payload, 1, "ctts version"); err != nil {
		return nil, err
	}
	signed = f.data[ctts.payload] == 1
	out := make([]int64, 0, sampleCount)
	for _, entry := range entries {
		for i := uint32(0); i < entry.count && len(out) < sampleCount; i++ {
			value := entry.value
			if signed {
				value = int64(int32(entry.value))
			}
			out = append(out, value)
		}
	}
	if len(out) != sampleCount {
		return nil, fmt.Errorf("ctts covers %d samples, expected %d", len(out), sampleCount)
	}
	return out, nil
}

func parseRunLengthEntries(data []byte, b box, what string) ([]timeEntry, error) {
	if err := needBytes(data, b.payload, 8, what+" header"); err != nil {
		return nil, err
	}
	count := binary.BigEndian.Uint32(data[b.payload+4 : b.payload+8])
	if err := needBytes(data, b.payload+8, int(count)*8, what+" entries"); err != nil {
		return nil, err
	}
	entries := make([]timeEntry, count)
	for i := range entries {
		off := b.payload + 8 + i*8
		entries[i] = timeEntry{
			count: binary.BigEndian.Uint32(data[off:]),
			value: int64(binary.BigEndian.Uint32(data[off+4:])),
		}
	}
	return entries, nil
}

func (f *File) parseSyncSamples(track *Track) error {
	track.hasSTSS = false
	track.rawSTSS = nil
	for i := range track.Samples {
		track.Samples[i].IndexedSync = true
	}
	stss, ok, err := findBox(f.data, track.stbl, "stss")
	if err != nil {
		return err
	}
	if !ok {
		for i := range track.Samples {
			track.IndexedSyncSamples = append(track.IndexedSyncSamples, uint32(i+1))
		}
		return nil
	}
	if err := needBytes(f.data, stss.payload, 8, "stss header"); err != nil {
		return err
	}
	count := binary.BigEndian.Uint32(f.data[stss.payload+4 : stss.payload+8])
	if err := needBytes(f.data, stss.payload+8, int(count)*4, "stss entries"); err != nil {
		return err
	}
	track.hasSTSS = true
	for i := range track.Samples {
		track.Samples[i].IndexedSync = false
	}
	seen := make(map[uint32]bool, count)
	for i := uint32(0); i < count; i++ {
		off := stss.payload + 8 + int(i)*4
		number := binary.BigEndian.Uint32(f.data[off:])
		track.rawSTSS = append(track.rawSTSS, number)
		if err := validateSampleNumber(number, len(track.Samples)); err != nil {
			track.Warnings = append(track.Warnings, "stss: "+err.Error())
			continue
		}
		if seen[number] {
			track.Warnings = append(track.Warnings, fmt.Sprintf("stss contains duplicate sample %d", number))
			continue
		}
		seen[number] = true
		track.IndexedSyncSamples = append(track.IndexedSyncSamples, number)
		track.Samples[number-1].IndexedSync = true
	}
	sort.Slice(track.IndexedSyncSamples, func(i, j int) bool {
		return track.IndexedSyncSamples[i] < track.IndexedSyncSamples[j]
	})
	return nil
}

func (f *File) parseSampleDependencies(track *Track) {
	sdtp, ok, err := findBox(f.data, track.stbl, "sdtp")
	if err != nil || !ok {
		return
	}
	start := sdtp.payload + 4
	available := sdtp.end - start
	if available > len(track.Samples) {
		available = len(track.Samples)
	}
	for i := 0; i < available; i++ {
		switch (f.data[start+i] >> 4) & 3 {
		case 1:
			track.Samples[i].SDTPDependency = "depends"
		case 2:
			track.Samples[i].SDTPDependency = "independent"
		default:
			track.Samples[i].SDTPDependency = "unknown"
		}
	}
}
