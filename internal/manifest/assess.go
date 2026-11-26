package manifest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"time"
)

const (
	defaultDurationToleranceMS int64   = 40
	defaultStartToleranceMS    int64   = 100
	defaultFrameRateTolerance  float64 = 0.0001
	toolName                           = "hdr-sdr-manifest"
)

type AssessOptions struct {
	Prober      MediaProber
	Now         func() time.Time
	ToolVersion string
}

type assessor struct {
	cfg         Config
	prober      MediaProber
	cutPoints   []CutPoint
	cutMode     string
	cutLoadErr  error
	mediaHashes bool
}

func Assess(ctx context.Context, cfg Config, options AssessOptions) (Report, error) {
	if err := cfg.Validate(); err != nil {
		return Report{}, err
	}
	if options.Prober == nil {
		return Report{}, fmt.Errorf("media prober is required")
	}
	cutPoints, cutLoadErr := LoadCutPoints(cfg.Cuts)
	now := time.Now
	if options.Now != nil {
		now = options.Now
	}
	version := options.ToolVersion
	if version == "" {
		version = "dev"
	}
	mediaHashes := cfg.Checks.MediaHashes != nil && *cfg.Checks.MediaHashes

	a := assessor{
		cfg:         cfg,
		prober:      options.Prober,
		cutPoints:   cutPoints,
		cutMode:     NormalizeCutSource(cfg.Cuts),
		cutLoadErr:  cutLoadErr,
		mediaHashes: mediaHashes,
	}
	report := Report{
		Schema:      OutputSchema,
		GeneratedAt: now().UTC().Format(time.RFC3339),
		Generator:   GeneratorInfo{Name: toolName, Version: version},
		Project:     cfg.Project,
		Versions:    VersionsReport{},
		Shared:      SharedReport{Assets: []AssetReport{}, Checks: []Check{}},
		Failures:    []Failure{},
	}

	hdrProbe, hdrErr := options.Prober.Probe(ctx, cfg.HDR.Path)
	sdrProbe, sdrErr := options.Prober.Probe(ctx, cfg.SDR.Path)
	hdrTracks, hdrTrackChecks := a.collectTracks(ctx, cfg.HDR.Path, hdrProbe, hdrErr)
	sdrTracks, sdrTrackChecks := a.collectTracks(ctx, cfg.SDR.Path, sdrProbe, sdrErr)

	report.Versions.HDR = a.buildVersion("HDR", cfg.HDR, hdrProbe, hdrErr, hdrTracks, hdrTrackChecks)
	report.Versions.SDR = a.buildVersion("SDR", cfg.SDR, sdrProbe, sdrErr, sdrTracks, sdrTrackChecks)
	report.Shared.Timeline = a.buildTimeline(report.Versions.HDR.Video, report.Versions.SDR.Video)

	hdrCuts, sdrCuts := a.buildCuts(hdrProbe, sdrProbe)
	report.Versions.HDR.Cuts = hdrCuts
	report.Versions.SDR.Cuts = sdrCuts
	if a.cutLoadErr != nil {
		checkDetail := "cut point list is unavailable: " + a.cutLoadErr.Error()
		report.Versions.HDR.Checks = append(report.Versions.HDR.Checks, Check{
			ID:       "cuts.source_available",
			Category: "cuts",
			Required: true,
			Expected: "valid cut point list",
			Actual:   "unavailable",
			Detail:   checkDetail,
		})
		report.Versions.SDR.Checks = append(report.Versions.SDR.Checks, Check{
			ID:       "cuts.source_available",
			Category: "cuts",
			Required: true,
			Expected: "valid cut point list",
			Actual:   "unavailable",
			Detail:   checkDetail,
		})
	}

	hdrDefaults, sdrDefaults := a.buildDefaults(report.Versions.HDR, report.Versions.SDR)
	report.Versions.HDR.Defaults = hdrDefaults
	report.Versions.SDR.Defaults = sdrDefaults

	a.buildSharedAssets(ctx, &report, hdrTracks, sdrTracks)
	a.buildSharedSets(&report, hdrTracks, sdrTracks)
	a.finalize(&report)
	return report, nil
}

func (a *assessor) collectTracks(ctx context.Context, path string, probe ProbeData, probeErr error) ([]TrackSummary, []Check) {
	if probeErr != nil {
		return nil, nil
	}
	var tracks []TrackSummary
	var checks []Check
	for _, kind := range []string{"audio", "subtitle"} {
		streams := probe.StreamsOfType(kind)
		checks = append(checks, Check{
			ID:       "tracks." + kind + "_present",
			Category: "tracks",
			Required: true,
			Pass:     len(streams) > 0,
			Expected: "at least one " + kind + " stream",
			Actual:   len(streams),
			Detail:   missingStreamDetail(kind, len(streams)),
		})
		for _, stream := range streams {
			hash, err := a.prober.StreamSHA256(ctx, path, stream.Index)
			check := Check{
				ID:       fmt.Sprintf("tracks.%s_%d_stream_sha256", kind, stream.Index),
				Category: "tracks",
				Required: true,
				Pass:     err == nil && hash != "",
				Expected: "SHA-256 stream hash",
				Actual:   hash,
			}
			if check.Pass {
				check.Actual = hash
			} else {
				check.Detail = "could not hash " + kind + " stream"
				if err != nil {
					check.Detail += ": " + err.Error()
				}
			}
			checks = append(checks, check)
			tracks = append(tracks, summarizeTrack(stream, hash))
		}
	}
	return tracks, checks
}

func missingStreamDetail(kind string, count int) string {
	if count > 0 {
		return ""
	}
	return "no " + kind + " stream is present"
}

func summarizeTrack(stream ProbeStream, hash string) TrackSummary {
	summary := TrackSummary{
		Index:         stream.Index,
		Type:          stream.CodecType,
		Codec:         stream.CodecName,
		Language:      stream.Tag("language"),
		Title:         stream.Tag("title"),
		Default:       stream.IsDefault(),
		Forced:        stream.IsForced(),
		StreamSHA256:  hash,
		Channels:      stream.Channels,
		ChannelLayout: stream.ChannelLayout,
	}
	if rate, ok := ParsePositiveInt(stream.SampleRate); ok {
		summary.SampleRate = rate
	}
	if startMS, ok := SecondsToMS(stream.StartTime); ok {
		summary.StartMS = startMS
		summary.StartAvailable = true
	}
	if durationMS, ok := SecondsToMS(stream.Duration); ok {
		summary.DurationMS = durationMS
		summary.DurationAvailable = true
	}
	return summary
}

func (a *assessor) buildVersion(role string, versionConfig VersionConfig, probe ProbeData, probeErr error, tracks []TrackSummary, trackChecks []Check) VersionReport {
	label := strings.TrimSpace(versionConfig.Label)
	if label == "" {
		label = role
	}
	if tracks == nil {
		tracks = []TrackSummary{}
	}
	report := VersionReport{
		Role:           role,
		Label:          label,
		Path:           versionConfig.Path,
		Video:          buildVideoInfo(probe),
		Color:          a.buildColor(role, versionConfig.Color, probe, probeErr),
		Defaults:       buildDefaultSummary(tracks),
		EmbeddedTracks: tracks,
		Checks:         append([]Check(nil), trackChecks...),
	}

	fileCheck := Check{
		ID:       "source.file",
		Category: "source",
		Required: true,
		Expected: "readable regular non-empty file",
	}
	info, statErr := os.Stat(versionConfig.Path)
	fileCheck.Pass = statErr == nil && info.Mode().IsRegular() && info.Size() > 0
	if statErr != nil {
		fileCheck.Actual = statErr.Error()
		fileCheck.Detail = "media file is missing or unreadable"
	} else {
		fileCheck.Actual = fmt.Sprintf("%d bytes", info.Size())
		if !info.Mode().IsRegular() {
			fileCheck.Detail = "media path is not a regular file"
		} else if info.Size() == 0 {
			fileCheck.Detail = "media file is empty"
		}
	}
	report.Checks = append(report.Checks, fileCheck)

	probeCheck := Check{
		ID:       "source.probe",
		Category: "source",
		Required: true,
		Expected: "ffprobe succeeds",
		Pass:     probeErr == nil,
	}
	if probeErr != nil {
		probeCheck.Actual = "probe failed"
		probeCheck.Detail = probeErr.Error()
	} else {
		probeCheck.Actual = fmt.Sprintf("%d streams", len(probe.Streams))
	}
	report.Checks = append(report.Checks, probeCheck)

	videoCount := len(probe.StreamsOfType("video"))
	videoCheck := Check{
		ID:       "source.video_stream_count",
		Category: "source",
		Required: true,
		Expected: 1,
		Actual:   videoCount,
		Pass:     probeErr == nil && videoCount == 1,
	}
	if videoCheck.Pass {
		videoCheck.Detail = ""
	} else {
		videoCheck.Detail = "version must contain exactly one video stream"
	}
	report.Checks = append(report.Checks, videoCheck)

	if a.mediaHashes {
		hash, err := hashFile(versionConfig.Path)
		hashCheck := Check{
			ID:       "source.media_sha256",
			Category: "source",
			Required: true,
			Expected: "SHA-256 file hash",
			Pass:     err == nil && hash != "",
			Actual:   hash,
		}
		if err != nil {
			hashCheck.Detail = "could not hash media file: " + err.Error()
		} else {
			report.FileSHA256 = hash
		}
		report.Checks = append(report.Checks, hashCheck)
	}
	return report
}

func buildVideoInfo(probe ProbeData) VideoInfo {
	stream, ok := probe.FirstVideo()
	if !ok {
		return VideoInfo{}
	}
	info := VideoInfo{
		Index:         stream.Index,
		Codec:         stream.CodecName,
		Profile:       stream.Profile,
		Width:         stream.Width,
		Height:        stream.Height,
		PixelFormat:   stream.PixelFormat,
		TimeBase:      stream.TimeBase,
		FrameRateText: firstNonEmpty(stream.AverageFrameRate, stream.RealFrameRate),
	}
	if durationMS, ok := SecondsToMS(stream.Duration); ok {
		info.DurationMS = durationMS
		info.DurationAvailable = true
		info.DurationSource = "video_stream"
	} else if durationMS, ok := SecondsToMS(probe.Format.Duration); ok {
		info.DurationMS = durationMS
		info.DurationAvailable = true
		info.DurationSource = "container"
	}
	if startMS, ok := SecondsToMS(stream.StartTime); ok {
		info.StartMS = startMS
		info.StartAvailable = true
	} else if startMS, ok := SecondsToMS(probe.Format.StartTime); ok {
		info.StartMS = startMS
		info.StartAvailable = true
	}
	if fps, ok := ParseRational(stream.AverageFrameRate); ok {
		info.FrameRate = fps
		info.FrameRateAvailable = true
	} else if fps, ok := ParseRational(stream.RealFrameRate); ok {
		info.FrameRate = fps
		info.FrameRateAvailable = true
	}
	if frames, ok := ParsePositiveInt64(stream.NBReadFrames); ok {
		info.FrameCount = frames
		info.FrameCountAvailable = true
		info.FrameCountSource = "decoded_frame_count"
	} else if frames, ok := ParsePositiveInt64(stream.NBFrames); ok {
		info.FrameCount = frames
		info.FrameCountAvailable = true
		info.FrameCountSource = "stream_metadata"
	}
	if info.DurationMS > 0 && info.FrameRate > 0 {
		info.EstimatedFrames = int64(math.Round(float64(info.DurationMS) * info.FrameRate / 1000))
	}
	return info
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}
