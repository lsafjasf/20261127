package diag

import (
	"fmt"
	"math"

	"github.com/lsafjasf/20261127/internal/dovi"
	"github.com/lsafjasf/20261127/internal/mp4"
)

func validateSegment(seg *Segment) {
	if seg.SampleCount == 0 {
		addIssue(seg, CategoryContainer, "no_video_samples", SeverityError,
			"sample table contains no samples for this video sample description")
	}
	if seg.Container.MajorBrand == "" {
		addIssue(seg, CategoryContainer, "missing_ftyp", SeverityWarn,
			"ftyp box is absent; Dolby Vision file brands cannot be checked")
	}
	if seg.Container.Fragmented {
		addIssue(seg, CategoryContainer, "fragmented_mp4", SeverityWarn,
			"fragmented MP4 is present; only moov sample tables are analyzed")
	}
	if seg.Container.Encrypted {
		addIssue(seg, CategoryDRM, "protected_track", SeverityInfo,
			"DRM-protected track: analysis stops at configuration/signaling and does not decrypt or bypass protection")
	}

	validateHEVC(seg)
	validateDV(seg)
	validateRPU(seg)
	validateEnhancement(seg)
}

func validateHEVC(seg *Segment) {
	if seg.Codec != "hevc" {
		return
	}
	if seg.HEVC == nil {
		addIssue(seg, CategoryContainer, "missing_hvcc", SeverityError,
			"HEVC sample entry has no parsable hvcC configuration")
		return
	}
	h := seg.HEVC
	if seg.DVConfig != nil && seg.DVConfig.Profile != 20 {
		if h.ProfileIDC != 2 {
			addIssue(seg, CategoryConflict, "hevc_profile_conflict", SeverityError,
				fmt.Sprintf("Dolby Vision HEVC requires Main10 (profile_idc 2), hvcC says %d (%s)",
					h.ProfileIDC, hevcProfileName(h.ProfileIDC)))
		}
		if h.BitDepthLuma != 10 || h.BitDepthChroma != 10 {
			addIssue(seg, CategoryConflict, "hevc_bit_depth_conflict", SeverityError,
				fmt.Sprintf("Dolby Vision HEVC expects 10-bit, hvcC says %d/%d-bit luma/chroma",
					h.BitDepthLuma, h.BitDepthChroma))
		}
		if h.ChromaFormat != 1 {
			addIssue(seg, CategoryConflict, "hevc_chroma_conflict", SeverityError,
				fmt.Sprintf("Dolby Vision HEVC expects 4:2:0, hvcC chroma_format is %d", h.ChromaFormat))
		}
	}
	if h.SPS == 0 && seg.SampleEntry != "hev1" {
		addIssue(seg, CategoryContainer, "missing_sps", SeverityError,
			"hvcC contains no SPS; hvc1 tracks require parameter sets in the configuration record")
	}
	if h.SPSProfileIDC != 0 {
		if h.SPSProfileIDC != h.ProfileIDC {
			addIssue(seg, CategoryConflict, "hevc_sps_profile_conflict", SeverityError,
				fmt.Sprintf("hvcC profile_idc is %d but SPS profile_idc is %d",
					h.ProfileIDC, h.SPSProfileIDC))
		}
		if h.SPSLevelIDC != 0 && h.SPSLevelIDC != h.LevelIDC {
			addIssue(seg, CategoryConflict, "hevc_level_conflict", SeverityError,
				fmt.Sprintf("hvcC level_idc is %d but SPS level_idc is %d",
					h.LevelIDC, h.SPSLevelIDC))
		}
	}
	if h.VPS == 0 && seg.SampleEntry != "hev1" {
		addIssue(seg, CategoryContainer, "missing_vps", SeverityWarn,
			"hvcC contains no VPS; in-band VPS may be required")
	}
	if h.PPS == 0 && seg.SampleEntry != "hev1" {
		addIssue(seg, CategoryContainer, "missing_pps", SeverityError,
			"hvcC contains no PPS; hvc1 tracks require parameter sets in the configuration record")
	}
}

func validateDV(seg *Segment) {
	dv := seg.DVConfig
	if dv == nil {
		if mp4.IsDVEntry(effectiveEntry(seg)) || (seg.RPU != nil && seg.RPU.SamplesWithRPU > 0) {
			addIssue(seg, CategoryContainer, "missing_dv_config", SeverityError,
				"Dolby Vision sample entry or RPU is present without dvcC/dvvC configuration")
		} else {
			addIssue(seg, CategoryEvidence, "no_dv_signaling", SeverityInfo,
				"no Dolby Vision configuration or RPU was found; this segment can only be assessed as a base-layer video")
		}
		return
	}

	if !knownProfile(dv.Profile) {
		addIssue(seg, CategoryConflict, "unknown_dv_profile", SeverityError,
			fmt.Sprintf("dv_profile %d is not a recognized profile", dv.Profile))
	}
	if _, ok := dovi.Limits(dv.Level); !ok {
		addIssue(seg, CategoryConflict, "invalid_dv_level", SeverityError,
			fmt.Sprintf("dv_level %d is outside the assigned level range 1..13", dv.Level))
	}
	expectedMajor := 1
	if dv.Profile == 20 {
		expectedMajor = 3
	}
	if dv.VersionMajor != expectedMajor || dv.VersionMinor != 0 {
		addIssue(seg, CategoryConflict, "dv_version_conflict", SeverityError,
			fmt.Sprintf("configuration version is %d.%d; profile %d expects %d.0",
				dv.VersionMajor, dv.VersionMinor, dv.Profile, expectedMajor))
	}
	expectedBox := "dvcC"
	if dv.Profile >= 8 && dv.Profile <= 10 {
		expectedBox = "dvvC"
	}
	if dv.Box != "" && dv.Box != "dvwC" && dv.Box != expectedBox {
		addIssue(seg, CategoryConflict, "dv_box_conflict", SeverityWarn,
			fmt.Sprintf("profile %d is carried in %s; specification expects %s",
				dv.Profile, dv.Box, expectedBox))
	}
	if !dv.RPUPresent {
		addIssue(seg, CategoryContainer, "rpu_not_declared", SeverityWarn,
			"rpu_present_flag is 0; full Dolby Vision processing cannot be expected")
	}
	if !dv.BLPresent {
		addIssue(seg, CategoryConflict, "attachment_only_track", SeverityWarn,
			"bl_present_flag is 0; this is an enhancement/RPU attachment track, not a standalone picture track")
	}
	if dv.MetadataCompression < 0 || dv.MetadataCompression > 3 {
		addIssue(seg, CategoryConflict, "invalid_metadata_compression", SeverityError,
			fmt.Sprintf("invalid dv_md_compression value %d", dv.MetadataCompression))
	} else if dv.MetadataCompression == 2 {
		addIssue(seg, CategoryConflict, "reserved_metadata_compression", SeverityError,
			"dv_md_compression=2 is reserved")
	} else if dv.MetadataCompression != 0 && dv.Profile <= 7 {
		addIssue(seg, CategoryConflict, "compression_not_allowed", SeverityError,
			"metadata compression is not permitted for profiles 4, 5, or 7")
	} else if dv.MetadataCompression != 0 && dv.Profile == 8 {
		addIssue(seg, CategoryEvidence, "compression_device_risk", SeverityWarn,
			"profile 8 metadata compression is not supported by many devices")
	}

	validateProfileFlags(seg, dv)
	validateLevel(seg, dv)
	validateCompatibility(seg, dv)
	validateEntrySignaling(seg, dv)
}

func validateProfileFlags(seg *Segment, dv *DVConfig) {
	switch dv.Profile {
	case 4:
		if dv.CompatibilityID != 2 {
			addIssue(seg, CategoryConflict, "profile4_ccid_conflict", SeverityWarn,
				fmt.Sprintf("profile 4 normally uses compatibility ID 2, found %d", dv.CompatibilityID))
		}
		if !dv.ELPresent {
			addIssue(seg, CategoryConflict, "missing_el_declaration", SeverityError,
				"profile 4 declares a dual-layer stream but el_present_flag is 0")
		}
	case 5:
		if dv.ELPresent {
			addIssue(seg, CategoryConflict, "profile5_el_conflict", SeverityError,
				"profile 5 is single-layer but el_present_flag is 1")
		}
		if dv.CompatibilityID != 0 {
			addIssue(seg, CategoryConflict, "profile5_ccid_conflict", SeverityError,
				fmt.Sprintf("profile 5 has no standards-compatible base but compatibility ID is %d", dv.CompatibilityID))
		}
	case 7:
		if dv.CompatibilityID != 6 {
			addIssue(seg, CategoryConflict, "profile7_ccid_conflict", SeverityWarn,
				fmt.Sprintf("profile 7 normally uses UHD Blu-ray compatibility ID 6, found %d", dv.CompatibilityID))
		}
		if !dv.ELPresent {
			addIssue(seg, CategoryConflict, "missing_el_declaration", SeverityError,
				"profile 7 declares a dual-layer stream but el_present_flag is 0; only the base layer is declared")
		}
	case 8:
		if dv.ELPresent {
			addIssue(seg, CategoryConflict, "profile8_el_conflict", SeverityError,
				"profile 8 is single-layer but el_present_flag is 1")
		}
		if dv.CompatibilityID != 1 && dv.CompatibilityID != 2 &&
			dv.CompatibilityID != 4 && dv.CompatibilityID != 5 {
			addIssue(seg, CategoryConflict, "profile8_ccid_conflict", SeverityError,
				fmt.Sprintf("profile 8 requires a compatible base (CCID 1, 2, 4, or 5), found %d", dv.CompatibilityID))
		}
	case 9:
		if dv.ELPresent {
			addIssue(seg, CategoryConflict, "profile9_el_conflict", SeverityError,
				"profile 9 is single-layer but el_present_flag is 1")
		}
		if dv.CompatibilityID != 2 {
			addIssue(seg, CategoryConflict, "profile9_ccid_conflict", SeverityError,
				fmt.Sprintf("profile 9 requires SDR compatibility ID 2, found %d", dv.CompatibilityID))
		}
	case 10:
		if dv.ELPresent {
			addIssue(seg, CategoryConflict, "profile10_el_conflict", SeverityError,
				"profile 10 is single-layer but el_present_flag is 1")
		}
		if dv.CompatibilityID != 0 && dv.CompatibilityID != 1 && dv.CompatibilityID != 4 {
			addIssue(seg, CategoryConflict, "profile10_ccid_conflict", SeverityError,
				fmt.Sprintf("profile 10 uses CCID 0, 1, or 4, found %d", dv.CompatibilityID))
		}
	case 20:
		if dv.CompatibilityID != 0 && dv.CompatibilityID != 1 && dv.CompatibilityID != 4 {
			addIssue(seg, CategoryConflict, "profile20_ccid_conflict", SeverityError,
				fmt.Sprintf("profile 20 uses CCID 0, 1, or 4, found %d", dv.CompatibilityID))
		}
	}
}

func validateLevel(seg *Segment, dv *DVConfig) {
	limit, ok := dovi.Limits(dv.Level)
	if !ok || seg.Width <= 0 || seg.Height <= 0 || seg.FPS <= 0 {
		return
	}
	if seg.Width > limit.MaxWidth {
		addIssue(seg, CategoryConflict, "dv_level_too_low", SeverityError,
			fmt.Sprintf("width %d exceeds level %d maximum %d", seg.Width, dv.Level, limit.MaxWidth))
		return
	}
	pps := math.Ceil(float64(seg.Width) * float64(seg.Height) * seg.FPS)
	if pps > float64(limit.MaxPixelsPerSecond)*1.005 {
		addIssue(seg, CategoryConflict, "dv_level_too_low", SeverityError,
			fmt.Sprintf("%.0f pixels/s at %dx%d %.3f fps exceeds level %d limit %d",
				pps, seg.Width, seg.Height, seg.FPS, dv.Level, limit.MaxPixelsPerSecond))
	}
}

func validateCompatibility(seg *Segment, dv *DVConfig) {
	wantTransfer := 0
	wantPrimaries := 0
	wantMatrix := 0
	switch dv.CompatibilityID {
	case 1, 6:
		wantTransfer = 16 // SMPTE ST 2084 (PQ)
		wantPrimaries = 9
		wantMatrix = 9
	case 3, 4:
		wantTransfer = 18 // ARIB STD-B67 (HLG)
		wantMatrix = 9
		wantPrimaries = 9
		if dv.CompatibilityID == 3 {
			wantPrimaries = 1
			wantMatrix = 1
		}
	case 2:
		wantTransfer = 1 // BT.709 SDR
		wantPrimaries = 1
		wantMatrix = 1
	case 5:
		wantTransfer = 1
		wantPrimaries = 9
		wantMatrix = 9
	}
	if wantTransfer != 0 {
		if seg.Colour == nil {
			addIssue(seg, CategoryContainer, "missing_colr", SeverityWarn,
				"compatible base has no colr box; SPS VUI is not available to this diagnostic")
			return
		}
		if seg.Colour.Type != "nclx" && seg.Colour.Type != "nclc" {
			addIssue(seg, CategoryEvidence, "colour_type_not_interpretable", SeverityWarn,
				fmt.Sprintf("colr type %q does not expose transfer/primaries/matrix fields", seg.Colour.Type))
			return
		}
		if seg.Colour.Transfer == 0 || seg.Colour.Transfer == 2 ||
			seg.Colour.Primaries == 2 || seg.Colour.Matrix == 2 {
			addIssue(seg, CategoryConflict, "color_unspecified_for_ccid", SeverityWarn,
				fmt.Sprintf("CCID %d needs a defined HDR/SDR base signal but colr contains unspecified color fields", dv.CompatibilityID))
			return
		}
		if seg.Colour.Transfer != wantTransfer {
			addIssue(seg, CategoryConflict, "color_ccid_conflict", SeverityError,
				fmt.Sprintf("CCID %d expects transfer_characteristics %d, colr says %d",
					dv.CompatibilityID, wantTransfer, seg.Colour.Transfer))
		}
		if wantPrimaries != 0 && seg.Colour.Primaries != wantPrimaries {
			addIssue(seg, CategoryConflict, "primaries_ccid_conflict", SeverityError,
				fmt.Sprintf("CCID %d expects color primaries %d, colr says %d",
					dv.CompatibilityID, wantPrimaries, seg.Colour.Primaries))
		}
		if wantMatrix != 0 && seg.Colour.Matrix != wantMatrix {
			addIssue(seg, CategoryConflict, "matrix_ccid_conflict", SeverityError,
				fmt.Sprintf("CCID %d expects matrix coefficients %d, colr says %d",
					dv.CompatibilityID, wantMatrix, seg.Colour.Matrix))
		}
		if seg.Colour.FullRange && dv.CompatibilityID != 5 {
			addIssue(seg, CategoryConflict, "full_range_base_conflict", SeverityWarn,
				fmt.Sprintf("CCID %d normally uses a limited-range base signal, colr declares full range", dv.CompatibilityID))
		}
	}
}

func validateEntrySignaling(seg *Segment, dv *DVConfig) {
	entry := effectiveEntry(seg)
	dvEntry := mp4.IsDVEntry(entry)
	if dv.Profile == 5 && !dvEntry && entry != "encv" {
		addIssue(seg, CategoryConflict, "profile5_entry_conflict", SeverityError,
			fmt.Sprintf("profile 5 has an IPTPQc2 base but uses standard sample entry %q; non-DV decoders will display it as a broken base layer", entry))
	}
	if (dv.Profile == 4 || dv.Profile == 7) && !dvEntry {
		addIssue(seg, CategoryConflict, "dual_layer_entry_conflict", SeverityWarn,
			fmt.Sprintf("dual-layer profile %d is carried in %q instead of dvhe/dvh1", dv.Profile, entry))
	}
	if dv.Profile == 8 {
		switch dv.CompatibilityID {
		case 1, 2, 4, 5:
			if dvEntry {
				addIssue(seg, CategoryConflict, "compatible_entry_conflict", SeverityWarn,
					fmt.Sprintf("profile 8.%d is cross-compatible but uses %q instead of hev1/hvc1", dv.CompatibilityID, entry))
			}
		case 0:
			if !dvEntry {
				addIssue(seg, CategoryConflict, "profile8_entry_conflict", SeverityError,
					fmt.Sprintf("profile 8 has no compatible base but uses standard sample entry %q", entry))
			}
		}
	}
}

func validateRPU(seg *Segment) {
	dv := seg.DVConfig
	stats := seg.RPU
	if stats == nil {
		return
	}
	if dv == nil {
		if stats.SamplesWithRPU > 0 {
			addIssue(seg, CategoryContainer, "undeclared_rpu", SeverityError,
				"RPU NAL units are present but the container does not declare Dolby Vision")
		}
		return
	}
	if !dv.RPUPresent && stats.SamplesWithRPU > 0 {
		addIssue(seg, CategoryConflict, "rpu_flag_conflict", SeverityError,
			"rpu_present_flag is 0 but RPU NAL units are present")
	}
	if dv.RPUPresent && stats.SamplesWithRPU == 0 {
		sev := SeverityWarn
		if stats.Complete {
			sev = SeverityError
		}
		addIssue(seg, CategoryConflict, "rpu_missing", sev,
			"rpu_present_flag is 1 but no RPU NAL was found in the scanned samples")
	}
	for _, profile := range stats.Profiles {
		if profile == 0 {
			addIssue(seg, CategoryConflict, "rpu_profile_unknown", SeverityError,
				"RPU header does not derive a recognized Dolby Vision profile")
			continue
		}
		if profile != dv.Profile {
			addIssue(seg, CategoryConflict, "rpu_profile_conflict", SeverityError,
				fmt.Sprintf("container declares profile %d but RPU header derives profile %d",
					dv.Profile, profile))
		}
	}
	if stats.CRCErrors > 0 {
		addIssue(seg, CategoryConflict, "rpu_crc_errors", SeverityError,
			fmt.Sprintf("%d RPU(s) failed CRC32 validation", stats.CRCErrors))
	}
	if stats.ParseErrors > 0 {
		addIssue(seg, CategoryConflict, "rpu_parse_errors", SeverityError,
			fmt.Sprintf("%d sample(s) or RPU(s) could not be parsed", stats.ParseErrors))
	}
	if !stats.ReferencesOK {
		addIssue(seg, CategoryConflict, "rpu_references_invalid", SeverityError,
			"RPU previous-mapping reference chain is not self-contained in this segment")
	}
}

func validateEnhancement(seg *Segment) {
	dv := seg.DVConfig
	enh := seg.Enhancement
	if enh.HVCETrackPresent {
		if dv == nil || dv.Profile != 7 {
			addIssue(seg, CategoryConflict, "hvce_profile_conflict", SeverityError,
				"hvcE enhancement-layer configuration is only defined for profile 7")
		}
		if dv != nil && !dv.ELPresent {
			addIssue(seg, CategoryConflict, "hvce_declaration_conflict", SeverityError,
				"hvcE is present but el_present_flag is 0")
		}
		if enh.HVCECodec == nil {
			addIssue(seg, CategoryConflict, "hvce_invalid", SeverityError,
				"hvcE exists but its HEVC decoder configuration is invalid")
		} else if enh.HVCECodec.ProfileIDC != 2 ||
			enh.HVCECodec.BitDepthLuma != 10 || enh.HVCECodec.BitDepthChroma != 10 {
			addIssue(seg, CategoryConflict, "hvce_codec_conflict", SeverityError,
				"enhancement layer is not HEVC Main10 10-bit")
		}
	}
	if dv != nil && dv.ELPresent && dv.Profile == 7 && !enh.HVCETrackPresent {
		addIssue(seg, CategoryEvidence, "hvce_absent", SeverityInfo,
			"el_present_flag is 1 but optional hvcE is absent; EL may still be multiplexed in-band")
	}
}

func knownProfile(profile int) bool {
	switch profile {
	case 4, 5, 7, 8, 9, 10, 20:
		return true
	default:
		return false
	}
}

func effectiveEntry(seg *Segment) string {
	if seg.Container.OriginalFormat != "" {
		return seg.Container.OriginalFormat
	}
	return seg.SampleEntry
}

func hevcProfileName(idc int) string {
	switch idc {
	case 1:
		return "Main"
	case 2:
		return "Main10"
	default:
		return fmt.Sprintf("profile_idc=%d", idc)
	}
}
