package iccproof

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"
)

// Proof runs every requested intent for every sample and returns flat,
// per-sample results plus intent summaries.
func Proof(req Request) (*Report, error) {
	return ProofContext(context.Background(), req)
}

func ProofContext(ctx context.Context, req Request) (*Report, error) {
	cfg := req.Config.withDefaults()
	if cfg.OutputBitDepth != 8 && cfg.OutputBitDepth != 16 {
		return nil, fmt.Errorf("unsupported output_bit_depth %d: use 8 or 16", cfg.OutputBitDepth)
	}
	intents := req.Intents
	if len(intents) == 0 {
		return nil, fmt.Errorf("%w: request.Intents is empty; use AllProofIntents() or ParseIntents(\"all\")", ErrInvalidIntent)
	}
	for _, intent := range intents {
		if !intent.Valid() {
			return nil, fmt.Errorf("%w: %q", ErrInvalidIntent, intent)
		}
	}
	if profileRefEmpty(req.Source) && req.PhotoPath != "" {
		embedded, err := SourceProfileFromPhoto(req.PhotoPath)
		if err != nil {
			return nil, err
		}
		req.Source = embedded
	}
	if err := ExplicitProfiles(req.Source, req.Media); err != nil {
		return nil, err
	}
	source, err := LoadProfile(RoleSource, req.Source)
	if err != nil {
		return nil, err
	}
	media, err := LoadProfile(RoleMedia, req.Media)
	if err != nil {
		return nil, err
	}
	var display *LoadedProfile
	if req.Display != nil {
		display, err = LoadProfile(RoleDisplay, *req.Display)
		if err != nil {
			return nil, err
		}
	}

	samples, err := resolveSamples(req)
	if err != nil {
		return nil, err
	}

	engine := req.Engine
	ownsEngine := false
	if engine == nil {
		engine, err = NewEngine(source, media, cfg)
		if err != nil {
			return nil, err
		}
		ownsEngine = true
	}
	if display != nil && display.Ref.Descriptor == nil {
		if reference, isReference := engine.(*ReferenceEngine); isReference && ownsEngine {
			_ = reference.Close()
			engine, err = newICCEngine(source, media, cfg)
			if err != nil {
				return nil, err
			}
		}
	}
	if ownsEngine {
		defer engine.Close()
	}
	if display != nil {
		attacher, ok := engine.(DisplayAttacher)
		if !ok {
			return nil, fmt.Errorf("%w: engine %T does not support display_profile", ErrBackend, engine)
		}
		if err := attacher.AttachDisplay(display); err != nil {
			return nil, err
		}
	}

	report := &Report{
		SchemaVersion: ReportSchemaVersion,
		GeneratedAt:   time.Now().UTC(),
		Intents:       append([]Intent(nil), intents...),
		SourceProfile: source.Info,
		MediaProfile:  media.Info,
		Config:        cfg,
		Results:       make([]SampleResult, 0, len(samples)*len(intents)),
	}
	if display != nil {
		info := display.Info
		report.DisplayProfile = &info
	}
	if req.Engine != nil {
		report.Config.Engine = "injected"
	} else if source.Ref.Descriptor != nil && media.Ref.Descriptor != nil {
		report.Config.Engine = "reference-descriptor"
	} else {
		report.Config.Engine = "littlecms2"
	}

	for _, intent := range intents {
		for sampleIndex, sample := range samples {
			if sample.Index == 0 {
				sample.Index = sampleIndex
			}
			report.Results = append(report.Results, proofOne(ctx, engine, intent, sample))
		}
	}
	report.Summaries = summarize(report.Intents, report.Results)
	report.IntentComparisons = compareIntents(report.Intents, report.Results)
	return report, nil
}

func resolveSamples(req Request) ([]Sample, error) {
	if len(req.Samples) > 0 {
		return append([]Sample(nil), req.Samples...), nil
	}
	if req.PhotoPath != "" {
		return SamplesFromPhoto(req.PhotoPath, req.SampleGrid)
	}
	return nil, fmt.Errorf("at least one sample or photo_path is required")
}

func proofOne(ctx context.Context, engine Engine, intent Intent, sample Sample) SampleResult {
	result := SampleResult{
		Intent: intent, IntentICC: intent.ICCValue(), SampleIndex: sample.Index, SampleID: sample.ID,
		Boundary: newOKBoundary(),
	}
	if x, y := sample.Coordinate(); x >= 0 && y >= 0 {
		xValue, yValue := x, y
		result.X, result.Y = &xValue, &yValue
	}
	if len(sample.RGB) == 3 {
		result.InputRGB = append([]float64(nil), sample.RGB...)
	} else if len(sample.CodeRGB) == 3 {
		result.InputRGB = codeToNormalized(sample.CodeRGB, sample.BitDepth)
	}
	if sample.Lab != nil {
		result.InputLab = sample.Lab
	}

	rgb, sourceViolations, err := sample.NormalizedRGB()
	if err != nil {
		result.Error = err.Error()
		result.Boundary.Classification = ClassError
		result.Boundary.Stage = BoundarySource
		result.Boundary.Messages = append(result.Boundary.Messages, err.Error())
		result.Boundary.Finalize()
		return result
	}
	result.Boundary.SourceViolations = append(result.Boundary.SourceViolations, sourceViolations...)

	out, err := engine.ProofSample(ctx, intent, sample, rgb)
	if err != nil {
		result.Error = err.Error()
		result.Boundary.Classification = ClassError
		result.Boundary.Messages = append(result.Boundary.Messages, err.Error())
		return result
	}

	result.SourceLab = out.SourceLab
	result.MediaColorSpace = out.MediaColorSpace
	result.MediaValues = out.MediaValues
	result.MediaRawValues = out.MediaRawValues
	result.MediaEncoded = out.MediaEncoded
	result.MediaCodes = out.MediaCodes
	result.MediaLab = out.MediaLab
	result.DisplayRGB = out.DisplayValues
	result.DisplayCodes = out.DisplayCodes
	result.Boundary.SourceGamutAlarm = out.SourceGamutAlarm
	result.Boundary.MediaGamutAlarm = out.MediaGamutAlarm
	result.Boundary.MediaViolations = channelViolations(out.GamutValues, out.MediaColorSpace)
	result.Boundary.ClippedChannels = clippingDetails(out.MediaValues, out.MediaEncoded, out.MediaCodes, out.MediaColorSpace)
	if len(result.Boundary.ClippedChannels) > 0 {
		result.Boundary.DeviceClipped = true
	}

	if out.SourceLab != nil && out.MediaLab != nil {
		d00 := DeltaE00(*out.SourceLab, *out.MediaLab)
		d76 := DeltaE76(*out.SourceLab, *out.MediaLab)
		result.DeltaE = &d00
		result.DeltaE00 = &d00
		result.DeltaE76 = &d76
		reliable := len(sourceViolations) == 0 && (out.SourceGamutAlarm == nil || !*out.SourceGamutAlarm)
		result.DeltaEReliable = &reliable
	}
	if len(sourceViolations) > 0 {
		result.Boundary.Messages = append(result.Boundary.Messages, "delta_e is an extrapolated transform; source encoding cannot represent this sample")
	}
	result.Boundary.Finalize()
	return result
}

func codeToNormalized(codes []int, bits int) []float64 {
	if bits == 0 {
		bits = 8
	}
	maxCode := float64(int(1)<<bits - 1)
	out := make([]float64, len(codes))
	for i, code := range codes {
		out[i] = float64(code) / maxCode
	}
	return out
}

func channelNames(space string, n int) []string {
	if space == "CMYK" || n == 4 {
		return []string{"C", "M", "Y", "K"}
	}
	return []string{"R", "G", "B"}
}

func channelViolations(values []float64, space string) []ChannelBoundary {
	names := channelNames(space, len(values))
	var out []ChannelBoundary
	for i, v := range values {
		name := fmt.Sprintf("ch%d", i)
		if i < len(names) {
			name = names[i]
		}
		if v < -DomainTolerance || v > 1+DomainTolerance {
			out = append(out, ChannelBoundary{
				Stage: BoundaryMedia, Channel: i, ChannelName: name,
				Value: v, Min: 0, Max: 1,
				Exceedance: maxFloat(-v, v-1), Reason: ReasonMediaGamut,
			})
		}
	}
	return out
}

func clippingDetails(pre, encoded []float64, codes []int, space string) []ChannelBoundary {
	names := channelNames(space, len(pre))
	var out []ChannelBoundary
	for i, v := range pre {
		name := fmt.Sprintf("ch%d", i)
		if i < len(names) {
			name = names[i]
		}
		over := v > 1+DomainTolerance
		under := v < -DomainTolerance
		if !over && !under {
			continue
		}
		atLimit := false
		var after float64
		if i < len(encoded) {
			after = encoded[i]
			atLimit = math.Abs(after) < 1e-6 || math.Abs(after-1) < 1e-6
		}
		out = append(out, ChannelBoundary{
			Stage: BoundaryMedia, Channel: i, ChannelName: name,
			Value: v, Min: 0, Max: 1, After: after,
			Exceedance: maxFloat(-v, v-1), AtLimit: atLimit,
			Reason: ReasonDeviceChannelClip,
		})
	}
	return out
}

func summarize(intents []Intent, results []SampleResult) []IntentSummary {
	summaries := make([]IntentSummary, 0, len(intents))
	for _, intent := range intents {
		s := IntentSummary{Intent: intent, IntentICCValue: intent.ICCValue()}
		var des []float64
		var maxIndex int
		maxDE := -1.0
		for _, r := range results {
			if r.Intent != intent {
				continue
			}
			s.SampleCount++
			switch r.Boundary.Classification {
			case ClassSourceUnrepresentable:
				s.SourceUnrepresentable++
			case ClassDeviceClipped:
				s.DeviceClipped++
			}
			if r.Boundary.MediaGamutAlarm {
				s.GamutAlarm++
			}
			if r.Boundary.Classification != ClassSourceUnrepresentable && r.Boundary.Classification != ClassError &&
				((r.Boundary.MediaInGamut != nil && !*r.Boundary.MediaInGamut) || r.Boundary.MediaGamutAlarm) {
				s.MediaOutOfGamut++
			}
			if r.DeltaE00 != nil && r.DeltaEReliable != nil && !*r.DeltaEReliable {
				s.UnreliableDeltaE++
			}
			if r.DeltaE00 != nil && (r.DeltaEReliable == nil || *r.DeltaEReliable) {
				des = append(des, *r.DeltaE00)
				if *r.DeltaE00 > maxDE {
					maxDE = *r.DeltaE00
					s.DeltaE.Max = *r.DeltaE00
					maxIndex = r.SampleIndex
				}
			}
		}
		if len(des) > 0 {
			sort.Float64s(des)
			s.DeltaE.Count = len(des)
			s.DeltaE.Min = des[0]
			s.DeltaE.Mean = mean(des)
			s.DeltaE.Max = des[len(des)-1]
			s.DeltaE.MaxSampleIndex = maxIndex
		}
		summaries = append(summaries, s)
	}
	return summaries
}

func mean(values []float64) float64 {
	var sum float64
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}
