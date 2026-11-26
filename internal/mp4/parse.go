package mp4

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"

	"github.com/lsafjasf/20261127/internal/dovi"
	"github.com/lsafjasf/20261127/internal/hevc"
)

type File struct {
	MajorBrand       string
	CompatibleBrands []string
	PSSHPresent      bool
	Fragmented       bool
	Tracks           []*Track
}

type Track struct {
	ID         uint32
	Handler    string
	Timescale  uint32
	Duration   uint64
	Width      int
	Height     int
	References map[string][]uint32
	Entries    []*SampleEntry
	Samples    []Sample
	Warnings   []string
}

type Sample struct {
	Index    int
	Desc     int
	Offset   int64
	Size     uint32
	Duration uint32
}

type Colour struct {
	Type      string
	Primaries int
	Transfer  int
	Matrix    int
	FullRange bool
}

type SampleEntry struct {
	Index          int
	StoredType     string
	Type           string
	Codec          string
	Width          int
	Height         int
	DV             *dovi.Config
	DVBox          string
	HVCC           *hevc.DecoderConfig
	HVCE           *hevc.DecoderConfig
	Colour         *Colour
	Encrypted      bool
	EncryptionType string
	OriginalFormat string
	Warnings       []string
}

func ParseFile(path string) (*File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	return Parse(f, st.Size())
}

func Parse(r io.ReaderAt, fileSize int64) (*File, error) {
	top, err := readTopLevel(r, fileSize)
	if err != nil {
		return nil, err
	}
	out := &File{}
	var moov *Box
	for i := range top {
		switch top[i].Type {
		case "ftyp":
			payload, err := readPayload(r, top[i])
			if err != nil {
				return nil, err
			}
			out.parseFtyp(payload)
		case "moov":
			moov = &top[i]
		case "moof":
			out.Fragmented = true
		case "pssh":
			out.PSSHPresent = true
		}
	}
	if moov == nil {
		return nil, fmt.Errorf("moov box not found")
	}
	payload, err := readPayload(r, *moov)
	if err != nil {
		return nil, err
	}
	boxes, err := ParseChildren(payload)
	if err != nil {
		return nil, fmt.Errorf("parse moov: %w", err)
	}
	if Find(boxes, "mvex") != nil {
		out.Fragmented = true
	}
	for _, trak := range FindAll(boxes, "trak") {
		track, err := parseTrack(trak.Payload)
		if err != nil {
			return nil, err
		}
		out.Tracks = append(out.Tracks, track)
	}
	for _, pssh := range FindAll(boxes, "pssh") {
		_ = pssh
		out.PSSHPresent = true
	}
	return out, nil
}

func (f *File) parseFtyp(payload []byte) {
	if len(payload) < 8 {
		return
	}
	f.MajorBrand = string(payload[:4])
	for pos := 8; pos+4 <= len(payload); pos += 4 {
		f.CompatibleBrands = append(f.CompatibleBrands, string(payload[pos:pos+4]))
	}
}

func parseTrack(payload []byte) (*Track, error) {
	boxes, err := ParseChildren(payload)
	if err != nil {
		return nil, fmt.Errorf("parse trak: %w", err)
	}
	track := &Track{References: make(map[string][]uint32)}
	if tkhd := Find(boxes, "tkhd"); tkhd != nil {
		parseTkhd(track, tkhd.Payload)
	}
	if tref := Find(boxes, "tref"); tref != nil {
		parseTref(track, tref.Payload)
	}
	mdia := Find(boxes, "mdia")
	if mdia == nil {
		return track, nil
	}
	mdiaBoxes, err := ParseChildren(mdia.Payload)
	if err != nil {
		return nil, fmt.Errorf("parse mdia: %w", err)
	}
	if mdhd := Find(mdiaBoxes, "mdhd"); mdhd != nil {
		parseMdhd(track, mdhd.Payload)
	}
	if hdlr := Find(mdiaBoxes, "hdlr"); hdlr != nil && len(hdlr.Payload) >= 12 {
		track.Handler = string(hdlr.Payload[8:12])
	}
	minf := Find(mdiaBoxes, "minf")
	if minf == nil {
		return track, nil
	}
	minfBoxes, err := ParseChildren(minf.Payload)
	if err != nil {
		return nil, fmt.Errorf("parse minf: %w", err)
	}
	stbl := Find(minfBoxes, "stbl")
	if stbl == nil {
		return track, nil
	}
	if err := parseSampleTable(track, stbl.Payload); err != nil {
		return nil, err
	}
	return track, nil
}

func parseTkhd(track *Track, payload []byte) {
	if len(payload) < 4 {
		return
	}
	version := payload[0]
	var pos int
	if version == 1 {
		pos = 4 + 8 + 8
	} else {
		pos = 4 + 4 + 4
	}
	if pos+4 <= len(payload) {
		track.ID = binary.BigEndian.Uint32(payload[pos : pos+4])
	}
	if len(payload) >= 8 {
		track.Width = int(binary.BigEndian.Uint32(payload[len(payload)-8:]) >> 16)
		track.Height = int(binary.BigEndian.Uint32(payload[len(payload)-4:]) >> 16)
	}
}

func parseMdhd(track *Track, payload []byte) {
	if len(payload) < 24 {
		return
	}
	if payload[0] == 1 {
		if len(payload) >= 32 {
			track.Timescale = binary.BigEndian.Uint32(payload[20:24])
			track.Duration = binary.BigEndian.Uint64(payload[24:32])
		}
		return
	}
	track.Timescale = binary.BigEndian.Uint32(payload[12:16])
	track.Duration = uint64(binary.BigEndian.Uint32(payload[16:20]))
}

func parseTref(track *Track, payload []byte) {
	boxes, err := ParseChildren(payload)
	if err != nil {
		return
	}
	for _, b := range boxes {
		for pos := 0; pos+4 <= len(b.Payload); pos += 4 {
			track.References[b.Type] = append(track.References[b.Type],
				binary.BigEndian.Uint32(b.Payload[pos:pos+4]))
		}
	}
}
