package mp4

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Sample 描述一个音视频样本(帧/音频包)在 mdat 中的位置与时间。
type Sample struct {
	Index    int    // 解码序样本序号(0 起)
	Offset   int64  // 文件内偏移
	Size     int64  // 字节数
	DTS      uint64 // 解码时间戳(媒体时基)
	Duration uint32 // 样本时长(媒体时基)
}

// Track 是一条媒体轨的样本表与编解码标识。
type Track struct {
	TrackID   uint32
	Handler   string // vide / soun / ...
	Codec     string // stsd 首条目类型,如 avc1 / mp4a
	Timescale uint32
	Samples   []Sample
}

// ExtractTracks 从 moov 盒提取所有媒体轨样本表。
func ExtractTracks(r io.ReaderAt, moov *Box) ([]*Track, error) {
	var tracks []*Track
	for _, trak := range moov.FindAll("trak") {
		t, err := parseTrak(r, trak)
		if err != nil {
			return nil, err
		}
		tracks = append(tracks, t)
	}
	if len(tracks) == 0 {
		return nil, fmt.Errorf("mp4: 未找到媒体轨")
	}
	return tracks, nil
}

func parseTrak(r io.ReaderAt, trak *Box) (*Track, error) {
	mdia := trak.Find("mdia")
	if mdia == nil {
		return nil, fmt.Errorf("mp4: trak 缺少 mdia")
	}
	minf := mdia.Find("minf")
	if minf == nil {
		return nil, fmt.Errorf("mp4: mdia 缺少 minf")
	}
	stbl := minf.Find("stbl")
	if stbl == nil {
		return nil, fmt.Errorf("mp4: minf 缺少 stbl(可能为分片 MP4,暂不支持)")
	}

	t := &Track{}
	if tkhd := trak.Find("tkhd"); tkhd != nil {
		t.TrackID = parseTkhdID(r, tkhd)
	}
	if mdhd := mdia.Find("mdhd"); mdhd != nil {
		t.Timescale = parseMdhdTimescale(r, mdhd)
	}
	if hdlr := mdia.Find("hdlr"); hdlr != nil {
		t.Handler = parseHdlr(r, hdlr)
	}
	if stsd := stbl.Find("stsd"); stsd != nil {
		t.Codec = parseStsdCodec(r, stsd)
	}

	sizes, err := parseStsz(r, stbl.Find("stsz"))
	if err != nil {
		return nil, err
	}
	deltas, err := parseStts(r, stbl.Find("stts"))
	if err != nil {
		return nil, err
	}
	chunks, err := parseChunkOffsets(r, stbl)
	if err != nil {
		return nil, err
	}
	stsc, err := parseStsc(r, stbl.Find("stsc"))
	if err != nil {
		return nil, err
	}
	t.Samples, err = buildSamples(sizes, deltas, chunks, stsc)
	if err != nil {
		return nil, err
	}
	return t, nil
}

func readBoxData(r io.ReaderAt, b *Box, what string) ([]byte, error) {
	if b == nil {
		return nil, fmt.Errorf("mp4: 缺少 %s 盒", what)
	}
	data, err := ReadData(r, b)
	if err != nil {
		return nil, fmt.Errorf("mp4: 读 %s 失败: %w", what, err)
	}
	return data, nil
}

func parseTkhdID(r io.ReaderAt, b *Box) uint32 {
	data, err := ReadData(r, b)
	if err != nil || len(data) < 20 {
		return 0
	}
	if data[0] == 1 { // version 1
		return binary.BigEndian.Uint32(data[20:24])
	}
	return binary.BigEndian.Uint32(data[12:16])
}

func parseMdhdTimescale(r io.ReaderAt, b *Box) uint32 {
	data, err := ReadData(r, b)
	if err != nil || len(data) < 20 {
		return 0
	}
	if data[0] == 1 {
		return binary.BigEndian.Uint32(data[20:24])
	}
	return binary.BigEndian.Uint32(data[12:16])
}

func parseHdlr(r io.ReaderAt, b *Box) string {
	data, err := ReadData(r, b)
	if err != nil || len(data) < 12 {
		return ""
	}
	return string(data[8:12])
}

func parseStsdCodec(r io.ReaderAt, b *Box) string {
	data, err := ReadData(r, b)
	if err != nil || len(data) < 16 {
		return ""
	}
	return string(data[12:16]) // 首条目类型
}
