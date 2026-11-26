package lighting

import (
	"fmt"
	"math"
	"sort"
)

// Finish builds the report. It can be called once all frames are supplied.
func (a *Analyzer) Finish() (Report, error) {
	n := len(a.samples)
	if n == 0 {
		return Report{}, fmt.Errorf("no frames analyzed")
	}
	fps := 0.0
	if n > 1 && a.lastTime > a.firstTime {
		fps = float64(n-1) / (a.lastTime - a.firstTime)
	}
	timeS := make([]float64, n)
	global := make([]float64, n)
	background := make([]float64, n)
	motion := make([]float64, n)
	motionEnergy := make([]float64, n)
	stableFraction := make([]float64, n)
	foreground := make([]float64, n)
	nRegions := a.cfg.RegionRows * a.cfg.RegionCols
	regionSeries := make([][]float64, nRegions)
	for r := range regionSeries {
		regionSeries[r] = make([]float64, n)
	}
	for i, s := range a.samples {
		timeS[i] = s.timeS
		global[i] = s.global
		background[i] = s.background
		motion[i] = s.motionFraction
		motionEnergy[i] = s.motionEnergy
		stableFraction[i] = s.stableFraction
		foreground[i] = s.foreground
		for r, v := range s.regions {
			regionSeries[r][i] = v
		}
	}

	smoothWindow := max(1, int(math.Round(0.75*fps)))
	sustained := movingAverage(global, smoothWindow)
	backgroundSustained := movingAverage(background, smoothWindow)
	fast := make([]float64, n)
	for i := range global {
		fast[i] = global[i] - sustained[i]
	}
	regionSmooth := make([][]float64, nRegions)
	regionWindow := max(1, int(math.Round(0.35*fps)))
	for r := range regionSeries {
		regionSmooth[r] = movingAverage(regionSeries[r], regionWindow)
	}

	bands := Bands{
		Global:     analyzeBands(global, fps, a.cfg),
		Background: analyzeBands(background, fps, a.cfg),
		Motion:     analyzeBands(motion, fps, a.cfg),
	}
	flicker := flickerFromBands(bands, a.cfg)
	events := detectLightEvents(a.samples, timeS, backgroundSustained, regionSmooth,
		fps, a.cfg, flicker)
	for i := range events {
		if events[i].Kind == "auto_exposure" {
			flicker.AutoExposureConfound = true
			break
		}
	}
	ledScore := flickerScore(bands, flicker, a.cfg)
	if flicker.Periodic {
		events = append(events, Event{
			Kind:       "led_flicker",
			StartS:     timeS[0],
			EndS:       timeS[n-1],
			Amplitude:  bands.Background[2].RMSLuma,
			Frequency:  flicker.PeakHz,
			Confidence: ledScore,
			Retained:   true,
			Evidence: EventEvidence{
				DurationS:          timeS[n-1] - timeS[0],
				AmplitudeLuma:      bands.Background[2].RMSLuma,
				BackgroundFraction: meanSlice(stableFraction),
				MotionFraction:     meanSlice(motion),
				FlickerRelative:    flicker.BackgroundRelative,
			},
		})
		sort.SliceStable(events, func(i, j int) bool {
			if events[i].StartS == events[j].StartS {
				return events[i].Kind == "led_flicker"
			}
			return events[i].StartS < events[j].StartS
		})
	}

	backgroundRange := rangeOf(backgroundSustained)
	globalRange := rangeOf(global)
	sustainedRange := rangeOf(sustained)
	regionCoherence := 1.0
	if backgroundRange >= 0.5 {
		sum, count := 0.0, 0
		for _, series := range regionSmooth {
			sum += corr(backgroundSustained, series)
			count++
		}
		if count > 0 {
			regionCoherence = clamp(sum/float64(count), -1, 1)
		}
	}
	corrBackground := corr(global, background)
	corrMotion := corr(global, motion)
	if globalRange < 0.05 && backgroundRange < 0.05 {
		corrBackground = 1
	}
	flickerIndep := 1.0
	if len(fast) > 1 {
		motionFast := movingAverage(motion, smoothWindow)
		for i := range motionFast {
			motionFast[i] = motion[i] - motionFast[i]
		}
		flickerIndep = clamp(1-math.Abs(corr(fast, motionFast)), 0, 1)
	}

	classification := classifyReport(events, flicker, ledScore, a.cfg)
	warnings := reportWarnings(a, fps, n, meanSlice(stableFraction))
	report := Report{
		Version: Version,
		Source: Source{
			Width:      a.width,
			Height:     a.height,
			FPS:        fps,
			FrameCount: n,
			DurationS:  timeS[n-1],
		},
		Config: EffectiveConfig{
			MaxAnalysisWidth:    a.cfg.MaxAnalysisWidth,
			MotionThreshold:     a.cfg.MotionThreshold,
			SlowCutoffHz:        a.cfg.SlowCutoffHz,
			ExposureBandHighHz:  a.cfg.ExposureBandHighHz,
			FlickerBandLowHz:    a.cfg.FlickerBandLowHz,
			FlickerMinRMS:       a.cfg.FlickerMinRMS,
			EventSlopeThreshold: a.cfg.EventSlopeThreshold,
			EventMinAmplitude:   a.cfg.EventMinAmplitude,
		},
		Classification: classification,
		Evidence: Evidence{
			StableBackground: BackgroundEvidence{
				MeanFraction:   meanSlice(stableFraction),
				MinFraction:    minOf(stableFraction),
				SlowRMS:        bandRMS(bands.Background, 0),
				SustainedRange: backgroundRange,
				Correlation:    corrBackground,
			},
			ForegroundMotion: MotionEvidence{
				MeanFraction:      meanSlice(motion),
				PeakFraction:      maxOf(motion),
				MeanEnergy:        meanSlice(motionEnergy),
				FlickerRMS:        bandRMS(bands.Motion, 2),
				GlobalCorrelation: corrMotion,
				FlickerIndep:      flickerIndep,
			},
			GlobalBrightness: GlobalEvidence{
				Range:              globalRange,
				SustainedRange:     sustainedRange,
				SlowRMS:            bandRMS(bands.Global, 0),
				FlickerRMS:         bandRMS(bands.Global, 2),
				BackgroundCorr:     corrBackground,
				RegionCoherence:    regionCoherence,
				DominantChangeRate: maxChangeRate(timeS, sustained),
			},
			Flicker: flicker,
		},
		Bands:  bands,
		Events: events,
		Curves: Curves{
			TimeS:                    timeS,
			GlobalLuma:               global,
			StableBackgroundLuma:     background,
			StableBackgroundFraction: stableFraction,
			ForegroundLuma:           foreground,
			ForegroundMotion:         motion,
			SustainedLuma:            sustained,
			SustainedBackgroundLuma:  backgroundSustained,
			FastResidualLuma:         fast,
			RegionLuma:               regionSeries,
		},
		Warnings: warnings,
	}
	return report, nil
}
