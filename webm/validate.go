package webm

import "fmt"

func finalizeTracks(rep *Report) {
	for i := range rep.Tracks {
		track := &rep.Tracks[i]
		for _, frame := range rep.Frames {
			if frame.TrackNumber == track.TrackNumber {
				track.FrameCount++
			}
		}
		validateAlpha(rep, track)
		validateCrop(rep, track)
	}
}

func validateAlpha(rep *Report, track *TrackReport) {
	if track.TrackType != 0 && track.TrackType != 1 {
		return
	}
	switch {
	case track.AlphaModePresent && (track.AlphaMode < 0 || track.AlphaMode > 1):
		addFinding(rep, Finding{
			Severity: "error", Layer: "container", Class: "muxing_error", Code: "invalid_alpha_mode",
			Track: track.TrackNumber, Message: fmt.Sprintf("AlphaMode=%d is invalid; WebM expects 0 or 1", track.AlphaMode),
		})
	case track.AlphaMode == 1 && track.Alpha.FramesWithAdditional == 0:
		addFinding(rep, Finding{
			Severity: "error", Layer: "container", Class: "muxing_error", Code: "alpha_mode_without_payload",
			Track:   track.TrackNumber,
			Message: "AlphaMode=1 but no BlockMore/BlockAddID=1/BlockAdditional alpha payload was found",
		})
	case (!track.AlphaModePresent || track.AlphaMode == 0) && track.Alpha.FramesWithAdditional > 0:
		addFinding(rep, Finding{
			Severity: "error", Layer: "container", Class: "muxing_error", Code: "alpha_payload_without_mode",
			Track:   track.TrackNumber,
			Message: fmt.Sprintf("found %d alpha BlockAdditional payloads but AlphaMode is absent or 0", track.Alpha.FramesWithAdditional),
		})
	case track.AlphaMode == 1 && track.FrameCount > 0 && track.Alpha.FramesWithAdditional > 0 &&
		track.Alpha.FramesWithAdditional < track.FrameCount:
		missing := track.FrameCount - track.Alpha.FramesWithAdditional
		severity := "warning"
		if missing == track.FrameCount {
			severity = "error"
		}
		addFinding(rep, Finding{
			Severity: severity, Layer: "container", Class: "muxing_error", Code: "partial_alpha_payloads",
			Track:   track.TrackNumber,
			Message: fmt.Sprintf("%d/%d frames lack alpha BlockAdditional", missing, track.FrameCount),
		})
	}
	if track.Alpha.EmptyPayloads > 0 {
		addFinding(rep, Finding{
			Severity: "error", Layer: "container", Class: "muxing_error", Code: "empty_alpha_payload",
			Track: track.TrackNumber, Message: fmt.Sprintf("%d empty alpha BlockAdditional element(s)", track.Alpha.EmptyPayloads),
		})
	}
}

func validateCrop(rep *Report, track *TrackReport) {
	if track.TrackType == 2 {
		return
	}
	if track.PixelWidth == 0 || track.PixelHeight == 0 {
		addFinding(rep, Finding{
			Severity: "warning", Layer: "container", Class: "muxing_error", Code: "missing_pixel_dimensions",
			Track: track.TrackNumber, Message: "Video PixelWidth/PixelHeight are missing or zero",
		})
		return
	}
	if track.Crop.Left+track.Crop.Right >= track.PixelWidth {
		addFinding(rep, Finding{
			Severity: "error", Layer: "container", Class: "muxing_error", Code: "invalid_horizontal_crop",
			Track:   track.TrackNumber,
			Message: fmt.Sprintf("crop L+R=%d exceeds PixelWidth=%d", track.Crop.Left+track.Crop.Right, track.PixelWidth),
		})
	}
	if track.Crop.Top+track.Crop.Bottom >= track.PixelHeight {
		addFinding(rep, Finding{
			Severity: "error", Layer: "container", Class: "muxing_error", Code: "invalid_vertical_crop",
			Track:   track.TrackNumber,
			Message: fmt.Sprintf("crop T+B=%d exceeds PixelHeight=%d", track.Crop.Top+track.Crop.Bottom, track.PixelHeight),
		})
	}
	if track.Bitstream != nil && track.Bitstream.Width > 0 {
		if uint64(track.Bitstream.Width) != track.PixelWidth || uint64(track.Bitstream.Height) != track.PixelHeight {
			addFinding(rep, Finding{
				Severity: "error", Layer: "container", Class: "muxing_error", Code: "pixel_bitstream_size_mismatch",
				Track: track.TrackNumber,
				Message: fmt.Sprintf("container PixelWidth/Height=%dx%d but %s keyframe header is %dx%d",
					track.PixelWidth, track.PixelHeight, track.CodecName, track.Bitstream.Width, track.Bitstream.Height),
				Evidence: "Pixel* must describe the coded rectangle before PixelCrop*",
			})
		}
	}
	if track.VisibleWidth > 0 && track.VisibleHeight > 0 && (track.VisibleWidth%2 == 1 || track.VisibleHeight%2 == 1) &&
		(track.CodecName == "VP8" || track.CodecName == "VP9") {
		addFinding(rep, Finding{
			Severity: "warning", Layer: "container", Class: "muxing_error", Code: "odd_visible_dimensions",
			Track:   track.TrackNumber,
			Message: fmt.Sprintf("cropped visible size %dx%d is odd; 4:2:0 paths may require even crop/output dimensions", track.VisibleWidth, track.VisibleHeight),
		})
	}
}
