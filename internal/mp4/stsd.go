package mp4

import (
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/lsafjasf/20261127/internal/dovi"
	"github.com/lsafjasf/20261127/internal/hevc"
)

func parseStsd(payload []byte) ([]*SampleEntry, error) {
	if len(payload) < 8 {
		return nil, fmt.Errorf("stsd is too short")
	}
	count := int(binary.BigEndian.Uint32(payload[4:8]))
	pos := 8
	out := make([]*SampleEntry, 0, count)
	for i := 0; i < count; i++ {
		if pos+8 > len(payload) {
			return out, fmt.Errorf("truncated sample entry %d", i)
		}
		size := int(binary.BigEndian.Uint32(payload[pos : pos+4]))
		typ := string(payload[pos+4 : pos+8])
		if size < 8 || pos+size > len(payload) {
			return out, fmt.Errorf("invalid sample entry %q size %d", typ, size)
		}
		entryPayload := payload[pos+8 : pos+size]
		out = append(out, parseSampleEntry(i, typ, entryPayload))
		pos += size
	}
	return out, nil
}

func parseSampleEntry(index int, typ string, payload []byte) *SampleEntry {
	entry := &SampleEntry{
		Index:      index,
		StoredType: typ,
		Type:       typ,
		Codec:      codecForEntry(typ),
	}
	visual := isVisualEntry(typ)
	if visual {
		if len(payload) < 78 {
			entry.Warnings = append(entry.Warnings,
				fmt.Sprintf("visual sample entry %q is only %d bytes", typ, len(payload)))
			return entry
		}
		entry.Width = int(binary.BigEndian.Uint16(payload[24:26]))
		entry.Height = int(binary.BigEndian.Uint16(payload[26:28]))
	}
	childStart := 0
	if visual {
		childStart = 78
	}
	if childStart >= len(payload) {
		return entry
	}
	boxes, err := ParseChildren(payload[childStart:])
	if err != nil {
		entry.Warnings = append(entry.Warnings, "sample entry children: "+err.Error())
		return entry
	}
	for _, b := range boxes {
		switch b.Type {
		case "hvcC":
			if cfg, err := hevc.ParseDecoderConfig(b.Payload); err != nil {
				entry.Warnings = append(entry.Warnings, "hvcC: "+err.Error())
			} else {
				entry.HVCC = cfg
			}
		case "hvcE":
			if cfg, err := hevc.ParseDecoderConfig(b.Payload); err != nil {
				entry.Warnings = append(entry.Warnings, "hvcE: "+err.Error())
			} else {
				entry.HVCE = cfg
			}
		case "dvcC", "dvvC", "dvwC":
			cfg, err := dovi.ParseConfig(b.Payload)
			if err != nil {
				entry.Warnings = append(entry.Warnings, b.Type+": "+err.Error())
			} else {
				if entry.DV != nil {
					entry.Warnings = append(entry.Warnings,
						"multiple Dolby Vision configuration boxes")
				}
				entry.DV = cfg
				entry.DVBox = b.Type
				if b.Type == "dvwC" {
					entry.Warnings = append(entry.Warnings,
						"dvwC is reserved and is not interpreted")
				}
			}
		case "colr":
			entry.Colour = parseColour(b.Payload)
		case "sinf":
			entry.Encrypted = true
			parseSinf(entry, b.Payload)
		}
	}
	if entry.Encrypted {
		if entry.OriginalFormat != "" {
			entry.Type = entry.OriginalFormat
			entry.Codec = codecForEntry(entry.OriginalFormat)
		}
	}
	return entry
}

func parseSinf(entry *SampleEntry, payload []byte) {
	boxes, err := ParseChildren(payload)
	if err != nil {
		entry.Warnings = append(entry.Warnings, "sinf: "+err.Error())
		return
	}
	for _, b := range boxes {
		switch b.Type {
		case "frma":
			if len(b.Payload) >= 4 {
				entry.OriginalFormat = string(b.Payload[:4])
			}
		case "schm":
			if len(b.Payload) >= 8 {
				entry.EncryptionType = string(b.Payload[4:8])
			}
		}
	}
}

func parseColour(payload []byte) *Colour {
	if len(payload) < 4 {
		return nil
	}
	c := &Colour{Type: string(payload[:4])}
	switch c.Type {
	case "nclx":
		if len(payload) < 10 {
			return c
		}
		c.Primaries = int(binary.BigEndian.Uint16(payload[4:6]))
		c.Transfer = int(binary.BigEndian.Uint16(payload[6:8]))
		c.Matrix = int(binary.BigEndian.Uint16(payload[8:10]))
		if len(payload) >= 11 {
			c.FullRange = payload[10]&0x80 != 0
		}
	case "nclc":
		if len(payload) < 10 {
			return c
		}
		c.Primaries = int(binary.BigEndian.Uint16(payload[4:6]))
		c.Transfer = int(binary.BigEndian.Uint16(payload[6:8]))
		c.Matrix = int(binary.BigEndian.Uint16(payload[8:10]))
	}
	return c
}

func isVisualEntry(typ string) bool {
	switch strings.ToLower(typ) {
	case "hvc1", "hev1", "dvh1", "dvhe", "avc1", "avc3", "dva1", "dvav",
		"av01", "dav1", "encv", "mhv1", "mhvc", "hvc2", "hev2", "hvc3", "hev3":
		return true
	default:
		return false
	}
}

func codecForEntry(typ string) string {
	switch typ {
	case "hvc1", "hev1", "dvh1", "dvhe", "hvc2", "hev2", "hvc3", "hev3":
		return "hevc"
	case "avc1", "avc3", "dva1", "dvav":
		return "avc"
	case "av01", "dav1":
		return "av1"
	case "encv":
		return "encrypted-video"
	default:
		return "unknown"
	}
}

func IsDVEntry(typ string) bool {
	return typ == "dvh1" || typ == "dvhe" || typ == "dva1" || typ == "dvav" || typ == "dav1"
}
