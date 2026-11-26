package webm

import "alphacheck/ebml"

func parseTrack(entry *ebml.Element) TrackReport {
	track := TrackReport{
		TrackNumber: entry.Find(ebml.IDTrackNumber).SafeUint(),
		TrackType:   entry.Find(ebml.IDTrackType).SafeUint(),
		CodecID:     entry.Find(ebml.IDCodecID).SafeString(),
		AlphaMode:   -1,
	}
	if el := entry.Find(ebml.IDDefaultDuration); el != nil {
		track.DefaultDurationNS = el.Int()
	}
	track.CodecName = codecName(track.CodecID)

	if video := entry.Find(ebml.IDVideo); video != nil {
		if alpha := video.Find(ebml.IDAlphaMode); alpha != nil {
			track.AlphaModePresent = true
			track.AlphaMode = int(alpha.Int())
		}
		track.PixelWidth = video.Find(ebml.IDPixelWidth).SafeUint()
		track.PixelHeight = video.Find(ebml.IDPixelHeight).SafeUint()
		track.DisplayWidth = video.Find(ebml.IDDisplayWidth).SafeUint()
		track.DisplayHeight = video.Find(ebml.IDDisplayHeight).SafeUint()
		track.DisplayUnit = video.Find(ebml.IDDisplayUnit).SafeUint()
		track.Crop = Crop{
			Left:   video.Find(ebml.IDCropLeft).SafeUint(),
			Right:  video.Find(ebml.IDCropRight).SafeUint(),
			Top:    video.Find(ebml.IDCropTop).SafeUint(),
			Bottom: video.Find(ebml.IDCropBottom).SafeUint(),
		}
		track.VisibleWidth = track.PixelWidth
		track.VisibleHeight = track.PixelHeight
		if track.PixelWidth > track.Crop.Left+track.Crop.Right {
			track.VisibleWidth = track.PixelWidth - track.Crop.Left - track.Crop.Right
		} else {
			track.VisibleWidth = 0
		}
		if track.PixelHeight > track.Crop.Top+track.Crop.Bottom {
			track.VisibleHeight = track.PixelHeight - track.Crop.Top - track.Crop.Bottom
		} else {
			track.VisibleHeight = 0
		}
	}
	return track
}

func codecName(id string) string {
	switch id {
	case "V_VP8":
		return "VP8"
	case "V_VP9":
		return "VP9"
	case "V_AV1":
		return "AV1"
	case "V_MPEG4/ISO/AVC":
		return "H.264"
	case "V_MPEGH/ISO/HEVC":
		return "HEVC"
	default:
		return id
	}
}
