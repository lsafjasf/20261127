package webm

import (
	"encoding/binary"
	"errors"
	"fmt"

	"alphacheck/ebml"
)

var errUnsupportedLacing = errors.New("laced block payloads are not decoded")

func processCluster(cluster *ebml.Element, clusterTC int64, scaleNS uint64, tracks map[uint64]*TrackReport, counts map[uint64]int, rep *Report, opts Options) {
	for _, child := range cluster.Children {
		switch child.ID {
		case ebml.IDSimpleBlock:
			pb, payload, err := parseVideoBlock(child.Data)
			if err != nil && !errors.Is(err, errUnsupportedLacing) {
				addFinding(rep, blockFinding("error", "muxing_error", "bad_block", 0, 0, err.Error()))
				continue
			}
			pb.keyframe = pb.flags&0x80 != 0
			if err != nil {
				addFinding(rep, blockFinding("warning", "muxing_error", "lacing_unsupported", pb.track, 0, err.Error()))
				payload = nil
			}
			appendFrame(rep, tracks, counts, clusterTC, scaleNS, pb, payload, alphaBundle{}, opts)

		case ebml.IDBlockGroup:
			block := child.Find(ebml.IDBlock)
			if block == nil {
				addFinding(rep, Finding{Severity: "error", Layer: "container", Class: "muxing_error", Code: "block_group_without_block", Message: "BlockGroup has no Block"})
				continue
			}
			pb, payload, err := parseVideoBlock(block.Data)
			if err != nil && !errors.Is(err, errUnsupportedLacing) {
				addFinding(rep, blockFinding("error", "muxing_error", "bad_block", 0, 0, err.Error()))
				continue
			}
			pb.keyframe = child.Find(ebml.IDReferenceBlock) == nil
			bundle := readAlphaBundle(child.Find(ebml.IDBlockAdditions))
			if err != nil {
				addFinding(rep, blockFinding("warning", "muxing_error", "lacing_unsupported", pb.track, 0, err.Error()))
				payload = nil
			}
			if dur := child.Find(ebml.IDBlockDuration); dur != nil {
				pb.durationTicks = int64(dur.Int())
			}
			appendFrame(rep, tracks, counts, clusterTC, scaleNS, pb, payload, bundle, opts)
		}
	}
}

type parsedBlock struct {
	track         uint64
	relativeTC    int16
	flags         byte
	keyframe      bool
	durationTicks int64
}

type alphaBundle struct {
	present bool
	addID   uint64
	payload []byte
	others  []uint64
	empty   int
}

func parseVideoBlock(b []byte) (parsedBlock, []byte, error) {
	if len(b) < 4 {
		return parsedBlock{}, nil, fmt.Errorf("block is shorter than header")
	}
	width := vintWidth(b[0])
	if width < 1 || width > 8 || width+3 > len(b) {
		return parsedBlock{}, nil, fmt.Errorf("invalid block track VINT")
	}
	var track uint64
	for i := 0; i < width; i++ {
		mask := byte(0xff)
		if i == 0 {
			mask = 0xff >> uint(width)
		}
		track = track<<8 | uint64(b[i]&mask)
	}
	rel := int16(binary.BigEndian.Uint16(b[width : width+2]))
	flags := b[width+2]
	header := width + 3
	lacing := (flags >> 1) & 0x03
	if lacing != 0 {
		return parsedBlock{track: track, relativeTC: rel, flags: flags}, nil,
			fmt.Errorf("%w: mode %d", errUnsupportedLacing, lacing)
	}
	return parsedBlock{track: track, relativeTC: rel, flags: flags}, b[header:], nil
}

func vintWidth(first byte) int {
	for n := 1; n <= 8; n++ {
		if first&(0x80>>uint(n-1)) != 0 {
			return n
		}
	}
	return 0
}

func readAlphaBundle(additions *ebml.Element) alphaBundle {
	bundle := alphaBundle{}
	if additions == nil {
		return bundle
	}
	for _, more := range additions.FindAll(ebml.IDBlockMore) {
		addID := uint64(1)
		if el := more.Find(ebml.IDBlockAddID); el != nil {
			addID = el.Int()
		}
		additional := more.Find(ebml.IDBlockAdditional)
		if additional == nil {
			continue
		}
		if len(additional.Data) == 0 {
			bundle.empty++
		}
		if addID == 1 && !bundle.present {
			bundle.present = true
			bundle.addID = addID
			bundle.payload = additional.Data
		} else {
			bundle.others = append(bundle.others, addID)
		}
	}
	return bundle
}

func appendFrame(rep *Report, tracks map[uint64]*TrackReport, counts map[uint64]int, clusterTC int64, scaleNS uint64, pb parsedBlock, payload []byte, alpha alphaBundle, opts Options) {
	track := tracks[pb.track]
	if track != nil && track.TrackType != 0 && track.TrackType != 1 {
		return
	}
	counts[pb.track]++
	pts := (clusterTC + int64(pb.relativeTC)) * int64(scaleNS)
	durationNS := uint64(0)
	if pb.durationTicks > 0 {
		durationNS = uint64(pb.durationTicks) * scaleNS
	} else if track != nil && track.DefaultDurationNS != 0 {
		durationNS = track.DefaultDurationNS
	}

	frame := FrameReport{
		Index:        counts[pb.track],
		TrackNumber:  pb.track,
		PTSNS:        pts,
		OffsetMS:     float64(pts) / 1e6,
		DurationNS:   durationNS,
		Keyframe:     pb.keyframe,
		PayloadBytes: len(payload),
		AlphaPresent: alpha.present,
		AlphaAddID:   alpha.addID,
		AlphaBytes:   len(alpha.payload),
	}
	if track == nil {
		addFinding(rep, blockFinding("error", "muxing_error", "unknown_track_number", pb.track, frame.Index, "block references an absent TrackEntry"))
	}
	if alpha.present && track != nil {
		track.Alpha.FramesWithAdditional++
		track.Alpha.AlphaAddIDOne++
		track.Alpha.EmptyPayloads += alpha.empty
		track.Alpha.OtherAddIDs = appendUnique(track.Alpha.OtherAddIDs, alpha.others...)
	}
	if track != nil && (track.CodecName == "VP8" || track.CodecName == "VP9") && len(payload) > 0 {
		info, err := ParseCodecHeader(track.CodecName, payload)
		if err != nil {
			frame.HeaderError = err.Error()
			addFinding(rep, blockFinding("error", "encoding_error", "bad_codec_header", pb.track, frame.Index, err.Error()))
		} else if info.Width > 0 {
			frame.BitstreamWidth = info.Width
			frame.BitstreamHeight = info.Height
			if info.Keyframe {
				track.Bitstream = info
			}
		}
		if alpha.present && len(alpha.payload) > 0 {
			alphaInfo, err := ParseCodecHeader(track.CodecName, alpha.payload)
			if err != nil {
				addFinding(rep, blockFinding("error", "encoding_error", "bad_alpha_header", pb.track, frame.Index, "alpha BlockAdditional: "+err.Error()))
			} else {
				track.Alpha.ParsedHeaders++
				if info != nil && alphaInfo.Width > 0 && info.Width > 0 && (alphaInfo.Width != info.Width || alphaInfo.Height != info.Height) {
					addFinding(rep, blockFinding("error", "encoding_error", "alpha_size_mismatch", pb.track, frame.Index,
						fmt.Sprintf("color %dx%d, alpha %dx%d", info.Width, info.Height, alphaInfo.Width, alphaInfo.Height)))
				}
			}
		}
	}
	rep.Frames = append(rep.Frames, frame)
}

func appendUnique(in []uint64, values ...uint64) []uint64 {
	for _, v := range values {
		exists := false
		for _, old := range in {
			if old == v {
				exists = true
				break
			}
		}
		if !exists {
			in = append(in, v)
		}
	}
	return in
}

func blockFinding(severity, class, code string, track uint64, frame int, msg string) Finding {
	return Finding{Severity: severity, Layer: classLayer(class), Class: class, Code: code, Track: track, Frame: frame, Message: msg}
}

func classLayer(class string) string {
	if class == "encoding_error" {
		return "bitstream"
	}
	return "container"
}
