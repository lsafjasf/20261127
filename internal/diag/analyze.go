package diag

import (
	"os"

	"github.com/lsafjasf/20261127/internal/dovi"
	"github.com/lsafjasf/20261127/internal/hevc"
	"github.com/lsafjasf/20261127/internal/mp4"
)

type Options struct {
	ScanSamples  int
	MaxScanBytes int64
}

func AnalyzeFile(path string, devices []DeviceCapabilities, opts Options) FileReport {
	report := FileReport{File: path}
	f, err := os.Open(path)
	if err != nil {
		report.Error = err.Error()
		return report
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		report.Error = err.Error()
		return report
	}
	parsed, err := mp4.Parse(f, st.Size())
	if err != nil {
		report.Error = err.Error()
		return report
	}
	if opts.MaxScanBytes <= 0 {
		opts.MaxScanBytes = 64 << 20
	}
	for _, track := range parsed.Tracks {
		if track.Handler != "vide" {
			continue
		}
		for _, entry := range track.Entries {
			if !codecSupported(entry) {
				continue
			}
			seg := buildSegment(path, parsed, track, entry)
			scanSegment(f, track, entry, &seg, opts)
			validateSegment(&seg)
			evaluateSegment(&seg, devices)
			report.Segments = append(report.Segments, seg)
		}
	}
	return report
}

func buildSegment(path string, file *mp4.File, track *mp4.Track, entry *mp4.SampleEntry) Segment {
	seg := Segment{
		File:              path,
		TrackID:           track.ID,
		SampleDescription: entry.Index + 1,
		SampleEntry:       entry.StoredType,
		Codec:             entry.Codec,
		Width:             entry.Width,
		Height:            entry.Height,
		Container: ContainerInfo{
			Format:           "isobmff",
			MajorBrand:       file.MajorBrand,
			CompatibleBrands: append([]string(nil), file.CompatibleBrands...),
			Fragmented:       file.Fragmented,
			Encrypted:        entry.Encrypted,
			EncryptionScheme: entry.EncryptionType,
			OriginalFormat:   entry.OriginalFormat,
			PSSHPresent:      file.PSSHPresent,
		},
	}
	if seg.Width == 0 {
		seg.Width = track.Width
	}
	if seg.Height == 0 {
		seg.Height = track.Height
	}
	var totalDuration uint64
	for _, sample := range track.Samples {
		if sample.Desc != entry.Index {
			continue
		}
		seg.SampleCount++
		totalDuration += uint64(sample.Duration)
	}
	if track.Timescale > 0 && totalDuration > 0 {
		seg.FPS = float64(seg.SampleCount) * float64(track.Timescale) / float64(totalDuration)
	}
	if entry.HVCC != nil {
		seg.HEVC = hevcInfo(entry.HVCC)
	}
	if entry.DV != nil {
		seg.DVConfig = dvInfo(entry.DV, entry.DVBox)
	}
	if entry.Colour != nil {
		seg.Colour = &ColourInfo{
			Type:      entry.Colour.Type,
			Primaries: entry.Colour.Primaries,
			Transfer:  entry.Colour.Transfer,
			Matrix:    entry.Colour.Matrix,
			FullRange: entry.Colour.FullRange,
		}
	}
	seg.Enhancement = EnhancementInfo{
		Declared:         entry.DV != nil && entry.DV.ELPresent,
		HVCETrackPresent: entry.HVCE != nil,
		HVCECodec:        hevcInfo(entry.HVCE),
		TrackReferences:  cloneReferences(track.References),
	}
	for _, warning := range track.Warnings {
		addIssue(&seg, CategoryContainer, "track_warning", SeverityWarn, warning)
	}
	for _, warning := range entry.Warnings {
		addIssue(&seg, CategoryContainer, "sample_entry_warning", SeverityWarn, warning)
	}
	return seg
}

func hevcInfo(cfg *hevc.DecoderConfig) *HEVCConfig {
	if cfg == nil {
		return nil
	}
	out := &HEVCConfig{
		ProfileSpace:              cfg.ProfileSpace,
		TierFlag:                  cfg.TierFlag,
		ProfileIDC:                cfg.ProfileIDC,
		LevelIDC:                  cfg.LevelIDC,
		ProfileCompatibilityFlags: cfg.ProfileCompatibilityFlags,
		ChromaFormat:              cfg.ChromaFormat,
		BitDepthLuma:              cfg.BitDepthLuma,
		BitDepthChroma:            cfg.BitDepthChroma,
		LengthSize:                cfg.LengthSize,
		VPS:                       cfg.VPS,
		SPS:                       cfg.SPS,
		PPS:                       cfg.PPS,
	}
	if cfg.SPSPTL != nil {
		out.SPSProfileIDC = cfg.SPSPTL.ProfileIDC
		out.SPSLevelIDC = cfg.SPSPTL.LevelIDC
		out.SPSCompatibilityFlags = cfg.SPSPTL.ProfileCompatibilityFlags
	}
	return out
}

func dvInfo(cfg *dovi.Config, box string) *DVConfig {
	if cfg == nil {
		return nil
	}
	return &DVConfig{
		Box:                 box,
		VersionMajor:        cfg.VersionMajor,
		VersionMinor:        cfg.VersionMinor,
		Profile:             cfg.Profile,
		Level:               cfg.Level,
		RPUPresent:          cfg.RPUPresent,
		ELPresent:           cfg.ELPresent,
		BLPresent:           cfg.BLPresent,
		CompatibilityID:     cfg.CompatibilityID,
		CompatibilityFormat: dovi.CompatibilityFormat(cfg.CompatibilityID),
		MetadataCompression: cfg.MetadataCompression,
		CompressedMetadata:  cfg.CompressedMetadata(),
		FeatureFlags:        append([]bool(nil), cfg.FeatureFlags...),
	}
}

func addIssue(seg *Segment, category, code, severity, message string) {
	for _, issue := range seg.Issues {
		if issue.Category == category && issue.Code == code {
			return
		}
	}
	seg.Issues = append(seg.Issues, Issue{
		Category: category,
		Code:     code,
		Severity: severity,
		Message:  message,
	})
}

func codecSupported(e *mp4.SampleEntry) bool {
	return e.Codec == "hevc" || e.Codec == "avc" || e.Codec == "av1"
}

func cloneReferences(in map[string][]uint32) map[string][]uint32 {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string][]uint32, len(in))
	for k, v := range in {
		out[k] = append([]uint32(nil), v...)
	}
	return out
}
