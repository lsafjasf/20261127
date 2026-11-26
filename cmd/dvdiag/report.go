package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/lsafjasf/20261127/internal/diag"
)

func writeText(report diag.Report) {
	writeTextTo(os.Stdout, report)
}

func writeTextTo(w io.Writer, report diag.Report) {
	for _, file := range report.Files {
		fmt.Fprintf(w, "FILE %s\n", file.File)
		if file.Error != "" {
			fmt.Fprintf(w, "  ERROR %s\n\n", file.Error)
			continue
		}
		if len(file.Segments) == 0 {
			fmt.Fprintln(w, "  no supported video segment found")
			fmt.Fprintln(w)
			continue
		}
		for _, seg := range file.Segments {
			writeSegment(w, seg)
		}
	}
}

func writeSegment(w io.Writer, seg diag.Segment) {
	fmt.Fprintf(w, "  SEGMENT track=%d description=%d entry=%s codec=%s %dx%d",
		seg.TrackID, seg.SampleDescription, seg.SampleEntry, seg.Codec, seg.Width, seg.Height)
	if seg.FPS > 0 {
		fmt.Fprintf(w, " %.3ffps", seg.FPS)
	}
	fmt.Fprintf(w, " samples=%d\n", seg.SampleCount)
	fmt.Fprintf(w, "    container: format=%s brand=%s encrypted=%t fragmented=%t\n",
		seg.Container.Format, valueOr(seg.Container.MajorBrand, "?"),
		seg.Container.Encrypted, seg.Container.Fragmented)
	if seg.DVConfig != nil {
		dv := seg.DVConfig
		fmt.Fprintf(w,
			"    dolby-vision: box=%s profile=%d level=%d rpu=%t el=%t bl=%t ccid=%d(%s) compression=%d\n",
			dv.Box, dv.Profile, dv.Level, dv.RPUPresent, dv.ELPresent, dv.BLPresent,
			dv.CompatibilityID, dv.CompatibilityFormat, dv.MetadataCompression)
	} else {
		fmt.Fprintln(w, "    dolby-vision: not declared")
	}
	if seg.HEVC != nil {
		h := seg.HEVC
		fmt.Fprintf(w, "    hevc: profile_idc=%d level_idc=%d bit_depth=%d/%d nal_length=%d vps/sps/pps=%d/%d/%d\n",
			h.ProfileIDC, h.LevelIDC, h.BitDepthLuma, h.BitDepthChroma, h.LengthSize,
			h.VPS, h.SPS, h.PPS)
		if h.SPSLevelIDC != 0 && h.SPSLevelIDC != h.LevelIDC {
			fmt.Fprintf(w, "    hevc-sps: profile_idc=%d level_idc=%d\n", h.SPSProfileIDC, h.SPSLevelIDC)
		}
	}
	fmt.Fprintf(w, "    enhancement: declared=%t hvcE=%t observed_el_nals=%d\n",
		seg.Enhancement.Declared, seg.Enhancement.HVCETrackPresent, seg.Enhancement.ObservedELNALs)
	if len(seg.Enhancement.TrackReferences) > 0 {
		fmt.Fprintf(w, "    track-references: %v\n", seg.Enhancement.TrackReferences)
	}
	if seg.RPU != nil {
		r := seg.RPU
		fmt.Fprintf(w, "    rpu: scanned=%d/%d rpu_samples=%d profiles=%v refs_ok=%t crc_errors=%d parse_errors=%d\n",
			r.ScannedSamples, seg.SampleCount, r.SamplesWithRPU, r.Profiles,
			r.ReferencesOK, r.CRCErrors, r.ParseErrors)
		if r.First != nil {
			fmt.Fprintf(w, "    first-rpu: sample=%d profile=%d format=%d use_prev=%t prev_id=%d mapping_id=%d\n",
				r.First.SampleIndex, r.First.Profile, r.First.RPUFormat,
				r.First.UsePreviousRPU, r.First.PreviousRPUID, r.First.MappingID)
		}
	}
	if len(seg.Issues) > 0 {
		fmt.Fprintln(w, "    findings:")
		for _, issue := range seg.Issues {
			fmt.Fprintf(w, "      [%s/%s] %s: %s\n",
				issue.Category, issue.Severity, issue.Code, issue.Message)
		}
	}
	writeAvailability(w, "full-dv", seg.FullDolbyVision)
	writeAvailability(w, "minimum", seg.MinimumAvailability)
	for _, device := range seg.Devices {
		fmt.Fprintf(w, "    device %s: %s/%s playable=%t -- %s\n",
			device.Device, device.Status, device.Mode, device.Playable, device.Reason)
		for _, issue := range device.Issues {
			fmt.Fprintf(w, "      [%s] %s: %s\n",
				issue.Category, issue.Code, issue.Message)
		}
	}
	fmt.Fprintln(w)
}

func writeAvailability(w io.Writer, label string, a diag.Availability) {
	fmt.Fprintf(w, "    %s: %s mode=%s usable=%t confidence=%s\n",
		label, a.Status, a.Mode, a.Usable, a.Confidence)
	if a.Reason != "" {
		fmt.Fprintf(w, "      %s\n", a.Reason)
	}
}

func valueOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
