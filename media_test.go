package mp4seek

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

type fixtureOptions struct {
	includeSTSS      bool
	syncEntries      []uint32
	sampleKinds      []byte
	compositionShift uint32
	includeEdit      bool
	editMediaTime    int32
	mdatBeforeMoov   bool
	useCO64          bool
}

func TestScanReportsMissingGOP(t *testing.T) {
	data := buildAVCFixture(t, fixtureOptions{
		includeSTSS: true,
		syncEntries: []uint32{1},
		sampleKinds: []byte{5, 1, 1, 5, 1, 1},
	})
	file, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	track := file.Tracks[0]
	if !track.ScanComplete {
		t.Fatalf("scan incomplete: %v", track.Warnings)
	}
	if !sameNumbers(track.ActualSyncSamples, []uint32{1, 4}) {
		t.Fatalf("actual sync samples = %v, want [1 4]", track.ActualSyncSamples)
	}
	report := file.Report()
	if !sameNumbers(report.MissingSyncSamples, []uint32{4}) {
		t.Fatalf("missing sync samples = %v, want [4]", report.MissingSyncSamples)
	}
	if len(report.BrokenRanges) != 1 || report.BrokenRanges[0].Reason != "missing-rap-index" {
		t.Fatalf("broken ranges = %+v, want one missing-rap-index range", report.BrokenRanges)
	}
	if report.BrokenRanges[0].EndUS != 200_000 {
		t.Fatalf("broken range ends at %dus, want 200000us", report.BrokenRanges[0].EndUS)
	}
	if len(report.GOPs) != 2 || report.GOPs[1].Indexed {
		t.Fatalf("GOPs = %+v, want second GOP unindexed", report.GOPs)
	}
	if report.GOPs[1].StartSample != 4 {
		t.Fatalf("second GOP starts at sample %d, want 4", report.GOPs[1].StartSample)
	}
}

func TestRepairOnlyChangesIndexAndOffsets(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "broken.mp4")
	output := filepath.Join(dir, "fixed.mp4")
	data := buildAVCFixture(t, fixtureOptions{
		includeSTSS: true,
		syncEntries: []uint32{1},
		sampleKinds: []byte{5, 1, 1, 5, 1, 1},
	})
	if err := os.WriteFile(input, data, 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := RepairFile(input, output)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Changed || !report.PayloadOK {
		t.Fatalf("repair report = %+v, want changed and payload verified", report)
	}
	if report.OffsetDelta != 4 {
		t.Fatalf("chunk offset delta = %d, want 4", report.OffsetDelta)
	}
	fixed, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	file, err := Parse(fixed)
	if err != nil {
		t.Fatal(err)
	}
	if !sameNumbers(file.Tracks[0].IndexedSyncSamples, []uint32{1, 4}) {
		t.Fatalf("fixed stss = %v, want [1 4]", file.Tracks[0].IndexedSyncSamples)
	}
	if !bytes.Equal(
		fixed[file.Tracks[0].Samples[3].Offset:file.Tracks[0].Samples[3].Offset+uint64(file.Tracks[0].Samples[3].Size)],
		originalSample(t, data, 4),
	) {
		t.Fatal("repaired sample 4 points at different payload bytes")
	}
}

func TestMissingSTSSIsRebuilt(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "all-sync.mp4")
	output := filepath.Join(dir, "fixed.mp4")
	data := buildAVCFixture(t, fixtureOptions{
		includeSTSS: false,
		sampleKinds: []byte{5, 1, 1, 5, 1, 1},
	})
	if err := os.WriteFile(input, data, 0o644); err != nil {
		t.Fatal(err)
	}
	file, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if !sameNumbers(file.Tracks[0].IndexedSyncSamples, []uint32{1, 2, 3, 4, 5, 6}) {
		t.Fatalf("absent stss should initially mean all sync, got %v", file.Tracks[0].IndexedSyncSamples)
	}
	report, err := RepairFile(input, output)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Changed {
		t.Fatal("repair did not add stss")
	}
	fixed, err := ParseFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !sameNumbers(fixed.Tracks[0].IndexedSyncSamples, []uint32{1, 4}) {
		t.Fatalf("fixed stss = %v, want [1 4]", fixed.Tracks[0].IndexedSyncSamples)
	}
}

func TestRepairWithCO64AndMoovAfterMDAT(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "late-moov.mp4")
	output := filepath.Join(dir, "fixed.mp4")
	data := buildAVCFixture(t, fixtureOptions{
		includeSTSS:    true,
		syncEntries:    []uint32{1},
		sampleKinds:    []byte{5, 1, 1, 5, 1, 1},
		mdatBeforeMoov: true,
		useCO64:        true,
	})
	if err := os.WriteFile(input, data, 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := RepairFile(input, output)
	if err != nil {
		t.Fatal(err)
	}
	if !report.PayloadOK || report.OffsetDelta != 0 {
		t.Fatalf("report = %+v, want verified payload and zero offset delta", report)
	}
	fixed, err := ParseFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !sameNumbers(fixed.Tracks[0].IndexedSyncSamples, []uint32{1, 4}) {
		t.Fatalf("fixed stss = %v, want [1 4]", fixed.Tracks[0].IndexedSyncSamples)
	}
}

func TestSeekDetectsFalseSuccess(t *testing.T) {
	data := buildAVCFixture(t, fixtureOptions{
		includeSTSS: true,
		syncEntries: []uint32{1, 2},
		sampleKinds: []byte{5, 1, 1, 5, 1, 1},
	})
	file, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	results, err := file.SeekResults(50_000, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || !results[0].FalseSuccess {
		t.Fatalf("seek results = %+v, want false success", results)
	}
	if results[0].ActualSample != 1 {
		t.Fatalf("actual sync sample = %d, want 1", results[0].ActualSample)
	}
}

func TestEditListMapsMediaTimeToPresentationTime(t *testing.T) {
	data := buildAVCFixture(t, fixtureOptions{
		includeSTSS:      true,
		syncEntries:      []uint32{1},
		sampleKinds:      []byte{5, 1, 1, 5, 1, 1},
		compositionShift: 3000,
		includeEdit:      true,
		editMediaTime:    3000,
	})
	file, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	track := file.Tracks[0]
	if track.EditOffset != -3000 {
		t.Fatalf("edit offset = %d, want -3000", track.EditOffset)
	}
	if got := track.PTSMicros(track.Samples[0]); got != 0 {
		t.Fatalf("first presentation PTS = %d, want 0", got)
	}
	if report := file.Report(); report.GOPs[0].StartPTSUS != 0 {
		t.Fatalf("first GOP starts at %d, want 0", report.GOPs[0].StartPTSUS)
	}
}

func buildAVCFixture(t *testing.T, opts fixtureOptions) []byte {
	t.Helper()
	var samples [][]byte
	for _, kind := range opts.sampleKinds {
		switch kind {
		case 5:
			samples = append(samples, lengthPrefixedNAL(7, 8, 5))
		case 1:
			samples = append(samples, lengthPrefixedNAL(1))
		default:
			t.Fatalf("unsupported sample kind %d", kind)
		}
	}
	return buildFixture(t, opts, samples)
}

func lengthPrefixedNAL(types ...byte) []byte {
	var out []byte
	for _, typ := range types {
		nal := []byte{typ, 0x80, 0x00, 0x01}
		var length [4]byte
		binary.BigEndian.PutUint32(length[:], uint32(len(nal)))
		out = append(out, length[:]...)
		out = append(out, nal...)
	}
	return out
}

func buildFixture(t *testing.T, opts fixtureOptions, samples [][]byte) []byte {
	t.Helper()
	const timescale = 90000
	const sampleDelta = 3000
	ftyp := makeBox("ftyp", append([]byte("isom\x00\x00\x02\x00"), []byte("isomiso2avc1mp41")...))
	var mdatPayload []byte
	var sizes []uint32
	for _, sample := range samples {
		mdatPayload = append(mdatPayload, sample...)
		sizes = append(sizes, uint32(len(sample)))
	}
	moov := buildFixtureMoov(t, opts, sizes, timescale, sampleDelta, 0)
	mdatOffset := uint64(len(ftyp) + len(moov) + 8)
	if opts.mdatBeforeMoov {
		mdatOffset = uint64(len(ftyp) + 8)
	}
	moov = buildFixtureMoov(t, opts, sizes, timescale, sampleDelta, mdatOffset)
	mdat := makeBox("mdat", mdatPayload)
	if opts.mdatBeforeMoov {
		return append(append(append([]byte(nil), ftyp...), mdat...), moov...)
	}
	return append(append(append([]byte(nil), ftyp...), moov...), mdat...)
}

func originalSample(t *testing.T, data []byte, number int) []byte {
	t.Helper()
	file, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	sample := file.Tracks[0].Samples[number-1]
	return data[sample.Offset : sample.Offset+uint64(sample.Size)]
}
