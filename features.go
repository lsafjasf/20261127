package tearcheck

import "math"

type bandMatch struct {
	diff  float64
	shift int
}

func matchBandShift(cur, prev *LumaFrame, y0, y1, maxShift int) bandMatch {
	if cur == nil || prev == nil || cur.Width != prev.Width || cur.Height != prev.Height {
		return bandMatch{diff: 1, shift: 0}
	}
	y0 = maxInt(0, y0)
	y1 = minInt(cur.Height, y1)
	if y1 <= y0 {
		return bandMatch{diff: 1, shift: 0}
	}

	best := bandMatch{diff: math.MaxFloat64}
	for shift := -maxShift; shift <= maxShift; shift++ {
		var sum float64
		var count int
		xStart := maxInt(0, -shift)
		xEnd := minInt(cur.Width, cur.Width-shift)
		for y := y0; y < y1; y++ {
			curOff := cur.offset(0, y)
			prevOff := prev.offset(0, y)
			for x := xStart; x < xEnd; x += 2 {
				sum += absByteDiff(cur.Pix[curOff+x], prev.Pix[prevOff+x+shift])
				count++
			}
		}
		if count == 0 {
			continue
		}
		diff := sum / float64(count) / 255.0
		if diff < best.diff {
			best = bandMatch{diff: diff, shift: shift}
		}
	}
	if best.diff == math.MaxFloat64 {
		best.diff = 1
	}
	return best
}

// TemporalSplice measures whether one side of a seam repeats an adjacent frame.
func TemporalSplice(cur, prev *LumaFrame, seamY int) (match, contrast float64, side Side, shift int) {
	if prev == nil || cur == nil {
		return 0, 0, SideNone, 0
	}
	bandHeight := clampInt(cur.Height/32, 4, 10)
	maxShift := clampInt(cur.Width/40, 6, 32)
	above := matchBandShift(cur, prev, seamY-bandHeight, seamY, maxShift)
	below := matchBandShift(cur, prev, seamY, seamY+bandHeight, maxShift)

	best := above
	worst := below
	bestSide := SideAbove
	if below.diff < above.diff {
		best = below
		worst = above
		bestSide = SideBelow
	}

	match = Clamp01(1 - best.diff/0.16)
	contrast = Clamp01((worst.diff - best.diff) / (worst.diff + 0.06))
	shift = best.shift

	switch {
	case above.diff <= 0.06 && below.diff <= 0.06:
		side = SideBoth
		// Both sides matching means a stable frame region, not a one-sided
		// old/new framebuffer splice.
		match *= 0.12
		contrast = math.Min(contrast, 0.15)
	case best.diff <= 0.12 && worst.diff >= best.diff+0.03 && contrast >= 0.32:
		side = bestSide
	default:
		side = SideNone
		// A low-error match without contrast is usually a static region.
		match *= 0.35
		contrast *= 0.25
	}
	return match, contrast, side, shift
}

// BlockArtifactScore estimates horizontal and vertical energy on codec grids.
func BlockArtifactScore(frame *LumaFrame, seamY int, sizes []int) float64 {
	if frame == nil || frame.Width < 32 || frame.Height < 32 {
		return 0
	}
	block := 16
	bestAlign := 0.0
	for _, size := range sizes {
		align := BlockAlignmentAt(seamY, []int{size})
		if align > bestAlign && size >= 8 {
			bestAlign = align
			block = size
		}
	}

	edgeMAD, _ := rowBoundaryStats(frame, seamY)
	near := make([]float64, 0, block*2)
	for y := maxInt(1, seamY-block*2); y < minInt(frame.Height, seamY+block*2+1); y++ {
		if absInt(y-seamY) <= 1 {
			continue
		}
		mad, _ := rowBoundaryStats(frame, y)
		near = append(near, mad)
	}
	local := medianFloat(near)
	horizontal := 0.0
	if local > 0.5 {
		horizontal = Clamp01((edgeMAD/local - 1.25) / 1.5)
	} else if edgeMAD > 18 {
		horizontal = 0.8
	}

	y0 := maxInt(0, seamY-block)
	y1 := minInt(frame.Height, seamY+2*block)
	var alignedSum, otherSum float64
	var alignedCount, otherCount int
	for y := y0; y < y1; y += 1 {
		off := frame.offset(0, y)
		for x := 1; x < frame.Width; x++ {
			diff := absByteDiff(frame.Pix[off+x], frame.Pix[off+x-1])
			if x%block == 0 {
				alignedSum += diff
				alignedCount++
			} else if x%4 == 2 {
				otherSum += diff
				otherCount++
			}
		}
	}
	aligned := average(alignedSum, alignedCount)
	other := average(otherSum, otherCount)
	vertical := Clamp01((aligned/(other+2.0) - 1.12) / 1.2)
	return Clamp01(0.45*horizontal + 0.55*vertical)
}

// TemporalBlockRepeat measures near-exact repetition in the aligned block row.
func TemporalBlockRepeat(cur, prev *LumaFrame, seamY int, sizes []int) float64 {
	if prev == nil || cur == nil {
		return 0
	}
	block := 16
	for _, size := range sizes {
		if BlockAlignmentAt(seamY, []int{size}) > 0.9 && size >= 8 {
			block = size
			break
		}
	}
	y0 := seamY - seamY%block
	y1 := minInt(cur.Height, y0+block)
	match := matchBandShift(cur, prev, y0, y1, 2)
	return Clamp01(1 - match.diff/0.10)
}

func average(sum float64, count int) float64 {
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}

func clampInt(v, low, high int) int {
	if v < low {
		return low
	}
	if v > high {
		return high
	}
	return v
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
