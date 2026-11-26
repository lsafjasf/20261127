package mp4

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
)

func Open(path string) (*File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() < 8 {
		return nil, errors.New("file is too small to be an ISO BMFF file")
	}
	boxes, err := readBoxes(f, 0, info.Size(), 0)
	if err != nil {
		return nil, err
	}
	moov := firstBox(boxes, "moov")
	if moov == nil {
		return nil, errors.New("moov box not found (fragmented-only files are not supported)")
	}
	result := &File{Path: path, Size: info.Size()}
	if err := parseMovie(f, moov, result); err != nil {
		return nil, err
	}
	for _, trak := range childrenOf(moov, "trak") {
		tr, err := parseTrack(f, trak, result.Movie.Timescale)
		if err != nil {
			return nil, err
		}
		result.Tracks = append(result.Tracks, tr)
	}
	for _, tr := range result.Tracks {
		for _, s := range tr.Samples {
			if s.FileOffset > uint64(info.Size()) || uint64(s.Size) > uint64(info.Size())-s.FileOffset {
				return nil, fmt.Errorf("track %d sample %d range is outside the file", tr.ID, s.Index)
			}
		}
	}
	return result, nil
}

func firstBox(boxes []*Box, typ string) *Box {
	for _, b := range boxes {
		if b.Type == typ {
			return b
		}
	}
	return nil
}

func childrenOf(b *Box, typ string) []*Box {
	var result []*Box
	for _, c := range b.Children {
		if c.Type == typ {
			result = append(result, c)
		}
	}
	return result
}

func parseMovie(r io.ReaderAt, moov *Box, out *File) error {
	mvhd := child(moov, "mvhd")
	if mvhd == nil {
		return errors.New("moov/mvhd is missing")
	}
	version, err := fullBoxVersion(r, mvhd)
	if err != nil {
		return err
	}
	switch version {
	case 0:
		out.Movie.Timescale, err = readU32(r, mvhd.Payload()+12)
		if err == nil {
			var duration uint32
			duration, err = readU32(r, mvhd.Payload()+16)
			out.Movie.Duration = uint64(duration)
		}
	case 1:
		out.Movie.Timescale, err = readU32(r, mvhd.Payload()+20)
		if err == nil {
			out.Movie.Duration, err = readU64(r, mvhd.Payload()+24)
		}
	default:
		return fmt.Errorf("unsupported mvhd version %d", version)
	}
	if err != nil {
		return fmt.Errorf("parse mvhd: %w", err)
	}
	if out.Movie.Timescale == 0 {
		return errors.New("mvhd has zero timescale")
	}
	return nil
}

func parseTrack(r io.ReaderAt, trak *Box, movieTimescale uint32) (*Track, error) {
	t := &Track{MovieTimescale: movieTimescale}
	if err := parseTrackHeader(r, trak, t); err != nil {
		return nil, err
	}
	mdia := child(trak, "mdia")
	if mdia == nil {
		return nil, fmt.Errorf("track %d: mdia is missing", t.ID)
	}
	if err := parseMediaHeader(r, child(mdia, "mdhd"), t); err != nil {
		return nil, err
	}
	if err := parseHandler(r, child(mdia, "hdlr"), t); err != nil {
		return nil, err
	}
	stbl := path(mdia, "minf", "stbl")
	if stbl == nil {
		return nil, fmt.Errorf("track %d: stbl is missing", t.ID)
	}
	if err := parseSampleDescription(r, child(stbl, "stsd"), t); err != nil {
		return nil, err
	}
	elst := path(trak, "edts", "elst")
	if elst != nil {
		if err := parseEditList(r, elst, t); err != nil {
			return nil, err
		}
	}
	if err := parseSyncSamples(r, child(stbl, "stss"), t); err != nil {
		return nil, err
	}
	if err := parseSamples(r, stbl, t); err != nil {
		return nil, err
	}
	return t, nil
}

func parseTrackHeader(r io.ReaderAt, trak *Box, t *Track) error {
	tkhd := child(trak, "tkhd")
	if tkhd == nil {
		return errors.New("trak/tkhd is missing")
	}
	version, err := fullBoxVersion(r, tkhd)
	if err != nil {
		return err
	}
	switch version {
	case 0:
		t.ID, err = readU32(r, tkhd.Payload()+12)
	case 1:
		t.ID, err = readU32(r, tkhd.Payload()+20)
	default:
		return fmt.Errorf("unsupported tkhd version %d", version)
	}
	if err != nil {
		return fmt.Errorf("parse tkhd: %w", err)
	}
	return nil
}

func parseMediaHeader(r io.ReaderAt, mdhd *Box, t *Track) error {
	if mdhd == nil {
		return fmt.Errorf("track %d: mdhd is missing", t.ID)
	}
	version, err := fullBoxVersion(r, mdhd)
	if err != nil {
		return err
	}
	var lang uint16
	switch version {
	case 0:
		if t.Timescale, err = readU32(r, mdhd.Payload()+12); err == nil {
			var duration uint32
			duration, err = readU32(r, mdhd.Payload()+16)
			t.Duration = uint64(duration)
		}
		if err == nil {
			lang, err = readU16(r, mdhd.Payload()+20)
		}
	case 1:
		if t.Timescale, err = readU32(r, mdhd.Payload()+20); err == nil {
			t.Duration, err = readU64(r, mdhd.Payload()+24)
		}
		if err == nil {
			lang, err = readU16(r, mdhd.Payload()+32)
		}
	default:
		return fmt.Errorf("unsupported mdhd version %d", version)
	}
	if err != nil {
		return fmt.Errorf("parse mdhd: %w", err)
	}
	if t.Timescale == 0 {
		return fmt.Errorf("track %d has zero media timescale", t.ID)
	}
	t.Language = decodeLanguage(lang)
	return nil
}

func decodeLanguage(v uint16) string {
	if v == 0 || v == 0x7fff {
		return ""
	}
	b := []byte{
		byte((v>>10)&0x1f) + '`',
		byte((v>>5)&0x1f) + '`',
		byte(v&0x1f) + '`',
	}
	if !bytes.Equal(b[:1], []byte("`")) {
		return string(b)
	}
	return ""
}

func parseHandler(r io.ReaderAt, hdlr *Box, t *Track) error {
	if hdlr == nil {
		return nil
	}
	var typ [4]byte
	if _, err := r.ReadAt(typ[:], hdlr.Payload()+8); err != nil {
		return fmt.Errorf("parse hdlr: %w", err)
	}
	t.Handler = string(typ[:])
	return nil
}

func parseSampleDescription(r io.ReaderAt, stsd *Box, t *Track) error {
	if stsd == nil {
		return nil
	}
	if _, err := readU32(r, stsd.Payload()+4); err != nil {
		return nil
	}
	entry := stsd.Payload() + 8
	if entry+8 > stsd.End() {
		return fmt.Errorf("track %d: truncated stsd entry", t.ID)
	}
	var typ [4]byte
	if _, err := r.ReadAt(typ[:], entry+4); err != nil {
		return fmt.Errorf("track %d: read stsd codec: %w", t.ID, err)
	}
	t.Codec = string(typ[:])
	if t.Handler != "soun" || entry+36 > stsd.End() {
		return nil
	}
	t.ChannelCount, _ = readU16(r, entry+24)
	t.SampleSize, _ = readU16(r, entry+26)
	rate, _ := readU32(r, entry+32)
	t.SampleRate = rate >> 16
	return nil
}
