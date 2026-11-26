package webm

import (
	"errors"

	"alphacheck/ebml"
)

var errMissingSegment = errors.New("webm: Segment element not found")

// FramePayload contains one codec frame and its optional alpha frame.
type FramePayload struct {
	Index        int
	TrackNumber  uint64
	PTSNS        int64
	Keyframe     bool
	Payload      []byte
	AlphaPayload []byte
	AlphaAddID   uint64
}

// ExtractVideoFrames returns compressed frame payloads without decoding them.
// If trackNumber is 0, the primary video track is selected.
func ExtractVideoFrames(data []byte, trackNumber uint64) ([]FramePayload, error) {
	if trackNumber == 0 {
		rep, err := Inspect(data, Options{MaxFrameDetails: -1})
		if err != nil {
			return nil, err
		}
		trackNumber = primaryTrackNumber(rep)
	}
	elems, err := ebml.Parse(data)
	if err != nil {
		return nil, err
	}
	var segment *ebml.Element
	for _, el := range elems {
		if el.ID == ebml.IDSegment {
			segment = el
		}
	}
	if segment == nil {
		return nil, errMissingSegment
	}
	scale := uint64(1_000_000)
	if info := segment.Find(ebml.IDInfo); info != nil {
		if el := info.Find(ebml.IDTimecodeScale); el != nil && el.Int() != 0 {
			scale = el.Int()
		}
	}

	var out []FramePayload
	index := 0
	for _, cluster := range segment.FindAll(ebml.IDCluster) {
		clusterTC := int64(cluster.Find(ebml.IDTimecode).SafeUint())
		for _, child := range cluster.Children {
			var pb parsedBlock
			var payload []byte
			var alpha alphaBundle
			var keyframe bool
			switch child.ID {
			case ebml.IDSimpleBlock:
				pb, payload, err = parseVideoBlock(child.Data)
				if err != nil || pb.track != trackNumber {
					continue
				}
				keyframe = child.Data[3]&0x80 != 0
			case ebml.IDBlockGroup:
				block := child.Find(ebml.IDBlock)
				if block == nil {
					continue
				}
				pb, payload, err = parseVideoBlock(block.Data)
				if err != nil || pb.track != trackNumber {
					continue
				}
				keyframe = child.Find(ebml.IDReferenceBlock) == nil
				alpha = readAlphaBundle(child.Find(ebml.IDBlockAdditions))
			default:
				continue
			}
			index++
			out = append(out, FramePayload{
				Index:        index,
				TrackNumber:  pb.track,
				PTSNS:        (clusterTC + int64(pb.relativeTC)) * int64(scale),
				Keyframe:     keyframe,
				Payload:      payload,
				AlphaPayload: alpha.payload,
				AlphaAddID:   alpha.addID,
			})
		}
	}
	return out, nil
}
