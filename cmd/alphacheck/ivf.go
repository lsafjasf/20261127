package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"

	"alphacheck/webm"
)

func runIVF(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: alphacheck ivf FILE OUT-BASE")
	}
	data, err := os.ReadFile(args[0])
	if err != nil {
		return err
	}
	report, err := webm.Inspect(data, webm.Options{MaxFrameDetails: 1})
	if err != nil {
		return err
	}
	track := primaryReportVideoTrack(report)
	frames, err := webm.ExtractVideoFrames(data, track.TrackNumber)
	if err != nil {
		return err
	}
	if len(frames) == 0 {
		return fmt.Errorf("no video frames extracted")
	}
	fourcc := "VP90"
	codec := "VP9"
	if track.CodecName == "VP8" {
		fourcc = "VP80"
		codec = "VP8"
	}
	width, height := int(track.PixelWidth), int(track.PixelHeight)
	if track.Bitstream != nil && track.Bitstream.Width > 0 {
		width, height = track.Bitstream.Width, track.Bitstream.Height
	}
	colorPath := args[1] + ".color.ivf"
	if err := writeIVF(colorPath, fourcc, width, height, frames, false); err != nil {
		return err
	}
	alphaFrames := 0
	for _, f := range frames {
		if len(f.AlphaPayload) > 0 {
			alphaFrames++
		}
	}
	if alphaFrames > 0 {
		alphaPath := args[1] + ".alpha.ivf"
		if err := writeIVF(alphaPath, fourcc, width, height, frames, true); err != nil {
			return err
		}
		fmt.Printf("wrote %s (%d %s frames) and %s (%d alpha frames)\n", colorPath, len(frames), codec, alphaPath, alphaFrames)
	} else {
		fmt.Printf("wrote %s (%d %s frames); no alpha payloads\n", colorPath, len(frames), codec)
	}
	return nil
}

func primaryReportVideoTrack(r *webm.Report) webm.TrackReport {
	for _, t := range r.Tracks {
		if t.TrackType == 1 {
			return t
		}
	}
	if len(r.Tracks) > 0 {
		return r.Tracks[0]
	}
	return webm.TrackReport{}
}

func writeIVF(path, fourcc string, width, height int, frames []webm.FramePayload, alpha bool) error {
	f, err := os.Create(filepath.Clean(path))
	if err != nil {
		return err
	}
	defer f.Close()

	header := make([]byte, 32)
	copy(header[0:4], "DKIF")
	binary.LittleEndian.PutUint16(header[4:6], 0)
	binary.LittleEndian.PutUint16(header[6:8], 32)
	copy(header[8:12], fourcc)
	binary.LittleEndian.PutUint16(header[12:14], uint16(width))
	binary.LittleEndian.PutUint16(header[14:16], uint16(height))
	binary.LittleEndian.PutUint32(header[16:20], 1)
	binary.LittleEndian.PutUint32(header[20:24], 1000)
	binary.LittleEndian.PutUint32(header[24:28], uint32(len(frames)))
	if _, err := f.Write(header); err != nil {
		return err
	}
	for _, frame := range frames {
		payload := frame.Payload
		if alpha {
			payload = frame.AlphaPayload
		}
		frameHeader := make([]byte, 12)
		binary.LittleEndian.PutUint32(frameHeader[0:4], uint32(len(payload)))
		binary.LittleEndian.PutUint64(frameHeader[4:12], uint64(frame.PTSNS/1_000_000))
		if _, err := f.Write(frameHeader); err != nil {
			return err
		}
		if _, err := f.Write(payload); err != nil {
			return err
		}
	}
	return f.Close()
}
