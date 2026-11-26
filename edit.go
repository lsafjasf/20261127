package mp4seek

import (
	"encoding/binary"
	"fmt"
)

func parseMovieTimescale(data []byte, mvhd box) (uint32, error) {
	if err := needBytes(data, mvhd.payload, 1, "mvhd version"); err != nil {
		return 0, err
	}
	switch data[mvhd.payload] {
	case 0:
		return readU32(data, mvhd.payload+12, "mvhd timescale")
	case 1:
		return readU32(data, mvhd.payload+20, "mvhd timescale")
	default:
		return 0, fmt.Errorf("unsupported mvhd version %d", data[mvhd.payload])
	}
}

func (f *File) parseEditList(track *Track, trak box) error {
	edts, ok, err := findBox(f.data, trak, "edts")
	if err != nil || !ok {
		return err
	}
	elst, ok, err := findBox(f.data, edts, "elst")
	if err != nil || !ok {
		return err
	}
	if err := needBytes(f.data, elst.payload, 8, "elst header"); err != nil {
		return err
	}
	version := f.data[elst.payload]
	entryCount := binary.BigEndian.Uint32(f.data[elst.payload+4 : elst.payload+8])
	entrySize := 12
	if version == 1 {
		entrySize = 20
	} else if version != 0 {
		return fmt.Errorf("unsupported elst version %d", version)
	}
	if err := needBytes(f.data, elst.payload+8, int(entryCount)*entrySize, "elst entries"); err != nil {
		return err
	}
	if f.MovieTimescale == 0 {
		return fmt.Errorf("movie timescale is zero")
	}
	var delay int64
	nonEmpty := 0
	for i := uint32(0); i < entryCount; i++ {
		off := elst.payload + 8 + int(i)*entrySize
		var segmentDuration uint64
		var mediaTime int64
		if version == 1 {
			segmentDuration = binary.BigEndian.Uint64(f.data[off : off+8])
			mediaTime = int64(binary.BigEndian.Uint64(f.data[off+8 : off+16]))
		} else {
			segmentDuration = uint64(binary.BigEndian.Uint32(f.data[off : off+4]))
			mediaTime = int64(int32(binary.BigEndian.Uint32(f.data[off+4 : off+8])))
		}
		if mediaTime == -1 {
			delay += convertTimescale(segmentDuration, f.MovieTimescale, track.Timescale)
			continue
		}
		nonEmpty++
		if nonEmpty == 1 {
			track.EditOffset = delay - mediaTime
		}
	}
	if nonEmpty > 1 {
		track.Warnings = append(track.Warnings,
			fmt.Sprintf("elst has %d non-empty edits; presentation times use the first edit", nonEmpty))
	}
	return nil
}

func convertTimescale(value uint64, from, to uint32) int64 {
	if from == 0 || value == 0 {
		return 0
	}
	whole := value / uint64(from)
	rem := value % uint64(from)
	if whole > uint64(^uint64(0)>>1)/uint64(to) {
		return int64(^uint64(0) >> 1)
	}
	return int64(whole*uint64(to) + rem*uint64(to)/uint64(from))
}
