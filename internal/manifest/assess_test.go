package manifest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeProber struct {
	probes map[string]ProbeData
	hashes map[string]string
	errors map[string]error
}

func (f *fakeProber) Probe(_ context.Context, path string) (ProbeData, error) {
	if err := f.errors[path]; err != nil {
		return ProbeData{}, err
	}
	probe, ok := f.probes[path]
	if !ok {
		return ProbeData{}, fmt.Errorf("unexpected probe path %s", path)
	}
	return probe, nil
}

func (f *fakeProber) StreamSHA256(_ context.Context, path string, streamIndex int) (string, error) {
	key := fmt.Sprintf("%s#%d", path, streamIndex)
	hash, ok := f.hashes[key]
	if !ok {
		return "", fmt.Errorf("missing fake hash for %s", key)
	}
	return hash, nil
}

type testFixture struct {
	dir    string
	cfg    Config
	prober *fakeProber
}

func newTestFixture(t *testing.T) *testFixture {
	t.Helper()
	dir := t.TempDir()
	hdrPath := writeFixtureFile(t, dir, "hdr.mkv", "hdr media")
	sdrPath := writeFixtureFile(t, dir, "sdr.mkv", "sdr media")
	audioPath := writeFixtureFile(t, dir, "main.m4a", "shared audio")
	subtitlePath := writeFixtureFile(t, dir, "zh.srt", "shared subtitle")

	cfg := Config{
		Schema:  InputSchema,
		Project: ProjectConfig{ID: "test-delivery", Title: "Test"},
		HDR:     VersionConfig{Path: hdrPath, Label: "HDR10"},
		SDR:     VersionConfig{Path: sdrPath, Label: "SDR"},
		Shared: SharedConfig{
			Audio: []SharedAsset{{
				ID: "main-5.1", Path: audioPath, Language: "zh", Title: "Main 5.1", Default: boolPointer(true),
			}},
			Subtitles: []SharedAsset{{
				ID: "zh", Path: subtitlePath, Language: "zh-Hans", Title: "Chinese", Default: boolPointer(true),
			}},
		},
		Timeline: TimelineConfig{
			DurationToleranceMS: 40,
			StartToleranceMS:    100,
			FrameRateTolerance:  0.0001,
		},
		Cuts: CutsConfig{
			Points: []CutPoint{
				{ID: "C001", TimeMS: 0, Label: "Opening"},
				{ID: "C002", TimeMS: 1000, Label: "Scene 2"},
			},
		},
	}

	hdrProbe := baseVersionProbe("hevc", "yuv420p10le", true)
	sdrProbe := baseVersionProbe("h264", "yuv420p", false)
	prober := &fakeProber{
		probes: map[string]ProbeData{
			hdrPath:      hdrProbe,
			sdrPath:      sdrProbe,
			audioPath:    assetProbe("audio", "aac"),
			subtitlePath: assetProbe("subtitle", "subrip"),
		},
		hashes: map[string]string{
			fmt.Sprintf("%s#1", hdrPath):      "audio-stream-sha256",
			fmt.Sprintf("%s#2", hdrPath):      "subtitle-stream-sha256",
			fmt.Sprintf("%s#1", sdrPath):      "audio-stream-sha256",
			fmt.Sprintf("%s#2", sdrPath):      "subtitle-stream-sha256",
			fmt.Sprintf("%s#0", audioPath):    "audio-stream-sha256",
			fmt.Sprintf("%s#0", subtitlePath): "subtitle-stream-sha256",
		},
		errors: map[string]error{},
	}
	return &testFixture{dir: dir, cfg: cfg, prober: prober}
}

func (f *testFixture) assess(t *testing.T) Report {
	t.Helper()
	report, err := Assess(context.Background(), f.cfg, AssessOptions{
		Prober:      f.prober,
		ToolVersion: "test",
		Now: func() time.Time {
			return time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("Assess() error = %v", err)
	}
	return report
}

func TestAssessCompleteDualVersion(t *testing.T) {
	fixture := newTestFixture(t)
	report := fixture.assess(t)

	if !report.Complete || report.Status != StatusComplete {
		t.Fatalf("expected complete manifest, got status=%s failures=%v", report.Status, report.Failures)
	}
	if report.Summary.FailedChecks != 0 {
		t.Fatalf("expected zero failed checks, got %d", report.Summary.FailedChecks)
	}
	if report.Versions.HDR.Status != StatusComplete || report.Versions.SDR.Status != StatusComplete {
		t.Fatalf("expected both versions complete, got HDR=%s SDR=%s", report.Versions.HDR.Status, report.Versions.SDR.Status)
	}
	if report.Versions.HDR.Color.Standard != "HDR10/PQ" {
		t.Fatalf("unexpected HDR standard %q", report.Versions.HDR.Color.Standard)
	}
	if report.Shared.Timeline.Status != StatusComplete {
		t.Fatalf("expected shared timeline complete, got %s", report.Shared.Timeline.Status)
	}
	if len(report.Shared.Assets) != 2 {
		t.Fatalf("expected two shared assets, got %d", len(report.Shared.Assets))
	}
}

func TestAssessIncompleteWhenOneVersionLacksSubtitle(t *testing.T) {
	fixture := newTestFixture(t)
	sdrProbe := fixture.prober.probes[fixture.cfg.SDR.Path]
	sdrProbe.Streams = sdrProbe.Streams[:2]
	fixture.prober.probes[fixture.cfg.SDR.Path] = sdrProbe

	report := fixture.assess(t)
	if report.Complete || report.Status != StatusIncomplete {
		t.Fatalf("missing SDR subtitle must make manifest incomplete, got %s", report.Status)
	}
	assertFailure(t, report, "sdr", "tracks.subtitle_present")
	assertFailure(t, report, "sdr", "shared.zh.sdr.present")
	assertFailure(t, report, "shared", "shared.subtitle_set.identical")
	if report.Versions.SDR.Status != StatusIncomplete {
		t.Fatalf("expected SDR incomplete, got %s", report.Versions.SDR.Status)
	}
}

func TestAssessIncompleteWhenHDRColorMetadataMissing(t *testing.T) {
	fixture := newTestFixture(t)
	hdrProbe := fixture.prober.probes[fixture.cfg.HDR.Path]
	hdrProbe.Frames[0].SideDataList = nil
	fixture.prober.probes[fixture.cfg.HDR.Path] = hdrProbe

	report := fixture.assess(t)
	assertFailure(t, report, "hdr", "color.mastering_display")
	assertFailure(t, report, "hdr", "color.content_light")
	if report.Complete {
		t.Fatal("HDR without mastering/content-light metadata must not be complete")
	}
}

func TestAssessIncompleteWhenCutPointMissing(t *testing.T) {
	fixture := newTestFixture(t)
	sdrProbe := fixture.prober.probes[fixture.cfg.SDR.Path]
	sdrProbe.Chapters = sdrProbe.Chapters[:1]
	fixture.prober.probes[fixture.cfg.SDR.Path] = sdrProbe

	report := fixture.assess(t)
	assertFailure(t, report, "sdr", "cuts.missing")
	if report.Versions.SDR.Cuts.Status != StatusIncomplete {
		t.Fatalf("expected SDR cuts incomplete, got %s", report.Versions.SDR.Cuts.Status)
	}
	if len(report.Versions.SDR.Cuts.Missing) != 1 {
		t.Fatalf("expected one missing cut, got %d", len(report.Versions.SDR.Cuts.Missing))
	}
}

func TestAssessIncompleteWhenDefaultAudioTrackMissing(t *testing.T) {
	fixture := newTestFixture(t)
	sdrProbe := fixture.prober.probes[fixture.cfg.SDR.Path]
	sdrProbe.Streams[1].Disposition = map[string]int{"default": 0}
	fixture.prober.probes[fixture.cfg.SDR.Path] = sdrProbe

	report := fixture.assess(t)
	assertFailure(t, report, "sdr", "defaults.audio_count")
	assertFailure(t, report, "sdr", "defaults.audio_shared")
	if report.Versions.SDR.Defaults.Audio != nil {
		t.Fatal("expected no default audio track")
	}
	if report.Complete {
		t.Fatal("missing default audio track must make manifest incomplete")
	}
}

func TestAssessIncompleteWhenDefaultSubtitleTimingMismatches(t *testing.T) {
	fixture := newTestFixture(t)
	sdrProbe := fixture.prober.probes[fixture.cfg.SDR.Path]
	sdrProbe.Streams[2].StartTime = "0.500000"
	fixture.prober.probes[fixture.cfg.SDR.Path] = sdrProbe

	report := fixture.assess(t)
	assertFailure(t, report, "hdr", "defaults.subtitle_timing")
	assertFailure(t, report, "sdr", "defaults.subtitle_timing")
	if report.Complete {
		t.Fatal("default subtitle timing mismatch must make manifest incomplete")
	}
}

func TestAssessIncompleteWhenSharedAssetNotEmbedded(t *testing.T) {
	fixture := newTestFixture(t)
	fixture.prober.hashes[fmt.Sprintf("%s#0", fixture.cfg.Shared.Audio[0].Path)] = "different-audio-hash"

	report := fixture.assess(t)
	assertFailure(t, report, "hdr", "shared.main-5.1.hdr.present")
	assertFailure(t, report, "sdr", "shared.main-5.1.sdr.present")
	for _, asset := range report.Shared.Assets {
		if asset.ID == "main-5.1" && asset.Status != StatusIncomplete {
			t.Fatalf("expected shared asset incomplete, got %s", asset.Status)
		}
	}
	if report.Complete {
		t.Fatal("unembedded shared asset must make manifest incomplete")
	}
}

func TestAssessPairedCutMode(t *testing.T) {
	fixture := newTestFixture(t)
	fixture.cfg.Cuts = CutsConfig{Source: "paired", ToleranceMS: 2}
	report := fixture.assess(t)
	if !report.Complete {
		t.Fatalf("matching chapter markers should pass paired mode: %v", report.Failures)
	}
	if report.Versions.HDR.Cuts.Mode != "paired" || report.Versions.SDR.Cuts.Mode != "paired" {
		t.Fatal("expected paired cut mode")
	}
}

func TestAssessPairedCutModeRejectsMismatch(t *testing.T) {
	fixture := newTestFixture(t)
	fixture.cfg.Cuts = CutsConfig{Source: "paired", ToleranceMS: 2}
	sdrProbe := fixture.prober.probes[fixture.cfg.SDR.Path]
	sdrProbe.Chapters[1].StartTime = "1.500000"
	fixture.prober.probes[fixture.cfg.SDR.Path] = sdrProbe

	report := fixture.assess(t)
	assertFailure(t, report, "sdr", "cuts.missing")
	assertFailure(t, report, "sdr", "cuts.unexpected")
}

func TestAssessMissingCutListProducesIncompleteReport(t *testing.T) {
	fixture := newTestFixture(t)
	fixture.cfg.Cuts = CutsConfig{Path: filepath.Join(fixture.dir, "missing-cuts.json")}

	report := fixture.assess(t)
	assertFailure(t, report, "hdr", "cuts.source_available")
	assertFailure(t, report, "sdr", "cuts.source_available")
	if report.Complete {
		t.Fatal("missing external cut list must not produce a complete manifest")
	}
}

func TestAssessExactFrameCountMismatch(t *testing.T) {
	fixture := newTestFixture(t)
	hdrProbe := fixture.prober.probes[fixture.cfg.HDR.Path]
	hdrProbe.Streams[0].NBFrames = "48"
	fixture.prober.probes[fixture.cfg.HDR.Path] = hdrProbe
	sdrProbe := fixture.prober.probes[fixture.cfg.SDR.Path]
	sdrProbe.Streams[0].NBFrames = "49"
	fixture.prober.probes[fixture.cfg.SDR.Path] = sdrProbe

	report := fixture.assess(t)
	assertFailure(t, report, "shared", "timeline.frame_count_delta")
	if report.Shared.Timeline.Status != StatusIncomplete {
		t.Fatalf("expected timeline incomplete, got %s", report.Shared.Timeline.Status)
	}
}

func TestRenderCompleteManifest(t *testing.T) {
	fixture := newTestFixture(t)
	report := fixture.assess(t)

	jsonData, err := MarshalJSON(report)
	if err != nil {
		t.Fatal(err)
	}
	jsonText := string(jsonData)
	for _, expected := range []string{`"status": "complete"`, `"complete": true`, `"schema": "hdr-sdr-delivery-manifest/v1"`} {
		if !strings.Contains(jsonText, expected) {
			t.Errorf("JSON manifest missing %s", expected)
		}
	}

	markdown := string(RenderMarkdown(report))
	for _, expected := range []string{"总体状态：**完整**", "## 共享时间线", "## 逐项检查", "HDR10/PQ"} {
		if !strings.Contains(markdown, expected) {
			t.Errorf("Markdown manifest missing %q", expected)
		}
	}
}

func assertFailure(t *testing.T, report Report, scope, checkID string) {
	t.Helper()
	for _, failure := range report.Failures {
		if failure.Scope == scope && failure.CheckID == checkID {
			return
		}
	}
	t.Fatalf("missing failure scope=%s id=%s; failures=%v", scope, checkID, report.Failures)
}

func baseVersionProbe(codec, pixelFormat string, hdr bool) ProbeData {
	transfer := "bt709"
	primaries := "bt709"
	matrix := "bt709"
	var sideData []ProbeSideData
	if hdr {
		transfer = "smpte2084"
		primaries = "bt2020"
		matrix = "bt2020nc"
		sideData = []ProbeSideData{
			{
				Type:         "Mastering display metadata",
				RedX:         "34000/50000",
				MaxLuminance: "10000000/10000",
				MinLuminance: "1/10000",
			},
			{Type: "Content light level metadata", MaxContent: 1000, MaxAverage: 400},
		}
	}
	return ProbeData{
		Format: ProbeFormat{StartTime: "0.000000", Duration: "2.000000"},
		Streams: []ProbeStream{
			{
				Index: 0, CodecType: "video", CodecName: codec,
				Width: 1920, Height: 1080, PixelFormat: pixelFormat,
				ColorRange: "tv", ColorSpace: matrix, ColorTransfer: transfer, ColorPrimaries: primaries,
				AverageFrameRate: "24/1", RealFrameRate: "24/1", TimeBase: "1/1000",
				NBFrames:    "48",
				Disposition: map[string]int{"default": 0},
			},
			{
				Index: 1, CodecType: "audio", CodecName: "aac",
				Channels: 6, ChannelLayout: "5.1", SampleRate: "48000",
				StartTime:   "0.000000",
				Tags:        map[string]string{"language": "zho", "title": "Main 5.1"},
				Disposition: map[string]int{"default": 1},
			},
			{
				Index: 2, CodecType: "subtitle", CodecName: "subrip",
				StartTime:   "0.000000",
				Tags:        map[string]string{"language": "zho", "title": "Chinese"},
				Disposition: map[string]int{"default": 1},
			},
		},
		Chapters: []ProbeChapter{
			{ID: JSONString("1"), StartTime: "0.000000", Tags: map[string]string{"title": "Opening"}},
			{ID: JSONString("2"), StartTime: "1.000000", Tags: map[string]string{"title": "Scene 2"}},
		},
		Frames: []ProbeFrame{{SideDataList: sideData}},
	}
}

func assetProbe(kind, codec string) ProbeData {
	return ProbeData{Streams: []ProbeStream{{
		Index: 0, CodecType: kind, CodecName: codec,
	}}}
}

func writeFixtureFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture %s: %v", name, err)
	}
	return path
}

func boolPointer(value bool) *bool {
	return &value
}
