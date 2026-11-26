package diag

import (
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/lsafjasf/20261127/internal/dovi"
	"github.com/lsafjasf/20261127/internal/hevc"
	"github.com/lsafjasf/20261127/internal/mp4"
)

func scanSegment(f *os.File, track *mp4.Track, entry *mp4.SampleEntry, seg *Segment, opts Options) {
	if entry.Encrypted {
		addIssue(seg, CategoryDRM, "encrypted_video", SeverityInfo,
			"video sample entry is protected; media samples are not read or decrypted")
		return
	}
	if entry.Codec != "hevc" {
		addIssue(seg, CategoryEvidence, "rpu_scan_unsupported", SeverityInfo,
			fmt.Sprintf("RPU NAL parsing is implemented for HEVC; %s signaling is parsed but media samples are not scanned", entry.Codec))
		return
	}
	if entry.HVCC == nil {
		return
	}
	var targets []mp4.Sample
	for _, sample := range track.Samples {
		if sample.Desc == entry.Index {
			targets = append(targets, sample)
		}
	}
	if len(targets) == 0 {
		return
	}
	limit := len(targets)
	if opts.ScanSamples > 0 && opts.ScanSamples < limit {
		limit = opts.ScanSamples
	}
	stats := &RPUStats{
		Complete:     limit == len(targets),
		ReferencesOK: true,
		NALCounts:    make(map[int]int),
	}
	seg.RPU = stats
	seenMappings := make(map[int]bool)
	layerIDs := make(map[int]bool)
	var budget int64
	skippedByBudget := false
	for i := 0; i < limit; i++ {
		sample := targets[i]
		if int64(sample.Size) > opts.MaxScanBytes-budget {
			skippedByBudget = true
			stats.Complete = false
			break
		}
		data := make([]byte, sample.Size)
		n, err := f.ReadAt(data, sample.Offset)
		if err != nil || n != len(data) {
			stats.ParseErrors++
			addIssue(seg, CategoryConflict, "sample_read_error", SeverityError,
				fmt.Sprintf("sample %d: read %d of %d bytes: %v", sample.Index, n, len(data), err))
			continue
		}
		budget += int64(sample.Size)
		stats.ScannedSamples++
		nalls, err := hevc.SplitLengthPrefixed(data, entry.HVCC.LengthSize)
		if err != nil {
			stats.ParseErrors++
			addIssue(seg, CategoryConflict, "nal_parse_error", SeverityError,
				fmt.Sprintf("sample %d: %v", sample.Index, err))
			continue
		}
		vclInSample := false
		rpuInSample := 0
		for _, nal := range nalls {
			stats.TotalNALCount++
			stats.NALCounts[nal.Type]++
			if nal.Type >= 0 && nal.Type <= 31 {
				vclInSample = true
			}
			if nal.Type >= 32 {
				layerIDs[nal.LayerID] = true
			}
			if nal.LayerID > 0 && nal.Type <= 31 {
				seg.Enhancement.ObservedELNALs++
			}
			if nal.Type != hevc.NALRPU {
				continue
			}
			rpuInSample++
			record, parseErr := dovi.ParseRPU(nal.RBSP())
			if parseErr != nil && !errors.Is(parseErr, dovi.ErrCRCMismatch) {
				stats.ParseErrors++
				addIssue(seg, CategoryConflict, "rpu_parse_error", SeverityError,
					fmt.Sprintf("sample %d: %v", sample.Index, parseErr))
				continue
			}
			if record == nil {
				continue
			}
			rec := rpuRecord(sample.Index, record)
			if stats.First == nil {
				copy := rec
				stats.First = &copy
			}
			stats.Profiles = appendUniqueInt(stats.Profiles, rec.Profile)
			appendRPUValidationIssues(seg, sample.Index, record)
			if errors.Is(parseErr, dovi.ErrCRCMismatch) {
				stats.CRCErrors++
				addIssue(seg, CategoryConflict, "rpu_crc_mismatch", SeverityError,
					fmt.Sprintf("sample %d: RPU CRC32 does not match its payload", sample.Index))
				continue
			}
			if rec.UsePreviousRPU {
				if !seenMappings[rec.PreviousRPUID] {
					stats.ReferenceErrors++
					stats.ReferencesOK = false
					addIssue(seg, CategoryConflict, "rpu_reference_missing", SeverityError,
						fmt.Sprintf("sample %d references RPU mapping %d before it appears in this segment",
							sample.Index, rec.PreviousRPUID))
				}
			} else if rec.MappingID >= 0 {
				seenMappings[rec.MappingID] = true
			}
		}
		if vclInSample && rpuInSample == 0 {
			stats.MissingRPUSamples = append(stats.MissingRPUSamples, sample.Index)
		}
		if rpuInSample > 0 {
			stats.SamplesWithRPU++
		}
	}
	if stats.ScannedSamples != limit {
		stats.Complete = false
	}
	for id := range layerIDs {
		seg.Enhancement.ObservedLayerIDs = append(seg.Enhancement.ObservedLayerIDs, id)
	}
	sort.Ints(seg.Enhancement.ObservedLayerIDs)
	if len(stats.MissingRPUSamples) > 0 {
		sampleText := formatSampleList(stats.MissingRPUSamples)
		addIssue(seg, CategoryConflict, "rpu_missing_in_samples", SeverityError,
			"video access units without an RPU in scanned samples: "+sampleText)
	}
	if skippedByBudget {
		addIssue(seg, CategoryEvidence, "rpu_scan_budget", SeverityWarn,
			fmt.Sprintf("RPU scan stopped at the %d MiB byte budget", opts.MaxScanBytes>>20))
	} else if !stats.Complete {
		addIssue(seg, CategoryEvidence, "rpu_scan_limited", SeverityWarn,
			fmt.Sprintf("only %d of %d samples were scanned", stats.ScannedSamples, len(targets)))
	}
	if stats.ScannedSamples > 0 && stats.SamplesWithRPU == 0 && stats.ParseErrors == 0 {
		stats.ReferencesOK = false
	}
}

func rpuRecord(sampleIndex int, r *dovi.RPU) RPURecord {
	return RPURecord{
		SampleIndex:     sampleIndex,
		Profile:         r.Profile,
		RPUType:         r.RPUType,
		RPUFormat:       r.RPUFormat,
		VDRRPUProfile:   r.VDRRPUProfile,
		VDRRPULevel:     r.VDRRPULevel,
		BLFullRange:     r.BLFullRange,
		ELResampling:    r.ELResampling,
		DisableResidual: r.DisableResidual,
		UsePreviousRPU:  r.UsePreviousRPU,
		PreviousRPUID:   r.PreviousRPUID,
		MappingID:       r.MappingID,
		CRCValid:        r.CRCValid,
	}
}

func appendRPUValidationIssues(seg *Segment, sampleIndex int, r *dovi.RPU) {
	for _, msg := range r.HeaderValidation {
		addIssue(seg, CategoryConflict, "rpu_header_invalid", SeverityError,
			fmt.Sprintf("sample %d: %s", sampleIndex, msg))
	}
}

func appendUniqueInt(values []int, v int) []int {
	for _, item := range values {
		if item == v {
			return values
		}
	}
	return append(values, v)
}

func formatSampleList(values []int) string {
	if len(values) > 8 {
		return fmt.Sprintf("%v ... (%d total)", values[:8], len(values))
	}
	return fmt.Sprint(values)
}
