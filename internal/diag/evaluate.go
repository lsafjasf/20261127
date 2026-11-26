package diag

import (
	"fmt"

	"github.com/lsafjasf/20261127/internal/dovi"
	"github.com/lsafjasf/20261127/internal/mp4"
)

func evaluateSegment(seg *Segment, devices []DeviceCapabilities) {
	seg.FullDolbyVision = evaluateFullDV(seg)
	base := evaluateBase(seg)
	seg.MinimumAvailability = evaluateMinimum(seg, base)
	for _, device := range devices {
		seg.Devices = append(seg.Devices, evaluateDevice(seg, base, device))
	}
}

func evaluateFullDV(seg *Segment) Availability {
	if seg.Container.Encrypted {
		return Availability{
			Status: "auth_required", Mode: "drm_authorization", Usable: false,
			Confidence: "certain", Reason: "DRM-protected track; authorized decryption is required and was not attempted",
		}
	}
	if seg.DVConfig == nil {
		return Availability{Status: "blocked", Mode: "none", Usable: false,
			Confidence: "certain", Reason: "no dvcC/dvvC Dolby Vision configuration"}
	}
	if !seg.DVConfig.BLPresent {
		return Availability{Status: "not_standalone", Mode: "dolby_vision", Usable: false,
			Confidence: "certain", Reason: "enhancement/RPU attachment track has no base layer"}
	}
	if issue := firstErrorIssue(seg, CategoryConflict); issue != nil {
		return Availability{Status: "blocked", Mode: "dolby_vision", Usable: false,
			Confidence: "certain", Reason: issue.Message}
	}
	if issue := firstContainerError(seg); issue != nil {
		return Availability{Status: "blocked", Mode: "dolby_vision", Usable: false,
			Confidence: "certain", Reason: issue.Message}
	}
	if seg.Codec != "hevc" {
		return Availability{Status: "conditional", Mode: "dolby_vision", Usable: false,
			Confidence: "conditional", Reason: "configuration is plausible, but RPU verification currently supports HEVC only"}
	}
	rpu := seg.RPU
	if rpu == nil {
		return Availability{Status: "blocked", Mode: "dolby_vision", Usable: false,
			Confidence: "certain", Reason: "no RPU NAL was found in the scanned video samples"}
	}
	if rpu.SamplesWithRPU == 0 {
		if !rpu.Complete {
			return Availability{Status: "conditional", Mode: "dolby_vision", Usable: false,
				Confidence: "conditional", Reason: "RPU scan did not complete, so RPU presence is unverified"}
		}
		return Availability{Status: "blocked", Mode: "dolby_vision", Usable: false,
			Confidence: "certain", Reason: "no RPU NAL was found in the video samples"}
	}
	if !rpu.ReferencesOK {
		return Availability{Status: "blocked", Mode: "dolby_vision", Usable: false,
			Confidence: "certain", Reason: "RPU reference chain is not self-contained"}
	}
	if !rpu.Complete {
		return Availability{Status: "conditional", Mode: "dolby_vision", Usable: false,
			Confidence: "conditional",
			Reason: fmt.Sprintf("configuration and scanned RPUs are consistent, but only %d of %d samples were checked",
				rpu.ScannedSamples, seg.SampleCount)}
	}
	return Availability{Status: "full_dv", Mode: "dolby_vision", Usable: true,
		Confidence: "certain", Reason: "container, HEVC configuration, and RPU reference chain are consistent"}
}

func evaluateBase(seg *Segment) Availability {
	if seg.Container.Encrypted {
		return Availability{Status: "auth_required", Mode: "drm_authorization", Usable: false,
			Confidence: "certain", Reason: "protected base layer cannot be verified without authorized decryption"}
	}
	if seg.DVConfig != nil && !seg.DVConfig.BLPresent {
		return Availability{Status: "not_standalone", Mode: "none", Usable: false,
			Confidence: "certain", Reason: "attachment track contains no base layer"}
	}
	if seg.Codec == "hevc" && seg.HEVC == nil {
		return Availability{Status: "unusable", Mode: "none", Usable: false,
			Confidence: "certain", Reason: "HEVC decoder configuration is missing"}
	}
	if seg.SampleCount == 0 {
		return Availability{Status: "unusable", Mode: "none", Usable: false,
			Confidence: "certain", Reason: "sample table contains no video samples"}
	}
	if seg.DVConfig == nil {
		if !isStandardBaseEntry(effectiveEntry(seg)) {
			return Availability{Status: "unusable", Mode: "none", Usable: false,
				Confidence: "conditional", Reason: "no Dolby Vision configuration and no recognized standard base entry"}
		}
		return Availability{Status: "base_layer", Mode: inferBaseMode(seg), Usable: true,
			Confidence: "certain", Reason: "standard base-layer video without Dolby Vision signaling"}
	}
	if seg.DVConfig.CompatibilityID == 0 || dovi.CompatibilityFormat(seg.DVConfig.CompatibilityID) == "reserved" {
		return Availability{Status: "dv_only", Mode: "dolby_vision", Usable: false,
			Confidence: "certain", Reason: "base layer is not standards-compatible (CCID 0); a Dolby Vision decoder is required"}
	}
	mode := baseModeForCCID(seg.DVConfig.CompatibilityID)
	if isStandardBaseEntry(effectiveEntry(seg)) {
		return Availability{Status: "base_layer", Mode: mode, Usable: true,
			Confidence: "certain", Reason: "cross-compatible base layer can be decoded without Dolby Vision processing"}
	}
	if mp4.IsDVEntry(effectiveEntry(seg)) {
		return Availability{Status: "base_layer", Mode: mode, Usable: true,
			Confidence: "conditional",
			Reason:     "a " + mode + " base layer exists, but the player must accept the Dolby Vision sample entry or the file must be remuxed"}
	}
	return Availability{Status: "unusable", Mode: "none", Usable: false,
		Confidence: "certain", Reason: "sample entry cannot be mapped to a usable base layer"}
}

func evaluateMinimum(seg *Segment, base Availability) Availability {
	if seg.Container.Encrypted {
		return Availability{Status: "auth_required", Mode: "drm_authorization", Usable: false,
			Confidence: "certain", Reason: "DRM authorization is required before any layer can be verified"}
	}
	if base.Usable {
		return base
	}
	full := seg.FullDolbyVision
	if full.Usable {
		return Availability{Status: "dv_only", Mode: "dolby_vision", Usable: true,
			Confidence: full.Confidence, Reason: "no usable compatible base; Dolby Vision decoding is the minimum path"}
	}
	if full.Status == "conditional" {
		return Availability{Status: "dv_only", Mode: "dolby_vision", Usable: false,
			Confidence: "conditional", Reason: "full Dolby Vision may work, but the available evidence is incomplete"}
	}
	if full.Status == "not_standalone" {
		return full
	}
	reason := full.Reason
	if reason == "" {
		reason = "no base-layer or Dolby Vision path can be established"
	}
	return Availability{Status: "unusable", Mode: "none", Usable: false,
		Confidence: full.Confidence, Reason: reason}
}

func evaluateDevice(seg *Segment, base Availability, dev DeviceCapabilities) DeviceResult {
	result := DeviceResult{Device: dev.Name}
	if seg.Container.Encrypted {
		result.Status = "auth_required"
		result.Mode = "drm_authorization"
		result.Reason = "protected media requires the authorized DRM path; no decryption or bypass is attempted"
		return result
	}

	capIssues := dvCapabilityIssues(seg, dev)
	if len(capIssues) == 0 && seg.FullDolbyVision.Usable {
		result.Status = "full_dv"
		result.Mode = "dolby_vision"
		result.Playable = true
		result.Reason = "device advertises the required Dolby Vision profile, level, layer, and metadata handling"
		return result
	}

	if base.Usable && baseFallbackAllowed(seg, dev) {
		result.Status = "base_layer"
		result.Mode = base.Mode
		result.Playable = true
		if len(capIssues) > 0 {
			result.Reason = "full Dolby Vision is unavailable; compatible " + base.Mode + " base layer can play"
		} else {
			result.Reason = "compatible " + base.Mode + " base layer can play while full DV verification is incomplete"
		}
		result.Issues = capIssues
		return result
	}

	if seg.FullDolbyVision.Status == "conditional" && len(capIssues) == 0 {
		result.Status = "inconclusive"
		result.Mode = "dolby_vision"
		result.Reason = seg.FullDolbyVision.Reason
		return result
	}

	result.Status = "black"
	result.Mode = "none"
	result.Issues = capIssues
	switch {
	case base.Status == "dv_only":
		result.Reason = "device lacks a usable Dolby Vision path and the bitstream has no standards-compatible base layer"
	case base.Status == "not_standalone":
		result.Reason = "attachment track has no base layer and cannot be presented alone"
	case len(capIssues) > 0:
		result.Reason = capIssues[0].Message
	default:
		result.Reason = seg.FullDolbyVision.Reason
	}
	return result
}

func dvCapabilityIssues(seg *Segment, dev DeviceCapabilities) []Issue {
	var issues []Issue
	dv := seg.DVConfig
	if !dev.DolbyVision {
		return append(issues, Issue{Category: CategoryCapability, Code: "dv_unsupported",
			Severity: SeverityWarn, Message: "device does not advertise Dolby Vision decoding"})
	}
	if dv == nil {
		return append(issues, Issue{Category: CategoryCapability, Code: "dv_config_absent",
			Severity: SeverityError, Message: "stream has no Dolby Vision configuration"})
	}
	if !containsInt(dev.Profiles, dv.Profile) {
		issues = append(issues, Issue{Category: CategoryCapability, Code: "dv_profile_unsupported",
			Severity: SeverityWarn, Message: fmt.Sprintf("device does not support Dolby Vision profile %d", dv.Profile)})
	}
	if dev.MaxLevel > 0 && dv.Level > dev.MaxLevel {
		issues = append(issues, Issue{Category: CategoryCapability, Code: "dv_level_unsupported",
			Severity: SeverityWarn, Message: fmt.Sprintf("stream is DV level %d; device supports up to %d", dv.Level, dev.MaxLevel)})
	}
	if !containsInt(dev.CompatibilityIDs, dv.CompatibilityID) {
		issues = append(issues, Issue{Category: CategoryCapability, Code: "dv_ccid_unsupported",
			Severity: SeverityWarn, Message: fmt.Sprintf("device does not advertise compatibility ID %d", dv.CompatibilityID)})
	}
	if dv.ELPresent && !dev.SupportsEnhancementLayer {
		issues = append(issues, Issue{Category: CategoryCapability, Code: "dv_el_unsupported",
			Severity: SeverityWarn, Message: "device does not support the enhancement layer"})
	}
	if dv.CompressedMetadata && !dev.SupportsCompressedMetadata {
		issues = append(issues, Issue{Category: CategoryCapability, Code: "dv_compression_unsupported",
			Severity: SeverityWarn, Message: "device does not support the stream's compressed RPU metadata"})
	}
	if seg.Codec == "hevc" && dev.MaxHEVCLevelIDC > 0 && seg.HEVC != nil &&
		seg.HEVC.LevelIDC > dev.MaxHEVCLevelIDC {
		issues = append(issues, Issue{Category: CategoryCapability, Code: "hevc_level_unsupported",
			Severity: SeverityWarn, Message: fmt.Sprintf("HEVC level_idc %d exceeds device maximum %d",
				seg.HEVC.LevelIDC, dev.MaxHEVCLevelIDC)})
	}
	if len(dev.SampleEntries) > 0 && !containsString(dev.SampleEntries, effectiveEntry(seg)) {
		issues = append(issues, Issue{Category: CategoryCapability, Code: "dv_entry_unsupported",
			Severity: SeverityWarn, Message: fmt.Sprintf("device does not list sample entry %q", effectiveEntry(seg))})
	}
	return issues
}

func baseFallbackAllowed(seg *Segment, dev DeviceCapabilities) bool {
	entry := effectiveEntry(seg)
	if len(dev.SampleEntries) > 0 && !containsString(dev.SampleEntries, entry) &&
		!(mp4.IsDVEntry(entry) && dev.DVEntryBaseFallback) {
		return false
	}
	if mp4.IsDVEntry(entry) && !dev.DVEntryBaseFallback {
		return false
	}
	base := evaluateBase(seg)
	if len(dev.BaseFormats) == 0 || !containsString(dev.BaseFormats, base.Mode) {
		return false
	}
	if seg.Codec == "hevc" {
		if !dev.HEVCMain10 {
			return false
		}
		if dev.MaxHEVCLevelIDC > 0 && seg.HEVC != nil && seg.HEVC.LevelIDC > dev.MaxHEVCLevelIDC {
			return false
		}
	}
	return true
}

func firstErrorIssue(seg *Segment, category string) *Issue {
	for i := range seg.Issues {
		if seg.Issues[i].Category == category && seg.Issues[i].Severity == SeverityError {
			return &seg.Issues[i]
		}
	}
	return nil
}

func firstContainerError(seg *Segment) *Issue {
	return firstErrorIssue(seg, CategoryContainer)
}

func isStandardBaseEntry(entry string) bool {
	switch entry {
	case "hvc1", "hev1", "avc1", "avc3", "av01":
		return true
	default:
		return false
	}
}

func baseModeForCCID(id int) string {
	switch id {
	case 1, 6:
		return "hdr10"
	case 3, 4:
		return "hlg"
	case 2, 5:
		return "sdr"
	default:
		return "unknown"
	}
}

func inferBaseMode(seg *Segment) string {
	if seg.Colour == nil {
		return "unknown"
	}
	switch seg.Colour.Transfer {
	case 16:
		return "hdr10"
	case 18:
		return "hlg"
	default:
		return "sdr"
	}
}
