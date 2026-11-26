package mp4

import (
	"encoding/binary"
	"fmt"
	"io"
)

// stscEntry 对应 stsc 表项:从 firstChunk 起每 chunk 含 samplesPerChunk 个样本。
type stscEntry struct {
	firstChunk      uint32 // 1 起
	samplesPerChunk uint32
}

func parseStsz(r io.ReaderAt, b *Box) ([]uint32, error) {
	data, err := readBoxData(r, b, "stsz")
	if err != nil {
		return nil, err
	}
	if len(data) < 12 {
		return nil, fmt.Errorf("mp4: stsz 过短")
	}
	uniform := binary.BigEndian.Uint32(data[4:8])
	count := binary.BigEndian.Uint32(data[8:12])
	sizes := make([]uint32, count)
	if uniform != 0 {
		for i := range sizes {
			sizes[i] = uniform
		}
		return sizes, nil
	}
	if len(data) < 12+int(count)*4 {
		return nil, fmt.Errorf("mp4: stsz 表截断")
	}
	for i := range sizes {
		sizes[i] = binary.BigEndian.Uint32(data[12+i*4:])
	}
	return sizes, nil
}

func parseStts(r io.ReaderAt, b *Box) ([]uint32, error) {
	data, err := readBoxData(r, b, "stts")
	if err != nil {
		return nil, err
	}
	if len(data) < 8 {
		return nil, fmt.Errorf("mp4: stts 过短")
	}
	n := binary.BigEndian.Uint32(data[4:8])
	if len(data) < 8+int(n)*8 {
		return nil, fmt.Errorf("mp4: stts 表截断")
	}
	var deltas []uint32
	for i := uint32(0); i < n; i++ {
		count := binary.BigEndian.Uint32(data[8+i*8:])
		delta := binary.BigEndian.Uint32(data[12+i*8:])
		for j := uint32(0); j < count; j++ {
			deltas = append(deltas, delta)
		}
	}
	return deltas, nil
}

func parseChunkOffsets(r io.ReaderAt, stbl *Box) ([]int64, error) {
	if b := stbl.Find("stco"); b != nil {
		data, err := readBoxData(r, b, "stco")
		if err != nil {
			return nil, err
		}
		n := binary.BigEndian.Uint32(data[4:8])
		out := make([]int64, n)
		for i := range out {
			out[i] = int64(binary.BigEndian.Uint32(data[8+i*4:]))
		}
		return out, nil
	}
	if b := stbl.Find("co64"); b != nil {
		data, err := readBoxData(r, b, "co64")
		if err != nil {
			return nil, err
		}
		n := binary.BigEndian.Uint32(data[4:8])
		out := make([]int64, n)
		for i := range out {
			out[i] = int64(binary.BigEndian.Uint64(data[8+i*8:]))
		}
		return out, nil
	}
	return nil, fmt.Errorf("mp4: 缺少 stco/co64")
}

func parseStsc(r io.ReaderAt, b *Box) ([]stscEntry, error) {
	data, err := readBoxData(r, b, "stsc")
	if err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(data[4:8])
	out := make([]stscEntry, n)
	for i := range out {
		out[i] = stscEntry{
			firstChunk:      binary.BigEndian.Uint32(data[8+i*12:]),
			samplesPerChunk: binary.BigEndian.Uint32(data[12+i*12:]),
		}
	}
	return out, nil
}

// buildSamples 依据 stsc+stco+stsz 展开每个样本的文件位置,并用 stts 赋 DTS。
func buildSamples(sizes, deltas []uint32, chunks []int64, stsc []stscEntry) ([]Sample, error) {
	n := len(sizes)
	if len(deltas) < n {
		return nil, fmt.Errorf("mp4: stts 条目数 %d 少于样本数 %d", len(deltas), n)
	}
	samples := make([]Sample, n)
	idx := 0
	for ci, chunkOff := range chunks {
		chunk := uint32(ci + 1)
		spc := stscForChunk(stsc, chunk)
		off := chunkOff
		for j := uint32(0); j < spc && idx < n; j++ {
			samples[idx] = Sample{Index: idx, Offset: off, Size: int64(sizes[idx])}
			off += int64(sizes[idx])
			idx++
		}
	}
	if idx != n {
		return nil, fmt.Errorf("mp4: chunk 展开样本数 %d 与 stsz %d 不符", idx, n)
	}
	var dts uint64
	for i := range samples {
		samples[i].DTS = dts
		samples[i].Duration = deltas[i]
		dts += uint64(deltas[i])
	}
	return samples, nil
}

func stscForChunk(stsc []stscEntry, chunk uint32) uint32 {
	spc := uint32(0)
	for _, e := range stsc {
		if e.firstChunk <= chunk {
			spc = e.samplesPerChunk
		} else {
			break
		}
	}
	return spc
}
