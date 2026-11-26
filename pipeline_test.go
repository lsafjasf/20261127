package tearcheck

import (
	"image"
	"math"
	"testing"
)

func patternGray(width, height, phase int) *image.Gray {
	img := image.NewGray(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			value := 128 +
				68*math.Sin(float64(x+phase*11)/6.2) +
				22*math.Sin(float64(y+phase*3)/8.0)
			if value < 0 {
				value = 0
			}
			if value > 255 {
				value = 255
			}
			img.Pix[img.PixOffset(x, y)] = uint8(value)
		}
	}
	return img
}

func splicedHardwareFrame(width, height, seam, prevPhase, curPhase int) (*image.Gray, *image.Gray) {
	prev := patternGray(width, height, prevPhase)
	cur := patternGray(width, height, curPhase)
	for y := seam; y < height; y++ {
		src := prev.PixOffset(0, y)
		dst := cur.PixOffset(0, y)
		copy(cur.Pix[dst:dst+width], prev.Pix[src:src+width])
	}
	return cur, prev
}

func TestSyntheticHardwareTearPipeline(t *testing.T) {
	const width, height, seam = 160, 120, 41
	curImg, prevImg := splicedHardwareFrame(width, height, seam, 0, 1)
	cur := LumaFromImage(curImg, 1, 1.0/30)
	prev := LumaFromImage(prevImg, 0, 0)

	observations := ExtractObservations(cur, prev, DefaultBlockSizes("h264"))
	if len(observations) == 0 {
		t.Fatal("expected seam observation")
	}
	var best Observation
	bestDistance := 999
	for _, observation := range observations {
		if distance := absInt(observation.SeamY - seam); distance < bestDistance {
			bestDistance = distance
			best = observation
		}
	}
	if bestDistance > 3 {
		t.Fatalf("seam y=%d, want near %d; observations=%+v", best.SeamY, seam, observations)
	}
	t.Logf("hardware observation: %+v", best)
	finding := ClassifyObservation(best)
	if finding.Cause != CauseHardwareSync || finding.ReviewRequired {
		t.Fatalf("verdict=%s confidence=%s review=%v scores=%#v manual=%+v",
			finding.Cause, finding.Confidence, finding.ReviewRequired, finding.Scores, finding.Manual)
	}
}

func TestSyntheticEncodingBlockPipeline(t *testing.T) {
	const width, height, seam = 160, 128, 48
	base := patternGray(width, height, 2)
	prev := LumaFromImage(base, 0, 0)

	corrupt := patternGray(width, height, 2)
	for y := seam; y < seam+16; y++ {
		for x := 16; x < 80; x++ {
			corrupt.Pix[corrupt.PixOffset(x, y)] = 8
		}
	}
	cur := LumaFromImage(corrupt, 1, 1.0/30)
	observations := ExtractObservations(cur, prev, DefaultBlockSizes("h264"))
	if len(observations) == 0 {
		t.Fatal("expected block-boundary observation")
	}
	var best Observation
	bestDistance := 999
	for _, observation := range observations {
		if distance := absInt(observation.SeamY - seam); distance < bestDistance {
			bestDistance = distance
			best = observation
		}
	}
	if bestDistance > 2 {
		t.Fatalf("block seam y=%d want %d; all=%+v", best.SeamY, seam, observations)
	}
	report := AnalyzeLumaFrames([]*LumaFrame{prev, cur}, Metadata{
		Codec: "h264",
		CodecEvents: []CodecEvent{{
			FrameIndex: 1,
			Y:          seam,
			Confidence: 1,
			Message:    "error while decoding MB 2 3",
		}},
	})
	var finding *Finding
	for i := range report.Findings {
		if report.Findings[i].FrameIndex == 1 && absInt(report.Findings[i].SeamY-seam) <= 2 {
			finding = &report.Findings[i]
			break
		}
	}
	if finding == nil {
		t.Fatalf("missing encoding finding: %+v", report)
	}
	t.Logf("encoding finding: cause=%s scores=%#v manual=%+v", finding.Cause, finding.Scores, finding.Manual)
	if finding.Cause != CauseEncoding || finding.ReviewRequired {
		t.Fatalf("expected actionable encoding_error, got %s review=%v scores=%#v", finding.Cause, finding.ReviewRequired, finding.Scores)
	}
}
