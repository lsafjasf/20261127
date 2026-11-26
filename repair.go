package mp4seek

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

type stssPatch struct {
	track   *Track
	oldSync []uint32
	newSync []uint32
	newStss []byte
	changed bool
}

func RepairFile(input, output string) (*RepairReport, error) {
	src, err := os.ReadFile(input)
	if err != nil {
		return nil, err
	}
	file, err := Parse(src)
	if err != nil {
		return nil, fmt.Errorf("parse input: %w", err)
	}
	patches, err := planSTSSPatches(file)
	if err != nil {
		return nil, err
	}
	report := &RepairReport{
		Input:        input,
		Output:       output,
		OriginalSize: int64(len(src)),
		Tracks:       make([]TrackRepair, 0, len(patches)),
	}
	report.MDATsBefore = mdatHashes(file)
	for _, patch := range patches {
		report.Tracks = append(report.Tracks, TrackRepair{
			ID:      patch.track.ID,
			OldSync: append([]uint32(nil), patch.oldSync...),
			NewSync: append([]uint32(nil), patch.newSync...),
			Added:   difference(patch.newSync, patch.oldSync),
			Removed: difference(patch.oldSync, patch.newSync),
		})
		report.Changed = report.Changed || patch.changed
	}

	repaired, err := rebuildFile(file, patches, &report.MoovSizeDelta, &report.OffsetDelta)
	if err != nil {
		return nil, err
	}
	report.RepairedSize = int64(len(repaired))
	if !report.Changed {
		repaired = append([]byte(nil), src...)
		report.MoovSizeDelta = 0
		report.OffsetDelta = 0
	}
	if err := verifyRepair(src, repaired, report); err != nil {
		return nil, err
	}
	if err := writeAtomically(output, repaired, fileMode(input)); err != nil {
		return nil, err
	}
	return report, nil
}

func planSTSSPatches(file *File) ([]*stssPatch, error) {
	var patches []*stssPatch
	for _, track := range file.Tracks {
		if track.Handler != "vide" || len(track.Samples) == 0 {
			continue
		}
		if !track.ScanComplete {
			return nil, fmt.Errorf("track %d: bitstream scan is incomplete; refusing to rewrite stss", track.ID)
		}
		desired := append([]uint32(nil), track.ActualSyncSamples...)
		if len(desired) == 0 {
			return nil, fmt.Errorf("track %d: no actual random access point found; refusing to rewrite stss", track.ID)
		}
		allSamples := len(desired) == len(track.Samples)
		oldSync := append([]uint32(nil), track.rawSTSS...)
		changed := !sameNumbers(oldSync, desired)
		if !track.hasSTSS {
			oldSync = append([]uint32(nil), track.IndexedSyncSamples...)
			changed = !allSamples
		}
		var encoded []byte
		if !allSamples {
			encoded = encodeSTSS(track, desired)
		}
		patches = append(patches, &stssPatch{
			track:   track,
			oldSync: oldSync,
			newSync: desired,
			newStss: encoded,
			changed: changed,
		})
	}
	if len(patches) == 0 {
		return nil, fmt.Errorf("no scannable video tracks to repair")
	}
	return patches, nil
}

func encodeSTSS(track *Track, samples []uint32) []byte {
	payload := make([]byte, 8+len(samples)*4)
	binary.BigEndian.PutUint32(payload[4:8], uint32(len(samples)))
	for i, number := range samples {
		binary.BigEndian.PutUint32(payload[8+i*4:], number)
	}
	stss, ok, _ := findBox(track.ownerData, track.stbl, "stss")
	if ok {
		return makeBoxLike("stss", stss, track.ownerData, payload)
	}
	return makeBox("stss", payload)
}

func rebuildFile(file *File, patches []*stssPatch, moovDelta, offsetDelta *int64) ([]byte, error) {
	first, err := buildMoov(file, patches, 0)
	if err != nil {
		return nil, err
	}
	*moovDelta = int64(len(first) - file.moov.size())
	*offsetDelta = *moovDelta
	if *moovDelta != 0 && !hasChunksAfter(file, uint64(file.moov.end)) {
		*offsetDelta = 0
	}
	finalMoov, err := buildMoov(file, patches, *offsetDelta)
	if err != nil {
		return nil, err
	}
	if len(finalMoov) != len(first) {
		return nil, fmt.Errorf("internal error: chunk offset patch changed moov size")
	}
	return replaceTopLevelBox(file.data, file.moov, finalMoov)
}

func buildMoov(file *File, patches []*stssPatch, chunkDelta int64) ([]byte, error) {
	replacements := make(map[int][]byte, len(patches))
	for _, patch := range patches {
		newStbl, err := rebuildSTBL(file, patch, chunkDelta)
		if err != nil {
			return nil, err
		}
		newTrak := rebuildAncestors(file.data, patch.track.stblPath, newStbl)
		replacements[patch.track.stblPath[len(patch.track.stblPath)-1].start] = newTrak
	}
	return replaceChildren(file.data, file.moov, replacements)
}

func rebuildAncestors(data []byte, path []box, replacement []byte) []byte {
	child := replacement
	for i := 0; i+1 < len(path); i++ {
		child = replaceChild(data, path[i+1], path[i], child)
	}
	return child
}

func replaceChildren(data []byte, parent box, replacements map[int][]byte) ([]byte, error) {
	children, err := parseBoxes(data, parent.payload, parent.end)
	if err != nil {
		return nil, err
	}
	var payload []byte
	pos := parent.payload
	for _, child := range children {
		payload = append(payload, data[pos:child.start]...)
		if replacement, ok := replacements[child.start]; ok {
			payload = append(payload, replacement...)
		} else {
			payload = append(payload, data[child.start:child.end]...)
		}
		pos = child.end
	}
	payload = append(payload, data[pos:parent.end]...)
	return makeBoxLike(parent.typ, parent, data, payload), nil
}

func replaceTopLevelBox(data []byte, oldBox box, replacement []byte) ([]byte, error) {
	boxes, err := parseBoxes(data, 0, len(data))
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, len(data)-oldBox.size()+len(replacement))
	pos := 0
	for _, b := range boxes {
		out = append(out, data[pos:b.start]...)
		if b.start == oldBox.start {
			out = append(out, replacement...)
		} else {
			out = append(out, data[b.start:b.end]...)
		}
		pos = b.end
	}
	out = append(out, data[pos:]...)
	return out, nil
}

func hasChunksAfter(file *File, position uint64) bool {
	for _, track := range file.Tracks {
		for _, sample := range track.Samples {
			if sample.Offset >= position {
				return true
			}
		}
	}
	return false
}

func mdatHashes(file *File) []PayloadSpan {
	out := make([]PayloadSpan, 0, len(file.MDATs))
	for _, span := range file.MDATs {
		sum := sha256.Sum256(file.data[span.Offset : span.Offset+span.Size])
		out = append(out, PayloadSpan{
			Offset: span.Offset,
			Size:   span.Size,
			SHA256: hex.EncodeToString(sum[:]),
		})
	}
	return out
}

func verifyRepair(before, after []byte, report *RepairReport) error {
	oldFile, err := Parse(before)
	if err != nil {
		return err
	}
	newFile, err := Parse(after)
	if err != nil {
		return fmt.Errorf("verify repaired file: %w", err)
	}
	report.MDATsAfter = mdatHashes(newFile)
	if len(report.MDATsBefore) != len(report.MDATsAfter) {
		return fmt.Errorf("mdat count changed during repair")
	}
	for i := range report.MDATsBefore {
		if report.MDATsBefore[i].Size != report.MDATsAfter[i].Size ||
			report.MDATsBefore[i].SHA256 != report.MDATsAfter[i].SHA256 {
			return fmt.Errorf("mdat %d payload changed during repair", i+1)
		}
	}
	newTracks := make(map[uint32]*Track, len(newFile.Tracks))
	for _, track := range newFile.Tracks {
		newTracks[track.ID] = track
	}
	for _, oldTrack := range oldFile.Tracks {
		newTrack := newTracks[oldTrack.ID]
		if newTrack == nil || len(newTrack.Samples) != len(oldTrack.Samples) {
			return fmt.Errorf("track %d sample table changed unexpectedly", oldTrack.ID)
		}
		for i := range oldTrack.Samples {
			oldSample := oldTrack.Samples[i]
			newSample := newTrack.Samples[i]
			if oldSample.Size != newSample.Size {
				return fmt.Errorf("track %d sample %d size changed", oldTrack.ID, oldSample.Number)
			}
			if !bytes.Equal(
				before[oldSample.Offset:oldSample.Offset+uint64(oldSample.Size)],
				after[newSample.Offset:newSample.Offset+uint64(newSample.Size)],
			) {
				return fmt.Errorf("track %d sample %d payload changed", oldTrack.ID, oldSample.Number)
			}
		}
	}
	report.PayloadOK = true
	return nil
}

func writeAtomically(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".mp4seek-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Chmod(mode); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempName, path)
}

func fileMode(path string) os.FileMode {
	info, err := os.Stat(path)
	if err != nil {
		return 0o644
	}
	return info.Mode().Perm()
}

func sameNumbers(left, right []uint32) bool {
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

func difference(left, right []uint32) []uint32 {
	seen := make(map[uint32]bool, len(right))
	for _, value := range right {
		seen[value] = true
	}
	var out []uint32
	for _, value := range left {
		if !seen[value] {
			out = append(out, value)
		}
	}
	return out
}
