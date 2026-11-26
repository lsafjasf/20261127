package mp4seek

import (
	"encoding/binary"
	"fmt"
	"os"
)

func ParseFile(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	file, err := Parse(data)
	if err != nil {
		return nil, err
	}
	file.Path = path
	return file, nil
}

func Parse(data []byte) (*File, error) {
	if len(data) < boxHeaderSize {
		return nil, fmt.Errorf("file is too small to contain an MP4 box")
	}
	top, err := parseBoxes(data, 0, len(data))
	if err != nil {
		return nil, fmt.Errorf("parse top-level boxes: %w", err)
	}
	file := &File{Size: int64(len(data)), data: data}
	var moov *box
	for _, b := range top {
		switch b.typ {
		case "moov":
			if moov != nil {
				return nil, fmt.Errorf("multiple moov boxes are not supported")
			}
			copy := b
			moov = &copy
		case "mdat":
			file.MDATs = append(file.MDATs, PayloadSpan{
				Offset: uint64(b.payload),
				Size:   uint64(b.payloadSize()),
			})
		case "moof":
			return nil, fmt.Errorf("fragmented MP4 (moof) is not supported; sample tables must be in moov")
		}
	}
	if moov == nil {
		return nil, fmt.Errorf("missing moov box")
	}
	file.moov = *moov
	mvhd, ok, err := findBox(file.data, file.moov, "mvhd")
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("missing mvhd box")
	}
	if file.MovieTimescale, err = parseMovieTimescale(file.data, mvhd); err != nil {
		return nil, err
	}
	if err := file.parseTracks(); err != nil {
		return nil, err
	}
	if err := file.scanBitstreams(); err != nil {
		return nil, err
	}
	return file, nil
}

func (f *File) parseTracks() error {
	traks, err := findBoxes(f.data, f.moov, "trak")
	if err != nil {
		return fmt.Errorf("parse trak boxes: %w", err)
	}
	for i, trak := range traks {
		track, err := f.parseTrak(trak)
		if err != nil {
			return fmt.Errorf("parse trak %d: %w", i+1, err)
		}
		f.Tracks = append(f.Tracks, track)
	}
	if len(f.Tracks) == 0 {
		return fmt.Errorf("moov contains no tracks")
	}
	return nil
}

func (f *File) parseTrak(trak box) (*Track, error) {
	tkhd, ok, err := findBox(f.data, trak, "tkhd")
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("missing tkhd")
	}
	mdia, ok, err := findBox(f.data, trak, "mdia")
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("missing mdia")
	}
	track := &Track{nalLength: 4}
	if track.ID, err = parseTrackID(f.data, tkhd); err != nil {
		return nil, err
	}
	mdhd, ok, err := findBox(f.data, mdia, "mdhd")
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("track %d: missing mdhd", track.ID)
	}
	if track.Timescale, track.Duration, err = parseMediaHeader(f.data, mdhd); err != nil {
		return nil, fmt.Errorf("track %d: %w", track.ID, err)
	}
	if err := f.parseEditList(track, trak); err != nil {
		track.Warnings = append(track.Warnings, "edit list: "+err.Error())
	}
	hdlr, ok, err := findBox(f.data, mdia, "hdlr")
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("track %d: missing hdlr", track.ID)
	}
	if track.Handler, err = parseHandler(f.data, hdlr); err != nil {
		return nil, fmt.Errorf("track %d: %w", track.ID, err)
	}
	minf, ok, err := findBox(f.data, mdia, "minf")
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("track %d: missing minf", track.ID)
	}
	stbl, ok, err := findBox(f.data, minf, "stbl")
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("track %d: missing stbl", track.ID)
	}
	track.stbl = stbl
	track.stblPath = []box{stbl, minf, mdia, trak}
	track.ownerData = f.data
	if err := f.parseSampleTables(track); err != nil {
		return nil, fmt.Errorf("track %d: %w", track.ID, err)
	}
	if err := f.parseCodec(track); err != nil {
		return nil, fmt.Errorf("track %d: %w", track.ID, err)
	}
	return track, nil
}

func parseTrackID(data []byte, tkhd box) (uint32, error) {
	if err := needBytes(data, tkhd.payload, 1, "tkhd version"); err != nil {
		return 0, err
	}
	switch data[tkhd.payload] {
	case 0:
		return readU32(data, tkhd.payload+12, "tkhd track_ID")
	case 1:
		return readU32(data, tkhd.payload+20, "tkhd track_ID")
	default:
		return 0, fmt.Errorf("unsupported tkhd version %d", data[tkhd.payload])
	}
}

func parseMediaHeader(data []byte, mdhd box) (uint32, uint64, error) {
	if err := needBytes(data, mdhd.payload, 1, "mdhd version"); err != nil {
		return 0, 0, err
	}
	switch data[mdhd.payload] {
	case 0:
		timescale, err := readU32(data, mdhd.payload+12, "mdhd timescale")
		if err != nil {
			return 0, 0, err
		}
		duration, err := readU32(data, mdhd.payload+16, "mdhd duration")
		return timescale, uint64(duration), err
	case 1:
		timescale, err := readU32(data, mdhd.payload+20, "mdhd timescale")
		if err != nil {
			return 0, 0, err
		}
		duration, err := readU64(data, mdhd.payload+24, "mdhd duration")
		return timescale, duration, err
	default:
		return 0, 0, fmt.Errorf("unsupported mdhd version %d", data[mdhd.payload])
	}
}

func parseHandler(data []byte, hdlr box) (string, error) {
	if err := needBytes(data, hdlr.payload, 12, "hdlr"); err != nil {
		return "", err
	}
	return string(data[hdlr.payload+8 : hdlr.payload+12]), nil
}

func (f *File) parseCodec(track *Track) error {
	stsd, ok, err := findBox(f.data, track.stbl, "stsd")
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("missing stsd")
	}
	if err := needBytes(f.data, stsd.payload, 8, "stsd header"); err != nil {
		return err
	}
	entryCount := binary.BigEndian.Uint32(f.data[stsd.payload+4 : stsd.payload+8])
	if entryCount == 0 {
		return fmt.Errorf("stsd has no sample entries")
	}
	entries, err := parseBoxes(f.data, stsd.payload+8, stsd.end)
	if err != nil {
		return fmt.Errorf("parse stsd entries: %w", err)
	}
	if len(entries) == 0 {
		return fmt.Errorf("stsd entry count is %d but no entries exist", entryCount)
	}
	entry := entries[0]
	track.Codec = entry.typ
	if track.Handler != "vide" {
		return nil
	}
	childStart := entry.payload + 78
	if childStart > entry.end {
		track.Warnings = append(track.Warnings, "visual sample entry is truncated; bitstream scan disabled")
		return nil
	}
	children, err := parseBoxes(f.data, childStart, entry.end)
	if err != nil {
		track.Warnings = append(track.Warnings, "cannot parse visual sample entry: "+err.Error())
		return nil
	}
	for _, child := range children {
		cfg := f.data[child.payload:child.end]
		switch child.typ {
		case "avcC":
			track.configSeen = true
			if len(cfg) >= 5 {
				track.nalLength = int(cfg[4]&3) + 1
			}
		case "hvcC":
			track.configSeen = true
			track.hevc = true
			if len(cfg) >= 22 {
				track.nalLength = int(cfg[21]&3) + 1
			}
		}
	}
	return nil
}

func (f *File) scanBitstreams() error {
	for _, track := range f.Tracks {
		if track.Handler == "vide" {
			f.scanTrackBitstream(track)
		}
	}
	return nil
}
