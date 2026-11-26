package manifest

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
)

type assetStatusChecks map[string][]Check

func (a *assessor) buildSharedAssets(ctx context.Context, report *Report, hdrTracks, sdrTracks []TrackSummary) {
	allAssetChecks := make(assetStatusChecks)
	groups := []struct {
		kind   string
		assets []SharedAsset
	}{
		{kind: "audio", assets: a.cfg.Shared.Audio},
		{kind: "subtitle", assets: a.cfg.Shared.Subtitles},
	}
	for _, group := range groups {
		for _, configured := range group.assets {
			asset := a.inspectAsset(ctx, group.kind, configured)
			hdrRef, hdrCount := findEmbeddedTrack(hdrTracks, group.kind, asset.StreamSHA256)
			sdrRef, sdrCount := findEmbeddedTrack(sdrTracks, group.kind, asset.StreamSHA256)
			asset.HDR = &hdrRef
			asset.SDR = &sdrRef

			hdrChecks := versionAssetChecks("HDR", "hdr", group.kind, configured, hdrRef, hdrCount)
			sdrChecks := versionAssetChecks("SDR", "sdr", group.kind, configured, sdrRef, sdrCount)
			report.Versions.HDR.Checks = append(report.Versions.HDR.Checks, hdrChecks...)
			report.Versions.SDR.Checks = append(report.Versions.SDR.Checks, sdrChecks...)
			allAssetChecks[configured.ID] = append(allAssetChecks[configured.ID], asset.Checks...)
			allAssetChecks[configured.ID] = append(allAssetChecks[configured.ID], hdrChecks...)
			allAssetChecks[configured.ID] = append(allAssetChecks[configured.ID], sdrChecks...)
			report.Shared.Assets = append(report.Shared.Assets, asset)
		}
	}
	for i := range report.Shared.Assets {
		id := report.Shared.Assets[i].ID
		report.Shared.Assets[i].Status = statusFromChecks(allAssetChecks[id])
	}
}

func (a *assessor) inspectAsset(ctx context.Context, kind string, configured SharedAsset) AssetReport {
	asset := AssetReport{
		ID:              configured.ID,
		Kind:            kind,
		Path:            configured.Path,
		Status:          StatusIncomplete,
		Language:        configured.Language,
		Title:           configured.Title,
		DeclaredDefault: configured.Default,
		Checks:          []Check{},
	}

	fileCheck := Check{
		ID:       "shared." + configured.ID + ".file",
		Category: "shared_asset",
		Required: true,
		Expected: "readable regular non-empty file",
	}
	info, statErr := os.Stat(configured.Path)
	fileCheck.Pass = statErr == nil && info.Mode().IsRegular() && info.Size() > 0
	if statErr != nil {
		fileCheck.Actual = statErr.Error()
		fileCheck.Detail = "shared asset file is missing or unreadable"
	} else {
		asset.SizeBytes = info.Size()
		fileCheck.Actual = fmt.Sprintf("%d bytes", info.Size())
		if !info.Mode().IsRegular() {
			fileCheck.Detail = "shared asset is not a regular file"
		} else if info.Size() == 0 {
			fileCheck.Detail = "shared asset file is empty"
		}
	}
	asset.Checks = append(asset.Checks, fileCheck)

	hash, hashErr := hashFile(configured.Path)
	hashCheck := Check{
		ID:       "shared." + configured.ID + ".sha256",
		Category: "shared_asset",
		Required: true,
		Expected: "SHA-256 file hash",
		Actual:   hash,
		Pass:     hashErr == nil && hash != "",
	}
	if hashErr != nil {
		hashCheck.Actual = "unavailable"
		hashCheck.Detail = "could not hash shared asset: " + hashErr.Error()
	} else {
		asset.SHA256 = hash
	}
	asset.Checks = append(asset.Checks, hashCheck)

	if !fileCheck.Pass {
		asset.Checks = append(asset.Checks,
			failedAssetProbeCheck(configured.ID, "file is unavailable"),
			failedAssetStreamCheck(configured.ID, "file is unavailable"),
			failedAssetStreamHashCheck(configured.ID, "file is unavailable"),
		)
		return asset
	}

	probe, probeErr := a.prober.Probe(ctx, configured.Path)
	probeCheck := Check{
		ID:       "shared." + configured.ID + ".probe",
		Category: "shared_asset",
		Required: true,
		Expected: "ffprobe succeeds",
		Pass:     probeErr == nil,
	}
	if probeErr != nil {
		probeCheck.Actual = "probe failed"
		probeCheck.Detail = probeErr.Error()
	} else {
		probeCheck.Actual = "ok"
	}
	asset.Checks = append(asset.Checks, probeCheck)

	var streams []ProbeStream
	if probeErr == nil {
		streams = probe.StreamsOfType(kind)
	}
	streamCheck := Check{
		ID:       "shared." + configured.ID + ".stream_count",
		Category: "shared_asset",
		Required: true,
		Expected: 1,
		Actual:   len(streams),
		Pass:     len(streams) == 1,
	}
	if !streamCheck.Pass {
		streamCheck.Detail = fmt.Sprintf("shared %s asset must contain exactly one %s stream", kind, kind)
	}
	asset.Checks = append(asset.Checks, streamCheck)

	if len(streams) == 1 {
		asset.Codec = streams[0].CodecName
	}
	streamHash := ""
	streamHashCheck := Check{
		ID:       "shared." + configured.ID + ".stream_sha256",
		Category: "shared_asset",
		Required: true,
		Expected: "SHA-256 stream hash",
	}
	if len(streams) == 1 {
		value, err := a.prober.StreamSHA256(ctx, configured.Path, streams[0].Index)
		streamHash = value
		streamHashCheck.Actual = value
		streamHashCheck.Pass = err == nil && value != ""
		if err != nil {
			streamHashCheck.Detail = "could not hash shared stream: " + err.Error()
		}
	} else {
		streamHashCheck.Actual = "unavailable"
		streamHashCheck.Detail = "a single matching stream is required before hashing"
	}
	asset.StreamSHA256 = streamHash
	asset.Checks = append(asset.Checks, streamHashCheck)
	return asset
}

func failedAssetProbeCheck(id, detail string) Check {
	return Check{
		ID:       "shared." + id + ".probe",
		Category: "shared_asset",
		Required: true,
		Expected: "ffprobe succeeds",
		Actual:   "unavailable",
		Detail:   detail,
	}
}

func failedAssetStreamCheck(id, detail string) Check {
	return Check{
		ID:       "shared." + id + ".stream_count",
		Category: "shared_asset",
		Required: true,
		Expected: 1,
		Actual:   0,
		Detail:   detail,
	}
}

func failedAssetStreamHashCheck(id, detail string) Check {
	return Check{
		ID:       "shared." + id + ".stream_sha256",
		Category: "shared_asset",
		Required: true,
		Expected: "SHA-256 stream hash",
		Actual:   "unavailable",
		Detail:   detail,
	}
}

func findEmbeddedTrack(tracks []TrackSummary, kind, hash string) (EmbeddedRef, int) {
	ref := EmbeddedRef{Found: false}
	if hash == "" {
		return ref, 0
	}
	count := 0
	for _, track := range tracks {
		if !strings.EqualFold(track.Type, kind) || track.StreamSHA256 != hash {
			continue
		}
		count++
		if !ref.Found {
			ref = EmbeddedRef{
				Found:    true,
				Index:    track.Index,
				Default:  track.Default,
				Language: track.Language,
				Title:    track.Title,
			}
		}
	}
	return ref, count
}

func versionAssetChecks(role, roleKey, kind string, configured SharedAsset, ref EmbeddedRef, matchCount int) []Check {
	prefix := "shared." + configured.ID + "." + roleKey + "."
	checks := []Check{
		{
			ID:       prefix + "present",
			Category: "shared_asset",
			Required: true,
			Expected: fmt.Sprintf("shared %s asset embedded in %s", kind, role),
			Actual:   refIndexText(ref),
			Pass:     ref.Found,
			Detail:   missingAssetDetail(role, ref.Found),
		},
		{
			ID:       prefix + "unique",
			Category: "shared_asset",
			Required: true,
			Expected: 1,
			Actual:   matchCount,
			Pass:     matchCount == 1,
			Detail:   duplicateAssetDetail(role, matchCount),
		},
	}
	if configured.Language != "" {
		checks = append(checks, assetMetadataCheck(
			prefix+"language",
			"language",
			configured.Language,
			ref.Language,
			ref.Found,
		))
	}
	if configured.Title != "" {
		checks = append(checks, assetMetadataCheck(
			prefix+"title",
			"title",
			configured.Title,
			ref.Title,
			ref.Found,
		))
	}
	if configured.Default != nil {
		check := Check{
			ID:       prefix + "default",
			Category: "shared_asset",
			Required: true,
			Expected: *configured.Default,
			Actual:   ref.Default,
			Pass:     ref.Found && ref.Default == *configured.Default,
		}
		if !check.Pass {
			check.Detail = "default flag for shared " + kind + " track does not match the manifest"
		}
		checks = append(checks, check)
	}
	return checks
}

func refIndexText(ref EmbeddedRef) any {
	if !ref.Found {
		return "missing"
	}
	return ref.Index
}

func missingAssetDetail(role string, found bool) string {
	if found {
		return ""
	}
	return "shared asset stream is absent from " + role
}

func duplicateAssetDetail(role string, count int) string {
	if count == 1 {
		return ""
	}
	return fmt.Sprintf("%s contains %d copies of the shared asset stream; exactly one is required", role, count)
}

func assetMetadataCheck(id, label, expected, actual string, found bool) Check {
	check := Check{
		ID:       id,
		Category: "shared_asset",
		Required: true,
		Expected: expected,
		Actual:   nonEmptyValue(actual),
	}
	if !found {
		check.Actual = "missing"
		check.Detail = "cannot verify " + label + " because the shared stream is absent"
		return check
	}
	if label == "language" {
		check.Pass = normalizeLanguage(expected) == normalizeLanguage(actual)
	} else {
		check.Pass = expected == actual
	}
	if !check.Pass {
		check.Detail = fmt.Sprintf("%s mismatch: expected %q, got %q", label, expected, nonEmptyValue(actual))
	}
	return check
}

func normalizeLanguage(value string) string {
	base := strings.ToLower(strings.TrimSpace(value))
	if index := strings.IndexAny(base, "-_"); index >= 0 {
		base = base[:index]
	}
	aliases := map[string]string{
		"zh": "zho", "chi": "zho", "zho": "zho",
		"en": "eng", "eng": "eng",
		"ja": "jpn", "jpn": "jpn",
		"ko": "kor", "kor": "kor",
		"fr": "fra", "fre": "fra", "fra": "fra",
		"de": "deu", "ger": "deu", "deu": "deu",
		"es": "spa", "spa": "spa",
		"it": "ita", "ita": "ita",
		"pt": "por", "por": "por",
		"ru": "rus", "rus": "rus",
	}
	if canonical, ok := aliases[base]; ok {
		return canonical
	}
	return base
}

func (a *assessor) buildSharedSets(report *Report, hdrTracks, sdrTracks []TrackSummary) {
	report.Shared.Audio = compareTrackSets("audio", hdrTracks, sdrTracks)
	report.Shared.Subtitles = compareTrackSets("subtitle", hdrTracks, sdrTracks)
}

func compareTrackSets(kind string, hdrTracks, sdrTracks []TrackSummary) EmbeddedSetReport {
	hdrHashes := sortedTrackHashes(hdrTracks, kind)
	sdrHashes := sortedTrackHashes(sdrTracks, kind)
	hdrSignatures := sortedTrackSignatures(hdrTracks, kind)
	sdrSignatures := sortedTrackSignatures(sdrTracks, kind)
	report := EmbeddedSetReport{
		Status:        StatusIncomplete,
		HDRCount:      len(hdrHashes),
		SDRCount:      len(sdrHashes),
		HDRHashes:     hdrHashes,
		SDRHashes:     sdrHashes,
		HDRSignatures: hdrSignatures,
		SDRSignatures: sdrSignatures,
		Checks:        []Check{},
	}
	report.Checks = append(report.Checks,
		Check{
			ID:       "shared." + kind + "_set.hdr_present",
			Category: "shared_tracks",
			Required: true,
			Expected: "at least one " + kind + " stream",
			Actual:   len(hdrHashes),
			Pass:     len(hdrHashes) > 0,
			Detail:   missingStreamDetail(kind, len(hdrHashes)),
		},
		Check{
			ID:       "shared." + kind + "_set.sdr_present",
			Category: "shared_tracks",
			Required: true,
			Expected: "at least one " + kind + " stream",
			Actual:   len(sdrHashes),
			Pass:     len(sdrHashes) > 0,
			Detail:   missingStreamDetail(kind, len(sdrHashes)),
		},
	)
	hashesAvailable := len(hdrHashes) > 0 && len(sdrHashes) > 0
	for _, hash := range append(append([]string(nil), hdrHashes...), sdrHashes...) {
		if hash == "" {
			hashesAvailable = false
			break
		}
	}
	report.Checks = append(report.Checks, Check{
		ID:       "shared." + kind + "_set.hashes_available",
		Category: "shared_tracks",
		Required: true,
		Expected: "all " + kind + " streams have SHA-256 hashes",
		Actual:   hashesAvailable,
		Pass:     hashesAvailable,
		Detail:   unavailableHashDetail(kind, hashesAvailable),
	})
	identical := hashesAvailable && equalStringSlices(hdrSignatures, sdrSignatures)
	report.Checks = append(report.Checks, Check{
		ID:       "shared." + kind + "_set.identical",
		Category: "shared_tracks",
		Required: true,
		Expected: "identical " + kind + " streams and metadata in HDR and SDR",
		Actual: map[string]any{
			"hdr": hdrSignatures,
			"sdr": sdrSignatures,
		},
		Pass:   identical,
		Detail: differentTrackSetDetail(kind, identical),
	})
	report.Status = statusFromChecks(report.Checks)
	return report
}

func sortedTrackHashes(tracks []TrackSummary, kind string) []string {
	hashes := []string{}
	for _, track := range tracks {
		if strings.EqualFold(track.Type, kind) {
			hashes = append(hashes, track.StreamSHA256)
		}
	}
	sort.Strings(hashes)
	return hashes
}

func sortedTrackSignatures(tracks []TrackSummary, kind string) []string {
	signatures := []string{}
	for _, track := range tracks {
		if strings.EqualFold(track.Type, kind) {
			signatures = append(signatures, trackShareSignature(track, kind))
		}
	}
	sort.Strings(signatures)
	return signatures
}

func equalStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func unavailableHashDetail(kind string, available bool) string {
	if available {
		return ""
	}
	return "one or more " + kind + " streams could not be hashed"
}

func differentTrackSetDetail(kind string, identical bool) string {
	if identical {
		return ""
	}
	return kind + " stream sets differ between HDR and SDR"
}
