package main

import (
	"math"
	"time"
)

func buildSegment(frames []FrameMetrics) Segment {
	worst := frames[0]
	var cancellation, correlation, sideMid, severity, phaseX, phaseY float64

	for _, frame := range frames {
		cancellation += frame.CancellationDB
		correlation += frame.Correlation
		phaseX += math.Cos(wrapPhase(frame.VocalPhaseRadians))
		phaseY += math.Sin(wrapPhase(frame.VocalPhaseRadians))
		sideMid += frame.SideMidRatioDB
		severity += frameSeverity(frame)
		if frameSeverity(frame) > frameSeverity(worst) {
			worst = frame
		}
	}

	count := float64(len(frames))
	segment := Segment{
		Start:            frames[0].StartTime,
		End:              frames[len(frames)-1].StartTime + frameDuration(frames[0]),
		Frames:           len(frames),
		Severity:         severity / count,
		WorstFrame:       worst,
		MeanCancellation: cancellation / count,
		MeanCorrelation:  correlation / count,
		MeanPhase:        math.Atan2(phaseY/count, phaseX/count),
		MeanSideMid:      sideMid / count,
		Evidence:         buildEvidence(worst),
	}
	return segment
}

func frameDuration(frame FrameMetrics) time.Duration {
	return frame.EndTime() - frame.StartTime
}

func (f FrameMetrics) EndTime() time.Duration {
	return f.StartTime + time.Duration(float64(f.WindowSize)/float64(f.SampleRate)*float64(time.Second))
}

func frameSeverity(frame FrameMetrics) float64 {
	cancellationScore := clamp((-frame.CancellationDB-10)/20, 0, 1)
	vocalScore := clamp((frame.VocalCancellation-0.65)/0.35, 0, 1)
	phaseScore := clamp((math.Abs(wrapPhase(frame.VocalPhaseRadians))-2)/(math.Pi-2), 0, 1)
	correlationScore := clamp((-frame.Correlation-0.15)/0.7, 0, 1)
	coherenceScore := clamp((frame.VocalCoherence-0.35)/0.45, 0, 1)
	sideScore := clamp((frame.SideMidRatioDB-3)/12, 0, 1)

	return 0.30*cancellationScore +
		0.30*vocalScore +
		0.15*phaseScore +
		0.10*correlationScore +
		0.10*coherenceScore +
		0.05*sideScore
}

func buildEvidence(frame FrameMetrics) Evidence {
	bands := make(map[string]float64, len(frame.Bands))
	for _, band := range frame.Bands {
		bands[band.Name] = band.Cancellation
	}
	return Evidence{
		LeftDBFS:         frame.LeftRMSDB,
		RightDBFS:        frame.RightRMSDB,
		BalanceDB:        frame.LeftRMSDB - frame.RightRMSDB,
		MonoDBFS:         frame.MonoRMSDB,
		CancellationDB:   frame.CancellationDB,
		Correlation:      frame.Correlation,
		SideMidRatioDB:   frame.SideMidRatioDB,
		VocalPhaseDeg:    wrapPhase(frame.VocalPhaseRadians) * 180 / math.Pi,
		VocalCoherence:   frame.VocalCoherence,
		BandCancellation: bands,
	}
}

func clamp(value, low, high float64) float64 {
	return math.Max(low, math.Min(high, value))
}
