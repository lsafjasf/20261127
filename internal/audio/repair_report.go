package audio

import (
	"math"
	"sort"
)

func buildRepairReport(input, output *Buffer, before, after *Analysis, evidenceLimit int) RepairReport {
	sr := input.Format.SampleRate
	dialogue := before.Report.DialogueChannel
	report := RepairReport{Before: before.Report, After: after.Report}
	var ranges [][2]int
	var beforeEnergy, afterEnergy, bandBeforeEnergy, bandAfterEnergy float64
	repaired := 0
	for i, event := range before.Report.Events {
		start := max(0, event.StartSample)
		end := min(len(before.DialogueLow), event.EndSample)
		if end <= start {
			continue
		}
		ranges = append(ranges, [2]int{start, end})
		repaired += end - start
		beforeCorr := correlationAt(before.DialogueLow, before.SourceLow, start, end, event.LagSamples)
		afterCorr := correlationAt(after.DialogueLow, after.SourceLow, start, end, event.LagSamples)
		beforeLow := RMSdBFS(before.DialogueLow[start:end])
		afterLow := RMSdBFS(after.DialogueLow[start:end])
		bandBefore := RMSdBFS(before.DialogueBand[start:end])
		bandAfter := RMSdBFS(after.DialogueBand[start:end])
		if afterLow <= leakVisibilityFloor(before.Config, bandAfter) {
			afterCorr = 0
		}
		var removedEnergy float64
		for sample := start; sample < end; sample++ {
			d := before.DialogueLow[sample] - after.DialogueLow[sample]
			removedEnergy += d * d
			beforeEnergy += before.DialogueLow[sample] * before.DialogueLow[sample]
			afterEnergy += after.DialogueLow[sample] * after.DialogueLow[sample]
			bandBeforeEnergy += before.DialogueBand[sample] * before.DialogueBand[sample]
			bandAfterEnergy += after.DialogueBand[sample] * after.DialogueBand[sample]
		}
		removedDB := 20 * math.Log10(max(math.Sqrt(removedEnergy/float64(end-start)), 1e-12))
		report.Events = append(report.Events, RepairEvent{
			Index:                  i,
			StartSample:            start,
			EndSample:              end,
			StartSec:               float64(start) / float64(sr),
			EndSec:                 float64(end) / float64(sr),
			Gain:                   event.Gain,
			LagSamples:             event.LagSamples,
			BeforeCorrelation:      beforeCorr,
			AfterCorrelation:       afterCorr,
			BeforeLowDBFS:          beforeLow,
			AfterLowDBFS:           afterLow,
			LowEnergyReductionDB:   beforeLow - afterLow,
			RemovedRMSDBFS:         removedDB,
			DialogueBandBeforeDBFS: bandBefore,
			DialogueBandAfterDBFS:  bandAfter,
			DialogueBandDeltaDB:    bandAfter - bandBefore,
		})
		report.Evidence = append(report.Evidence, selectEvidence(event, before, output, input, sr, evidenceLimit)...)
	}
	sort.SliceStable(report.Evidence, func(i, j int) bool {
		if report.Evidence[i].EventIndex == report.Evidence[j].EventIndex {
			return report.Evidence[i].Sample < report.Evidence[j].Sample
		}
		return report.Evidence[i].EventIndex < report.Evidence[j].EventIndex
	})
	corrLag := 0
	if len(report.Events) > 0 {
		corrLag = report.Events[0].LagSamples
	}
	verification := RepairVerification{
		SourceName:          before.Report.SourceName,
		DialogueName:        before.Report.DialogueName,
		SourceIndex:         before.Report.SourceIndex,
		DialogueIndex:       dialogue,
		RepairedSamples:     repaired,
		LeakBeforeSec:       before.Report.LeakTotalSec,
		LeakAfterSec:        after.Report.LeakTotalSec,
		DialogueBandDeltaDB: energyDB(bandAfterEnergy, repaired) - energyDB(bandBeforeEnergy, repaired),
	}
	if repaired > 0 {
		verification.LowEnergyBeforeDBFS = energyDB(beforeEnergy, repaired)
		verification.LowEnergyAfterDBFS = energyDB(afterEnergy, repaired)
		verification.LowEnergyReductionDB = verification.LowEnergyBeforeDBFS - verification.LowEnergyAfterDBFS
		verification.DialogueBandBeforeDBFS = energyDB(bandBeforeEnergy, repaired)
		verification.DialogueBandAfterDBFS = energyDB(bandAfterEnergy, repaired)
		verification.CorrelationBefore = correlationAcross(before.DialogueLow, before.SourceLow, ranges, corrLag)
		verification.CorrelationAfter = correlationAcross(after.DialogueLow, after.SourceLow, ranges, corrLag)
		if verification.LowEnergyAfterDBFS <= leakVisibilityFloor(before.Config, verification.DialogueBandAfterDBFS) {
			verification.CorrelationAfter = 0
		}
	}
	report.Verification = verification
	return report
}

func energyDB(sum float64, count int) float64 {
	if count <= 0 {
		return -240
	}
	return 20 * math.Log10(max(math.Sqrt(sum/float64(count)), 1e-12))
}

func correlationAcross(dialogue, reference []float64, ranges [][2]int, lag int) float64 {
	var dot, de, re float64
	for _, r := range ranges {
		d, dend, rend, _ := dotAt(dialogue, reference, r[0], r[1], lag)
		dot += d
		de += dend
		re += rend
	}
	if de <= 1e-18 || re <= 1e-18 {
		return 0
	}
	return dot / math.Sqrt(de*re)
}

func selectEvidence(event LeakEvent, before *Analysis, output, input *Buffer, sampleRate, limit int) []SampleEvidence {
	start := max(0, event.StartSample)
	end := min(len(before.DialogueLow), event.EndSample)
	if end <= start || limit <= 0 {
		return nil
	}
	part := max(1, limit/3)
	peakPart := max(1, limit-2*part)
	chosen := make(map[int]string)
	addRange := func(from, to int, region string) {
		for i := max(start, from); i < min(end, to); i++ {
			if _, ok := chosen[i]; !ok {
				chosen[i] = region
			}
		}
	}
	addRange(start, start+part, "onset")
	addRange(end-part, end, "release")
	peak := start
	best := -1.0
	for i := start; i < end; i++ {
		j := i - event.LagSamples
		if j < 0 || j >= len(before.SourceLow) {
			continue
		}
		v := math.Abs(before.SourceLow[j])
		if v > best {
			best, peak = v, i
		}
	}
	left := max(start, peak-peakPart/2)
	right := min(end, left+peakPart)
	left = max(start, right-peakPart)
	addRange(left, right, "peak")
	indices := make([]int, 0, len(chosen))
	for i := range chosen {
		indices = append(indices, i)
	}
	sort.Ints(indices)
	dialogue := before.Report.DialogueChannel
	evidence := make([]SampleEvidence, 0, len(indices))
	for _, sample := range indices {
		refIndex := sample - event.LagSamples
		var ref float64
		if refIndex >= 0 && refIndex < len(before.SourceLow) {
			ref = before.SourceLow[refIndex]
		}
		beforeSample := input.Samples[dialogue][sample]
		afterSample := output.Samples[dialogue][sample]
		removed := beforeSample - afterSample
		evidence = append(evidence, SampleEvidence{
			EventIndex:   event.Index,
			Region:       chosen[sample],
			Sample:       sample,
			TimeSec:      float64(sample) / float64(sampleRate),
			Before:       beforeSample,
			ReferenceLow: ref,
			Removed:      removed,
			After:        afterSample,
			BeforePCM16:  pcm16(beforeSample),
			RemovedPCM16: pcm16(removed),
			AfterPCM16:   pcm16(afterSample),
		})
	}
	return evidence
}

func pcm16(v float64) int {
	return int(math.Round(clamp(v, -1, 1) * 32768))
}
