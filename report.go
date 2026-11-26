package mp4seek

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
)

func ScanFile(path string) (*Report, error) {
	file, err := ParseFile(path)
	if err != nil {
		return nil, err
	}
	return file.Report(), nil
}

func (f *File) Report() *Report {
	report := &Report{Path: f.Path, Size: f.Size}
	for _, span := range f.MDATs {
		copy := span
		if span.Offset <= uint64(len(f.data)) && span.Size <= uint64(len(f.data))-span.Offset {
			sum := sha256.Sum256(f.data[span.Offset : span.Offset+span.Size])
			copy.SHA256 = hex.EncodeToString(sum[:])
		} else {
			report.Warnings = append(report.Warnings, "mdat range is outside the file")
		}
		report.MDATs = append(report.MDATs, copy)
	}
	for _, track := range f.Tracks {
		report.Tracks = append(report.Tracks, TrackReport{
			ID:           track.ID,
			Handler:      track.Handler,
			Codec:        track.Codec,
			Timescale:    track.Timescale,
			EditOffsetUS: scaleToMicros(track.EditOffset, track.Timescale),
			SampleCount:  len(track.Samples),
			IndexedSync:  append([]uint32(nil), track.IndexedSyncSamples...),
			ActualSync:   append([]uint32(nil), track.ActualSyncSamples...),
			ScanComplete: track.ScanComplete,
			Samples:      append([]Sample(nil), track.Samples...),
			Warnings:     append([]string(nil), track.Warnings...),
		})
		for _, warning := range track.Warnings {
			report.Warnings = append(report.Warnings, fmt.Sprintf("track %d: %s", track.ID, warning))
		}
		if track.Handler != "vide" || len(track.Samples) == 0 {
			continue
		}
		report.GOPs = append(report.GOPs, collectGOPs(track)...)
		report.SeekHits = append(report.SeekHits, collectSeekHits(track)...)
		report.InexactRanges = append(report.InexactRanges, gopInteriorRanges(track)...)
		if track.ScanComplete {
			broken, missing, falseEntries := brokenIndexRanges(track)
			report.BrokenRanges = append(report.BrokenRanges, broken...)
			report.MissingSyncSamples = append(report.MissingSyncSamples, missing...)
			report.FalseSyncSamples = append(report.FalseSyncSamples, falseEntries...)
		}
	}
	sort.Slice(report.GOPs, func(i, j int) bool {
		if report.GOPs[i].TrackID != report.GOPs[j].TrackID {
			return report.GOPs[i].TrackID < report.GOPs[j].TrackID
		}
		return report.GOPs[i].StartDTSUS < report.GOPs[j].StartDTSUS
	})
	return report
}

func collectGOPs(track *Track) []GOP {
	actual := actualSyncSamples(track)
	if len(actual) == 0 {
		return nil
	}
	endPTS := trackEndPTS(track)
	gops := make([]GOP, 0, len(actual))
	for i, sample := range actual {
		next := sample
		endDTS := trackEndDTS(track)
		end := endPTS
		if i+1 < len(actual) {
			next = actual[i+1]
			endDTS = next.DTS
			end = track.presentationCTS(next)
		}
		gop := GOP{
			TrackID:     track.ID,
			StartSample: sample.Number,
			EndSample:   uint32(len(track.Samples)),
			StartDTSUS:  sample.DTMicros(track.Timescale),
			StartPTSUS:  track.PTSMicros(sample),
			EndDTSUS:    scaleToMicros(endDTS, track.Timescale),
			EndPTSUS:    scaleToMicros(end, track.Timescale),
			Offset:      sample.Offset,
			Size:        sample.Size,
			Kind:        sample.Kind,
			Indexed:     sample.IndexedSync,
		}
		if i+1 < len(actual) {
			gop.EndSample = next.Number - 1
		}
		for n := int(sample.Number) - 1; n >= 0 && n < len(track.Samples) && n < int(gop.EndSample); n++ {
			if track.Samples[n].LeadingRASL {
				gop.RASLLeading = true
				break
			}
			if n > int(sample.Number) && track.Samples[n].RandomAccess {
				break
			}
		}
		gops = append(gops, gop)
	}
	return gops
}

func collectSeekHits(track *Track) []SeekHit {
	hits := make([]SeekHit, 0, len(track.IndexedSyncSamples)+len(track.ActualSyncSamples))
	for _, number := range track.IndexedSyncSamples {
		sample := track.Samples[number-1]
		hits = append(hits, SeekHit{
			TrackID:            track.ID,
			Source:             "stss",
			Sample:             sample.Number,
			PTSUS:              track.PTSMicros(sample),
			Offset:             sample.Offset,
			Size:               sample.Size,
			Kind:               sample.Kind,
			ActualRandomAccess: sample.RandomAccess,
		})
	}
	for _, sample := range actualSyncSamples(track) {
		hits = append(hits, SeekHit{
			TrackID:            track.ID,
			Source:             "bitstream",
			Sample:             sample.Number,
			PTSUS:              track.PTSMicros(sample),
			Offset:             sample.Offset,
			Size:               sample.Size,
			Kind:               sample.Kind,
			ActualRandomAccess: true,
		})
	}
	return hits
}

func gopInteriorRanges(track *Track) []TimeRange {
	gops := collectGOPs(track)
	ranges := make([]TimeRange, 0, len(gops))
	for _, gop := range gops {
		if gop.EndPTSUS <= gop.StartPTSUS {
			continue
		}
		ranges = append(ranges, TimeRange{
			TrackID: track.ID,
			StartUS: gop.StartPTSUS,
			EndUS:   gop.EndPTSUS,
			Reason:  "gop-interior",
		})
	}
	return ranges
}

func brokenIndexRanges(track *Track) ([]TimeRange, []uint32, []uint32) {
	actual := actualSyncSamples(track)
	if len(actual) == 0 {
		return nil, nil, nil
	}
	endPTS := scaleToMicros(trackEndPTS(track), track.Timescale)
	var ranges []TimeRange
	var missing []uint32
	var falseEntries []uint32
	for i, sample := range actual {
		end := endPTS
		if i+1 < len(actual) {
			end = track.PTSMicros(actual[i+1])
		}
		if !sample.IndexedSync {
			missing = append(missing, sample.Number)
			ranges = append(ranges, TimeRange{
				TrackID: track.ID,
				StartUS: track.PTSMicros(sample),
				EndUS:   end,
				Reason:  "missing-rap-index",
			})
		}
	}
	for _, number := range track.IndexedSyncSamples {
		sample := track.Samples[number-1]
		if sample.RandomAccess {
			continue
		}
		falseEntries = append(falseEntries, number)
		end := endPTS
		startPTS := track.PTSMicros(sample)
		for _, rap := range actual {
			rapPTS := track.PTSMicros(rap)
			if rapPTS >= startPTS && rap.Number != sample.Number {
				end = rapPTS
				break
			}
		}
		ranges = append(ranges, TimeRange{
			TrackID: track.ID,
			StartUS: startPTS,
			EndUS:   end,
			Reason:  "false-sync-entry",
		})
	}
	return ranges, missing, falseEntries
}

func actualSyncSamples(track *Track) []Sample {
	actual := make([]Sample, 0, len(track.ActualSyncSamples))
	for _, number := range track.ActualSyncSamples {
		if number >= 1 && int(number) <= len(track.Samples) {
			actual = append(actual, track.Samples[number-1])
		}
	}
	sort.SliceStable(actual, func(i, j int) bool { return actual[i].DTS < actual[j].DTS })
	return actual
}

func trackEndDTS(track *Track) int64 {
	var end int64
	for _, sample := range track.Samples {
		if sample.DTS+int64(sample.Duration) > end {
			end = sample.DTS + int64(sample.Duration)
		}
	}
	if track.Duration != 0 && int64(track.Duration) > end {
		end = int64(track.Duration)
	}
	return end
}

func trackEndPTS(track *Track) int64 {
	if len(track.Samples) == 0 {
		return 0
	}
	end := track.presentationCTS(track.Samples[0]) + int64(track.Samples[0].Duration)
	for _, sample := range track.Samples[1:] {
		sampleEnd := track.presentationCTS(sample) + int64(sample.Duration)
		if sampleEnd > end {
			end = sampleEnd
		}
	}
	return end
}

func (f *File) SeekResults(targetUS int64, trackID uint32) ([]SeekResult, error) {
	var results []SeekResult
	for _, track := range f.Tracks {
		if track.Handler != "vide" {
			continue
		}
		if trackID != 0 && track.ID != trackID {
			continue
		}
		if len(track.Samples) == 0 {
			continue
		}
		results = append(results, seekTrack(track, targetUS))
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no matching video track")
	}
	return results, nil
}

func seekTrack(track *Track, targetUS int64) SeekResult {
	requested := sampleForTime(track, targetUS)
	indexed := latestIndexedSync(track, track.presentationCTS(requested), targetUS)
	actual := latestActualSync(track, track.presentationCTS(requested))
	result := SeekResult{
		TrackID:         track.ID,
		TargetUS:        targetUS,
		RequestedSample: requested.Number,
		RequestedPTSUS:  track.PTSMicros(requested),
		IndexedSample:   indexed.Number,
		IndexedPTSUS:    track.PTSMicros(indexed),
		IndexedOffset:   indexed.Offset,
		IndexedIsRAP:    indexed.RandomAccess,
		FalseSuccess:    indexed.IndexedSync && !indexed.RandomAccess,
	}
	if actual.Number != 0 {
		result.ActualSample = actual.Number
		result.ActualPTSUS = track.PTSMicros(actual)
	}
	result.Exact = requested.Number == actual.Number && actual.RandomAccess
	result.PreciseAfterFix = requested.RandomAccess && requested.BitstreamScanned
	return result
}

func sampleForTime(track *Track, targetUS int64) Sample {
	ordered := append([]Sample(nil), track.Samples...)
	sort.SliceStable(ordered, func(i, j int) bool {
		left := track.PTSMicros(ordered[i])
		right := track.PTSMicros(ordered[j])
		if left != right {
			return left < right
		}
		return ordered[i].Number < ordered[j].Number
	})
	if targetUS <= track.PTSMicros(ordered[0]) {
		return ordered[0]
	}
	chosen := ordered[0]
	for _, sample := range ordered {
		if track.PTSMicros(sample) <= targetUS {
			chosen = sample
		} else {
			break
		}
	}
	return chosen
}

func latestIndexedSync(track *Track, cts int64, targetUS int64) Sample {
	var chosen Sample
	for _, number := range track.IndexedSyncSamples {
		sample := track.Samples[number-1]
		if track.presentationCTS(sample) <= cts {
			chosen = sample
		}
	}
	if chosen.Number == 0 {
		for _, number := range track.IndexedSyncSamples {
			sample := track.Samples[number-1]
			if chosen.Number == 0 || track.PTSMicros(sample) < track.PTSMicros(chosen) {
				chosen = sample
			}
		}
	}
	if chosen.Number == 0 {
		return sampleForTime(track, targetUS)
	}
	return chosen
}

func latestActualSync(track *Track, cts int64) Sample {
	var chosen Sample
	for _, number := range track.ActualSyncSamples {
		sample := track.Samples[number-1]
		if track.presentationCTS(sample) <= cts {
			chosen = sample
		}
	}
	return chosen
}
